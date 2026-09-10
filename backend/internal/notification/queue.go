package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SnapshotSource interface {
	Snapshot(context.Context, Event) (Snapshot, error)
	Enabled(context.Context, Event) (bool, error)
}

type TextSender interface {
	SendText(context.Context, Target, string) error
}

type Queue struct {
	dir     string
	targets map[uint64][]Target
	source  SnapshotSource
	sender  TextSender
	sent    *sentStateStore
	mu      sync.Mutex
}

type job struct {
	Event            Event     `json:"event"`
	Target           Target    `json:"target"`
	Messages         []string  `json:"messages,omitempty"`
	ExpiresAt        time.Time `json:"expires_at,omitempty"`
	NextPart         int       `json:"next_part"`
	Attempts         int       `json:"attempts"`
	NextTry          time.Time `json:"next_try"`
	Status           string    `json:"status"`
	ErrorCode        string    `json:"error_code,omitempty"`
	Topic            string    `json:"topic,omitempty"`
	ContentVersion   string    `json:"content_version,omitempty"`
	ContentHash      string    `json:"content_hash,omitempty"`
	CanonicalContent string    `json:"canonical_content,omitempty"`
}

func NewQueue(dir string, targets map[uint64][]Target, source SnapshotSource, sender TextSender) (*Queue, error) {
	for _, state := range []string{"pending", "completed", "failed"} {
		if err := os.MkdirAll(filepath.Join(dir, state), 0o700); err != nil {
			return nil, fmt.Errorf("create notification queue: %w", err)
		}
	}
	sent, err := newSentStateStore(dir)
	if err != nil {
		return nil, err
	}
	return &Queue{dir: dir, targets: cloneTargets(targets), source: source, sender: sender, sent: sent}, nil
}

func (q *Queue) Enqueue(event Event) error {
	targets := q.targetsForGroup(event.GroupID)
	if len(targets) == 0 {
		return nil
	}
	if _, err := time.Parse("2006-01-02", event.LogicalDate); err != nil || event.RecordID == 0 || event.Initial != "" {
		return errors.New("invalid notification event")
	}
	for _, target := range targets {
		name := fmt.Sprintf("%020d-%020d-%020d-%d-%s.json",
			event.RecordID, event.GroupID, target.ChatID, target.ChatType, event.LogicalDate)
		if err := q.enqueue(name, event, target); err != nil {
			return err
		}
	}
	return nil
}

// EnqueueInitial persists one job per topic and binding, independent of restarts.
func (q *Queue) EnqueueInitial(now time.Time) error {
	for groupID, targets := range q.targetsSnapshot() {
		for _, target := range targets {
			if err := q.EnqueueInitialBinding(groupID, target, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q *Queue) EnqueueInitialBinding(groupID uint64, target Target, now time.Time) error {
	for _, topic := range []string{"daily", "weekly"} {
		name := fmt.Sprintf("%020d-initial-%020d-%d-%d-%s.json",
			0, groupID, target.ChatID, target.ChatType, topic)
		event := Event{GroupID: groupID, OccurredAt: now, Initial: topic}
		if err := q.enqueue(name, event, target); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) SetTargets(targets map[uint64][]Target) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.targets = cloneTargets(targets)
}

func (q *Queue) WakeInitial(groupID uint64, now time.Time) error {
	for _, target := range q.targetsForGroup(groupID) {
		for _, topic := range []string{"daily", "weekly"} {
			name := fmt.Sprintf("%020d-initial-%020d-%d-%d-%s.json",
				0, groupID, target.ChatID, target.ChatType, topic)
			if err := q.rearmInitial(name, now); err != nil {
				return err
			}
		}
		if err := q.EnqueueInitialBinding(groupID, target, now); err != nil {
			return err
		}
	}
	return nil
}

func (q *Queue) rearmInitial(name string, now time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, err := os.Stat(filepath.Join(q.dir, "pending", name)); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check pending initial notification: %w", err)
	}
	var sourcePath string
	var data []byte
	for _, state := range []string{"completed", "failed"} {
		path := filepath.Join(q.dir, state, name)
		var err error
		data, err = os.ReadFile(path)
		if err == nil {
			sourcePath = path
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("read initial notification: %w", err)
		}
	}
	if sourcePath == "" {
		return nil
	}
	var item job
	if err := json.Unmarshal(data, &item); err != nil {
		return fmt.Errorf("decode initial notification: %w", err)
	}
	if item.Event.Initial == "" {
		return nil
	}
	item.Event.OccurredAt = now
	item.Messages = nil
	item.ExpiresAt = time.Time{}
	item.NextPart = 0
	item.Attempts = 0
	item.NextTry = time.Time{}
	item.Status = "pending"
	item.ErrorCode = ""
	pending := filepath.Join(q.dir, "pending", name)
	if err := writeJob(pending, item); err != nil {
		return err
	}
	if err := os.Remove(sourcePath); err != nil {
		return fmt.Errorf("remove previous initial notification: %w", err)
	}
	return nil
}

func (q *Queue) enqueue(name string, event Event, target Target) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, state := range []string{"pending", "completed", "failed"} {
		if _, err := os.Stat(filepath.Join(q.dir, state, name)); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check notification event: %w", err)
		}
	}
	pending := job{Event: event, Target: target, Status: "pending"}
	if err := writeJob(filepath.Join(q.dir, "pending", name), pending); err != nil {
		return err
	}
	slog.Info("checkin notification queued", "record_id", event.RecordID, "group_id", event.GroupID,
		"initial", event.Initial)
	return nil
}

