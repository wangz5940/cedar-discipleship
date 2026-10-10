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

	"agp/backend/internal/logctx"
)

const maxRetryDelay = 5 * time.Minute

type SnapshotSource interface {
	Snapshot(context.Context, Event) (Snapshot, error)
	Enabled(context.Context, Event) (bool, error)
}

type TextSender interface {
	SendText(context.Context, Target, string) error
}

// FailureReporter observes terminal failures without changing delivery state.
// Implementations must enqueue work without blocking the notification queue.
type FailureReporter interface {
	ReportNotificationFailure(Event, string, string)
}

type Queue struct {
	dir         string
	targets     map[uint64][]Target
	source      SnapshotSource
	sender      TextSender
	sent        *sentStateStore
	unbindMuted func(Target, uint64, uint64) error
	mu          sync.Mutex

	// Target leases serialize binding publication with only that target's external send.
	targetUpdates  sync.Mutex
	targetVersions map[Target]uint64
	targetLeases   map[Target]*sync.RWMutex
}

type QueueStats struct {
	Pending   int `json:"pending"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
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
	ErrorReason      string    `json:"error_reason,omitempty"`
	Topic            string    `json:"topic,omitempty"`
	ContentVersion   string    `json:"content_version,omitempty"`
	PeriodID         string    `json:"period_id,omitempty"`
	ContentHash      string    `json:"content_hash,omitempty"`
	CanonicalContent string    `json:"canonical_content,omitempty"`
	CoveredRecordID  uint64    `json:"covered_record_id,omitempty"`
	SentAt           time.Time `json:"sent_at,omitempty"`
	RefreshAt        time.Time `json:"refresh_at,omitempty"`
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
	versions := make(map[Target]uint64)
	for _, targets := range targets {
		for _, target := range targets {
			versions[target] = 1
		}
	}
	return &Queue{
		dir: dir, targets: cloneTargets(targets), source: source, sender: sender, sent: sent,
		targetVersions: versions, targetLeases: make(map[Target]*sync.RWMutex),
	}, nil
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
	q.targetUpdates.Lock()
	defer q.targetUpdates.Unlock()

	targets = cloneTargets(targets)
	q.mu.Lock()
	previous := groupsByTarget(q.targets)
	next := groupsByTarget(targets)
	changed := changedTargets(previous, next)
	leases := make([]*sync.RWMutex, len(changed))
	for index, target := range changed {
		leases[index] = q.targetLeaseLocked(target)
	}
	q.mu.Unlock()

	for _, lease := range leases {
		lease.Lock()
	}
	defer func() {
		for index := len(leases) - 1; index >= 0; index-- {
			leases[index].Unlock()
		}
	}()

	q.mu.Lock()
	for _, target := range changed {
		q.targetVersions[target]++
		if q.targetVersions[target] == 0 {
			q.targetVersions[target] = 1
		}
	}
	q.targets = targets
	q.mu.Unlock()
}

func (q *Queue) Stats() (QueueStats, error) {
	var stats QueueStats
	for state, target := range map[string]*int{
		"pending":   &stats.Pending,
		"completed": &stats.Completed,
		"failed":    &stats.Failed,
	} {
		entries, err := os.ReadDir(filepath.Join(q.dir, state))
		if err != nil {
			return QueueStats{}, fmt.Errorf("read %s notification queue: %w", state, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				(*target)++
			}
		}
	}
	return stats, nil
}

func (q *Queue) ClearFailed() (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	dir := filepath.Join(q.dir, "failed")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read failed notification queue: %w", err)
	}
	cleared := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return cleared, fmt.Errorf("remove failed notification: %w", err)
		}
		cleared++
	}
	return cleared, nil
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
	pending := filepath.Join(q.dir, "pending", name)
	data, err := os.ReadFile(pending)
	if err == nil {
		var item job
		if err := json.Unmarshal(data, &item); err != nil {
			return fmt.Errorf("decode pending initial notification: %w", err)
		}
		if item.Event.Initial != "" && now.After(item.RefreshAt) {
			item.RefreshAt = now
			if item.Status == "pending" {
				item.NextTry = time.Time{}
			}
			return writeJob(pending, item)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check pending initial notification: %w", err)
	}
	var sourcePath string
	data = nil
	for _, state := range []string{"completed", "failed"} {
		path := filepath.Join(q.dir, state, name)
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
	resetInitialJob(&item, now)
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
	ctx := logctx.WithLogID(context.Background(), event.LogID)
	slog.InfoContext(ctx, "checkin notification queued", "record_id", event.RecordID, "group_id", event.GroupID,
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
		itemContext := logctx.WithLogID(ctx, item.Event.LogID)
		refreshPending := item.Event.Initial != "" && !item.RefreshAt.IsZero()
		if item.Status == "pending" && !refreshPending &&
			!item.ExpiresAt.IsZero() && !now.Before(item.ExpiresAt) {
			item.Status, item.ErrorCode = "skipped", "period_expired"
			q.finish(itemContext, path, &item, time.Now())
			continue
		}
		if blocked[item.Target] {
			continue
		}
		blocked[item.Target] = true
		// Preserve ordering within a chat while allowing other chats to progress.
		if item.Status == "pending" && now.Before(item.NextTry) {
			continue
		}
		q.process(itemContext, path, &item, now)
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
		} else {
			q.finalize(ctx, path, item)
		}
		return
	}
	if err := q.prepareInitialRefresh(path, item); err != nil {
		slog.ErrorContext(ctx, "initial notification refresh failed", "error", err)
		return
	}
	targetVersion, targetEnabled := q.targetVersion(item.Event.GroupID, item.Target)
	if !targetEnabled {
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
	freshSnapshot := len(item.Messages) == 0
	if err == nil && freshSnapshot {
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
			item.PeriodID = snapshot.PeriodID
			item.CanonicalContent = canonicalNotificationContent(snapshot.Text)
			item.ContentHash = contentHash(item.CanonicalContent)
			item.CoveredRecordID = snapshot.CoveredRecordID
		} else {
			err = &deliveryError{code: "summary_read_failed", retry: true}
		}
	}
	if err == nil {
		// A full snapshot can supersede an event even between its message parts.
		needsSend, stateErr := q.sent.NeedsSend(stateFromJob(*item), item.Event.RecordID)
		if stateErr != nil {
			err = &deliveryError{code: "sent_state_read_failed", retry: true}
			if freshSnapshot {
				item.Messages = nil
				item.ExpiresAt = time.Time{}
				item.Topic = ""
				item.ContentVersion = ""
				item.PeriodID = ""
				item.ContentHash = ""
				item.CanonicalContent = ""
				item.CoveredRecordID = 0
			}
		} else if !needsSend {
			item.Status, item.ErrorCode = "skipped", "content_not_updated"
			q.finish(ctx, path, item, start)
			return
		}
	}
	// Freeze the exact body before making a non-idempotent external call.
	if err == nil && freshSnapshot {
		if err := q.saveJob(path, item); err != nil {
			slog.ErrorContext(ctx, "checkin notification snapshot save failed", "error", err)
			return
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
	if err == nil {
		releaseTarget, current := q.acquireTargetLease(item.Event.GroupID, item.Target, targetVersion)
		if !current {
			item.Status = "skipped"
			item.ErrorCode = "target_changed"
			q.finish(ctx, path, item, start)
			return
		}
		item.Attempts++
		err = q.sender.SendText(ctx, item.Target, item.Messages[item.NextPart])
		releaseTarget()
	} else {
		item.Attempts++
	}
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	if err == nil {
		item.NextPart++
		item.ErrorCode = ""
		item.ErrorReason = ""
		if item.NextPart == len(item.Messages) {
			item.Status = "sent"
		}
	} else {
		item.Status, item.ErrorCode = "failed", "send_failed"
		item.ErrorReason = ""
		var failure *deliveryError
		if errors.As(err, &failure) {
			item.ErrorCode = failure.code
			item.ErrorReason = failure.reason
			if failure.code == "potato_3023" && q.unbindMuted != nil {
				if err := q.unbindMuted(item.Target, item.Event.GroupID, targetVersion); err != nil {
					slog.ErrorContext(ctx, "muted notification binding removal failed",
						"group_id", item.Event.GroupID, "chat_id", item.Target.ChatID, "error", err)
				}
			}
			if failure.retry && item.Attempts < 5 {
				item.Status = "pending"
				delay := 10 * time.Second * time.Duration(1<<(item.Attempts-1))
				if failure.retryAfter > delay {
					delay = failure.retryAfter
				}
				if delay > maxRetryDelay {
					delay = maxRetryDelay
				}
				item.NextTry = now.Add(delay)
				if !item.ExpiresAt.IsZero() && item.NextTry.After(item.ExpiresAt) {
					item.NextTry = item.ExpiresAt
				}
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

func (q *Queue) targetVersion(groupID uint64, target Target) (uint64, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, current := range q.targets[groupID] {
		if current == target {
			return q.targetVersions[target], true
		}
	}
	return 0, false
}

func (q *Queue) acquireTargetLease(groupID uint64, target Target, version uint64) (func(), bool) {
	q.mu.Lock()
	lease := q.targetLeaseLocked(target)
	q.mu.Unlock()

	lease.RLock()
	currentVersion, enabled := q.targetVersion(groupID, target)
	if !enabled || currentVersion != version {
		lease.RUnlock()
		return nil, false
	}
	return lease.RUnlock, true
}

func (q *Queue) targetLeaseLocked(target Target) *sync.RWMutex {
	lease := q.targetLeases[target]
	if lease == nil {
		lease = &sync.RWMutex{}
		q.targetLeases[target] = lease
	}
	return lease
}

func groupsByTarget(targets map[uint64][]Target) map[Target]uint64 {
	groups := make(map[Target]uint64)
	for groupID, groupTargets := range targets {
		for _, target := range groupTargets {
			groups[target] = groupID
		}
	}
	return groups
}

func changedTargets(previous, next map[Target]uint64) []Target {
	changed := make([]Target, 0)
	for target, previousGroupID := range previous {
		if next[target] != previousGroupID {
			changed = append(changed, target)
		}
	}
	for target, nextGroupID := range next {
		if previous[target] == 0 && nextGroupID != 0 {
			changed = append(changed, target)
		}
	}
	return changed
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
		"error_code", item.ErrorCode, "transport_reason", item.ErrorReason, "duration_ms", time.Since(start).Milliseconds(),
	}
	if item.ErrorCode != "" {
		slog.WarnContext(ctx, "checkin notification delivery", attrs...)
	} else {
		slog.InfoContext(ctx, "checkin notification delivery", attrs...)
	}
	if item.ErrorCode == "" {
		item.Attempts = 0
	}
	if item.Status == "sent" && item.SentAt.IsZero() {
		item.SentAt = time.Now().UTC()
	}
	if err := q.saveJob(path, item); err != nil {
		slog.ErrorContext(ctx, "checkin notification state save failed", "record_id", item.Event.RecordID, "error", err)
		return
	}
	if item.Status == "sent" {
		state := stateFromJob(*item)
		if err := q.sent.Record(state); err != nil {
			slog.ErrorContext(ctx, "sent notification state save failed",
				"record_id", item.Event.RecordID, "group_id", item.Event.GroupID,
				"topic", item.Topic, "error", err)
			return
		}
	}
	if item.Status != "pending" {
		if item.Status == "failed" {
			q.reportFailure(item.Event, item.ErrorCode, item.ErrorReason)
		}
		q.finalize(ctx, path, item)
	}
}

func (q *Queue) reportFailure(event Event, code, reason string) {
	defer func() {
		if recover() != nil {
			slog.Error("notification failure observer panicked")
		}
	}()
	if reporter, ok := q.source.(FailureReporter); ok {
		reporter.ReportNotificationFailure(event, code, reason)
	}
}

func (q *Queue) prepareInitialRefresh(path string, item *job) error {
	if item.Event.Initial == "" || item.NextPart != 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := mergeInitialRefresh(path, item); err != nil {
		return err
	}
	if item.RefreshAt.IsZero() {
		return nil
	}
	resetInitialJob(item, item.RefreshAt)
	return writeJob(path, *item)
}

func (q *Queue) saveJob(path string, item *job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := mergeInitialRefresh(path, item); err != nil {
		return err
	}
	return writeJob(path, *item)
}

func (q *Queue) finalize(ctx context.Context, path string, item *job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := mergeInitialRefresh(path, item); err != nil {
		slog.ErrorContext(ctx, "initial notification refresh merge failed",
			"record_id", item.Event.RecordID, "error", err)
		return
	}
	if item.Event.Initial != "" && !item.RefreshAt.IsZero() {
		resetInitialJob(item, item.RefreshAt)
		if err := writeJob(path, *item); err != nil {
			slog.ErrorContext(ctx, "initial notification requeue failed",
				"group_id", item.Event.GroupID, "initial", item.Event.Initial, "error", err)
		}
		return
	}
	state := "completed"
	if item.Status == "failed" {
		state = "failed"
	}
	if err := os.Rename(path, filepath.Join(q.dir, state, filepath.Base(path))); err != nil {
		slog.ErrorContext(ctx, "checkin notification archive failed", "record_id", item.Event.RecordID, "error", err)
	}
}

func mergeInitialRefresh(path string, item *job) error {
	if item.Event.Initial == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read current initial notification: %w", err)
	}
	var current job
	if err := json.Unmarshal(data, &current); err != nil {
		return fmt.Errorf("decode current initial notification: %w", err)
	}
	if current.RefreshAt.After(item.RefreshAt) {
		item.RefreshAt = current.RefreshAt
	}
	return nil
}

func resetInitialJob(item *job, now time.Time) {
	item.Event.OccurredAt = now
	item.Messages = nil
	item.ExpiresAt = time.Time{}
	item.NextPart = 0
	item.Attempts = 0
	item.NextTry = time.Time{}
	item.Status = "pending"
	item.ErrorCode = ""
	item.ErrorReason = ""
	item.Topic = ""
	item.ContentVersion = ""
	item.PeriodID = ""
	item.ContentHash = ""
	item.CanonicalContent = ""
	item.CoveredRecordID = 0
	item.SentAt = time.Time{}
	item.RefreshAt = time.Time{}
}

func writeJob(path string, item job) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode notification job: %w", err)
	}
	if err := writeAtomic(path, data); err != nil {
		return fmt.Errorf("write notification job: %w", err)
	}
	return nil
}
