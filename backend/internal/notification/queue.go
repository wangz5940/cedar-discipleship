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
}

type TextSender interface {
	SendText(context.Context, Target, string) error
}

type Queue struct {
	dir     string
	targets map[uint64]Target
	source  SnapshotSource
	sender  TextSender
	mu      sync.Mutex
}

type job struct {
	Event     Event     `json:"event"`
	Target    Target    `json:"target"`
	Messages  []string  `json:"messages,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	NextPart  int       `json:"next_part"`
	Attempts  int       `json:"attempts"`
	NextTry   time.Time `json:"next_try"`
	Status    string    `json:"status"`
	ErrorCode string    `json:"error_code,omitempty"`
}

func NewQueue(dir string, targets map[uint64]Target, source SnapshotSource, sender TextSender) (*Queue, error) {
	for _, state := range []string{"pending", "completed", "failed"} {
		if err := os.MkdirAll(filepath.Join(dir, state), 0o700); err != nil {
			return nil, fmt.Errorf("create notification queue: %w", err)
		}
	}
	copyTargets := make(map[uint64]Target, len(targets))
	for id, target := range targets {
		copyTargets[id] = target
	}
	return &Queue{dir: dir, targets: copyTargets, source: source, sender: sender}, nil
}

func (q *Queue) Enqueue(event Event) error {
	target, ok := q.targets[event.GroupID]
	if !ok {
		return nil
	}
	if _, err := time.Parse("2006-01-02", event.LogicalDate); err != nil || event.RecordID == 0 || event.Initial != "" {
		return errors.New("invalid notification event")
	}
	name := fmt.Sprintf("%020d-%020d-%s.json", event.RecordID, event.GroupID, event.LogicalDate)
	return q.enqueue(name, event, target)
}

// EnqueueInitial persists one job per topic and binding, independent of restarts.
func (q *Queue) EnqueueInitial(now time.Time) error {
	for groupID, target := range q.targets {
		for _, topic := range []string{"daily", "weekly"} {
			name := fmt.Sprintf("%020d-initial-%020d-%d-%d-%s.json",
				0, groupID, target.ChatID, target.ChatType, topic)
			event := Event{GroupID: groupID, OccurredAt: now, Initial: topic}
			if err := q.enqueue(name, event, target); err != nil {
				return err
			}
		}
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
		q.archive(ctx, path, item)
		return
	}
	target, enabled := q.targets[item.Event.GroupID]
	if !enabled || target != item.Target {
		item.Status = "skipped"
		item.ErrorCode = "target_changed"
		q.finish(ctx, path, item, start)
		return
	}
	var err error
	if len(item.Messages) == 0 {
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
			// Freeze the exact body before making a non-idempotent external call.
			if err := writeJob(path, *item); err != nil {
				slog.ErrorContext(ctx, "checkin notification snapshot save failed", "error", err)
				return
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