// Run has one owner per directory. Each tick sends at most one message part.
func (q *Queue) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			q.processNext(ctx, now)
		}
	}
}

func (q *Queue) processNext(ctx context.Context, now time.Time) {
	files, err := os.ReadDir(filepath.Join(q.dir, "pending"))
	if err != nil {
		slog.ErrorContext(ctx, "checkin notification queue read failed", "error", err)
		return
	}
	blocked := make(map[Target]bool)
	for _, file := range files {
		if ctx.Err() != nil {
			return
		}
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		path := filepath.Join(q.dir, "pending", file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.ErrorContext(ctx, "checkin notification queue read failed", "error", err)
			return
		}
		var item job
		if err := json.Unmarshal(data, &item); err != nil ||
			item.NextPart < 0 || item.NextPart > len(item.Messages) || item.Attempts < 0 {
			slog.ErrorContext(ctx, "checkin notification queue invalid", "file", file.Name())
			if err := os.Rename(path, filepath.Join(q.dir, "failed", file.Name())); err != nil {
				slog.ErrorContext(ctx, "checkin notification quarantine failed", "error", err)
			}
			return
		}
		if blocked[item.Target] {
			continue
		}
		blocked[item.Target] = true
		// Preserve ordering within a chat while allowing other chats to progress.
		if item.Status == "pending" && now.Before(item.NextTry) {
			continue
		}
		q.process(ctx, path, &item, now)
		return
	}
}

func (q *Queue) process(ctx context.Context, path string, item *job, now time.Time) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if item.Status != "pending" {
		if item.Status == "sent" {
			q.finish(ctx, path, item, start)
			return
		}
		q.archive(ctx, path, item)
		return
	}
	if !q.targetEnabled(item.Event.GroupID, item.Target) {
		item.Status = "skipped"
		item.ErrorCode = "target_changed"
		q.finish(ctx, path, item, start)
		return
	}
	enabled, err := q.source.Enabled(ctx, item.Event)
	if err != nil {
		err = &deliveryError{code: "notification_settings_read_failed", retry: true}
	} else if !enabled {
		item.Status, item.ErrorCode = "skipped", "notification_disabled"
		q.finish(ctx, path, item, start)
		return
	}
	if err == nil && len(item.Messages) == 0 {
		if item.Event.Initial != "" {
			item.Event.OccurredAt = now
		}
		var snapshot Snapshot
		snapshot, err = q.source.Snapshot(ctx, item.Event)
		if err == nil {
			item.Messages = splitMessage(snapshot.Text)
			item.ExpiresAt = snapshot.ExpiresAt
			if len(item.Messages) == 0 {
				item.Status = "skipped"
				item.ErrorCode = "ineligible_or_deleted"
				q.finish(ctx, path, item, start)
				return
			}
			item.Topic = snapshot.Topic
			if !validTopic(item.Topic) {
				item.Topic = inferTopic(snapshot.Text)
			}
			item.ContentVersion = snapshot.Version
			if item.ContentVersion == "" {
				item.ContentVersion = legacyContentVersion(*item, item.Topic)
			}
			item.CanonicalContent = canonicalNotificationContent(snapshot.Text)
			item.ContentHash = contentHash(item.CanonicalContent)
			needsSend, stateErr := q.sent.NeedsSend(
				item.Target, item.Topic, item.ContentVersion, item.ContentHash,
			)
			if stateErr != nil {
				err = &deliveryError{code: "sent_state_read_failed", retry: true}
				item.Messages = nil
				item.ExpiresAt = time.Time{}
				item.Topic = ""
				item.ContentVersion = ""
				item.ContentHash = ""
				item.CanonicalContent = ""
			} else if !needsSend {
				item.Status, item.ErrorCode = "skipped", "content_not_updated"
				q.finish(ctx, path, item, start)
				return
			}
			// Freeze the exact body before making a non-idempotent external call.
			if err == nil {
				err = writeJob(path, *item)
			}
			if err != nil {
				slog.ErrorContext(ctx, "checkin notification snapshot save failed", "error", err)
				if _, ok := err.(*deliveryError); !ok {
					return
				}
			}
		} else {
			err = &deliveryError{code: "summary_read_failed", retry: true}
		}
	}
	if err == nil && !now.Before(item.ExpiresAt) {
		item.Status, item.ErrorCode = "skipped", "period_expired"
		q.finish(ctx, path, item, start)
		return
	}
	if err == nil && item.NextPart == len(item.Messages) {
		item.Status = "sent"
		q.finish(ctx, path, item, start)
		return
	}
	item.Attempts++
	if err == nil {
		err = q.sender.SendText(ctx, item.Target, item.Messages[item.NextPart])
	}
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if err == nil {
		item.NextPart++
		item.ErrorCode = ""
		if item.NextPart == len(item.Messages) {
			item.Status = "sent"
		}
	} else {
		item.Status, item.ErrorCode = "failed", "send_failed"
		var failure *deliveryError
		if errors.As(err, &failure) {
			item.ErrorCode = failure.code
			if failure.retry && item.Attempts < 5 {
				item.Status = "pending"
				delay := 10 * time.Second * time.Duration(1<<(item.Attempts-1))
				if failure.retryAfter > delay {
					delay = failure.retryAfter
				}
				item.NextTry = now.Add(delay)
			}
		}
	}
	q.finish(ctx, path, item, start)
}

func (q *Queue) targetsForGroup(groupID uint64) []Target {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Target(nil), q.targets[groupID]...)
}

func (q *Queue) targetsSnapshot() map[uint64][]Target {
	q.mu.Lock()
	defer q.mu.Unlock()
	return cloneTargets(q.targets)
}

func (q *Queue) targetEnabled(groupID uint64, target Target) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, current := range q.targets[groupID] {
		if current == target {
			return true
		}
	}
	return false
}

func cloneTargets(targets map[uint64][]Target) map[uint64][]Target {
	cloned := make(map[uint64][]Target, len(targets))
	for groupID, items := range targets {
		cloned[groupID] = append([]Target(nil), items...)
	}
	return cloned
}

func (q *Queue) finish(ctx context.Context, path string, item *job, start time.Time) {
	attrs := []any{
		"record_id", item.Event.RecordID, "group_id", item.Event.GroupID,
		"initial", item.Event.Initial,
		"attempt", item.Attempts, "next_part", item.NextPart, "status", item.Status,
		"error_code", item.ErrorCode, "duration_ms", time.Since(start).Milliseconds(),
	}
	if item.ErrorCode != "" {
		slog.WarnContext(ctx, "checkin notification delivery", attrs...)
	} else {
		slog.InfoContext(ctx, "checkin notification delivery", attrs...)
	}
	if item.ErrorCode == "" {
		item.Attempts = 0
	}
	if err := writeJob(path, *item); err != nil {
		slog.ErrorContext(ctx, "checkin notification state save failed", "record_id", item.Event.RecordID, "error", err)
		return
	}
	if item.Status == "sent" {
		state := stateFromJob(*item)
		state.SentAt = time.Now().UTC()
		if err := q.sent.Record(state); err != nil {
			slog.ErrorContext(ctx, "sent notification state save failed",
				"record_id", item.Event.RecordID, "group_id", item.Event.GroupID,
				"topic", item.Topic, "error", err)
			return
		}
	}
	if item.Status != "pending" {
		q.archive(ctx, path, item)
	}
}

func (q *Queue) archive(ctx context.Context, path string, item *job) {
	state := "completed"
	if item.Status == "failed" {
		state = "failed"
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := os.Rename(path, filepath.Join(q.dir, state, filepath.Base(path))); err != nil {
		slog.ErrorContext(ctx, "checkin notification archive failed", "record_id", item.Event.RecordID, "error", err)
	}
}

func writeJob(path string, item job) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode notification job: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".notification-*")
	if err != nil {
		return fmt.Errorf("create notification job: %w", err)
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write notification job: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync notification job: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close notification job: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("publish notification job: %w", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open notification directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync notification directory: %w", err)
	}
	return nil
}
