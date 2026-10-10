package notification

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	calls     int
	snapshot  Snapshot
	err       error
	disabled  bool
	policyErr error
}

func (s *fakeSource) Enabled(context.Context, Event) (bool, error) {
	return !s.disabled, s.policyErr
}

func (s *fakeSource) Snapshot(context.Context, Event) (Snapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type fakeSender struct {
	messages []string
	targets  []Target
	err      error
}

func (s *fakeSender) SendText(_ context.Context, target Target, text string) error {
	s.messages = append(s.messages, text)
	s.targets = append(s.targets, target)
	return s.err
}

type blockingSender struct {
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
	mu       sync.Mutex
	messages []string
}

func (s *blockingSender) SendText(_ context.Context, _ Target, text string) error {
	block := false
	s.once.Do(func() {
		block = true
		close(s.started)
	})
	if block {
		<-s.release
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, text)
	return nil
}

func queueFixture(t *testing.T) (*Queue, *fakeSource, *fakeSender, Event, time.Time) {
	t.Helper()
	now := time.Now()
	source := &fakeSource{snapshot: Snapshot{
		Text:      "每日灵修\n1 【新】张三",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-09",
	}}
	sender := &fakeSender{}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{1: {{ChatID: 99, ChatType: 2}}}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	return queue, source, sender, Event{RecordID: 10, GroupID: 1, LogicalDate: "2026-09-09", OccurredAt: now}, now
}

func stateFiles(t *testing.T, queue *Queue, state string) int {
	t.Helper()
	files, err := os.ReadDir(filepath.Join(queue.dir, state))
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func TestClearFailedRemovesOnlyFailedJSONJobs(t *testing.T) {
	t.Parallel()

	queue, _, _, _, _ := queueFixture(t)
	for _, path := range []string{
		filepath.Join(queue.dir, "failed", "one.json"),
		filepath.Join(queue.dir, "failed", "two.json"),
		filepath.Join(queue.dir, "failed", "keep.txt"),
		filepath.Join(queue.dir, "pending", "pending.json"),
		filepath.Join(queue.dir, "completed", "completed.json"),
	} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cleared, err := queue.ClearFailed()
	if err != nil {
		t.Fatal(err)
	}
	if cleared != 2 {
		t.Fatalf("cleared = %d, want 2", cleared)
	}
	for _, path := range []string{
		filepath.Join(queue.dir, "failed", "keep.txt"),
		filepath.Join(queue.dir, "pending", "pending.json"),
		filepath.Join(queue.dir, "completed", "completed.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved file %s: %v", path, err)
		}
	}
	if got := stateFiles(t, queue, "failed"); got != 1 {
		t.Fatalf("failed entries = %d, want non-JSON file only", got)
	}
}

func TestQueuePersistsAndDeduplicates(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	event.LogID = "0123456789abcdef0123456789abcdef"
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if err := queue.Enqueue(event); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if stateFiles(t, queue, "pending") != 1 {
		t.Fatal("duplicate enqueue")
	}
	files, err := os.ReadDir(filepath.Join(queue.dir, "pending"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := files[0].Info()
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("job file permissions = %v, err=%v", info, err)
	}
	payload, err := os.ReadFile(filepath.Join(queue.dir, "pending", files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var persisted job
	if err := json.Unmarshal(payload, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Event.LogID != event.LogID {
		t.Fatalf("persisted log ID = %q, want %q", persisted.Event.LogID, event.LogID)
	}
	restarted, err := NewQueue(queue.dir, queue.targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now)
	if len(sender.messages) != 1 || stateFiles(t, queue, "completed") != 1 {
		t.Fatal("persisted job was not delivered")
	}
	if err := restarted.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.Add(time.Second))
	if len(sender.messages) != 1 {
		t.Fatal("completed event delivered twice")
	}
}

func TestQueueSendsToEveryBoundChat(t *testing.T) {
	t.Parallel()
	now := time.Now()
	source := &fakeSource{snapshot: Snapshot{
		Text:      "每日灵修\n1 张三",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-09",
	}}
	sender := &fakeSender{}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{
		1: {
			{ChatID: 10, ChatType: 2},
			{ChatID: 20, ChatType: 3},
		},
	}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	event := Event{RecordID: 10, GroupID: 1, LogicalDate: "2026-09-09", OccurredAt: now}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	queue.processNext(t.Context(), now.Add(time.Second))
	if len(sender.targets) != 2 || sender.targets[0].ChatID != 10 || sender.targets[1].ChatID != 20 {
		t.Fatalf("targets = %#v", sender.targets)
	}
}

func TestQueueTargetUpdateDoesNotWaitForOtherTargetSend(t *testing.T) {
	t.Parallel()
	now := time.Now()
	sendingTarget := Target{ChatID: 10, ChatType: 3}
	updatedTarget := Target{ChatID: 20, ChatType: 3}
	source := &fakeSource{snapshot: Snapshot{
		Text:      "每日灵修\n1 张三",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-29",
	}}
	sender := &blockingSender{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{
		1: {sendingTarget},
		2: {updatedTarget},
	}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(Event{
		RecordID: 1, GroupID: 1, LogicalDate: "2026-09-29", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	processed := make(chan struct{})
	go func() {
		defer close(processed)
		queue.processNext(t.Context(), now)
	}()
	<-sender.started
	updated := make(chan struct{})
	go func() {
		defer close(updated)
		queue.SetTargets(map[uint64][]Target{
			1: {sendingTarget},
			3: {updatedTarget},
		})
	}()
	select {
	case <-updated:
	case <-time.After(time.Second):
		close(sender.release)
		<-processed
		t.Fatal("unrelated target update waited for active send")
	}
	close(sender.release)
	<-processed
}

func TestQueueRetryFreezesNewMarker(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	sender.err = &deliveryError{code: "potato_1007", retry: true}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	target := queue.targets[event.GroupID][0]
	if _, err := os.Stat(queue.sent.path(event.GroupID, target, "daily")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed delivery advanced sent state: %v", err)
	}
	source.snapshot.Text = "1 张三 视频 2 李四 【新】基督"
	queue.processNext(t.Context(), now.Add(time.Second))
	if len(sender.messages) != 1 {
		t.Fatal("retry backoff was ignored")
	}
	sender.err = nil
	restarted, err := NewQueue(queue.dir, queue.targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.Add(10*time.Second))
	if len(sender.messages) != 2 || sender.messages[0] != sender.messages[1] || source.calls != 1 {
		t.Fatalf("retry changed snapshot: %#v, source calls=%d", sender.messages, source.calls)
	}
}

func TestQueueCapsRetryAfter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		expiresIn   time.Duration
		wantDelay   time.Duration
		wantRetries int
	}{
		{name: "maximum delay", expiresIn: time.Hour, wantDelay: 5 * time.Minute, wantRetries: 1},
		{name: "expiration boundary", expiresIn: 2 * time.Minute, wantDelay: 2 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue, source, sender, event, now := queueFixture(t)
			source.snapshot.ExpiresAt = now.Add(tt.expiresIn)
			sender.err = &deliveryError{code: "http_429", retry: true, retryAfter: 24 * time.Hour}
			if err := queue.Enqueue(event); err != nil {
				t.Fatal(err)
			}
			queue.processNext(t.Context(), now)

			files, err := os.ReadDir(filepath.Join(queue.dir, "pending"))
			if err != nil || len(files) != 1 {
				t.Fatalf("pending files = %d, err=%v", len(files), err)
			}
			data, err := os.ReadFile(filepath.Join(queue.dir, "pending", files[0].Name()))
			if err != nil {
				t.Fatal(err)
			}
			var pending job
			if err := json.Unmarshal(data, &pending); err != nil {
				t.Fatal(err)
			}
			if want := now.Add(tt.wantDelay); !pending.NextTry.Equal(want) {
				t.Fatalf("next try = %v, want %v", pending.NextTry, want)
			}

			sender.err = nil
			queue.processNext(t.Context(), now.Add(tt.wantDelay))
			if got := len(sender.messages) - 1; got != tt.wantRetries {
				t.Fatalf("retries = %d, want %d", got, tt.wantRetries)
			}
		})
	}
}

func TestQueueExpiredRetryDoesNotBlockTarget(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	sender.err = &deliveryError{code: "http_429", retry: true, retryAfter: 24 * time.Hour}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	event.RecordID++
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)

	sender.err = nil
	queue.processNext(t.Context(), now.Add(4*time.Minute))
	if len(sender.messages) != 1 || stateFiles(t, queue, "pending") != 2 {
		t.Fatal("unexpired retry did not preserve target ordering")
	}

	source.snapshot.ExpiresAt = now.Add(3 * time.Hour)
	queue.processNext(t.Context(), now.Add(2*time.Hour))
	if len(sender.messages) != 2 {
		t.Fatalf("expired retry blocked later notification: %#v", sender.messages)
	}
	if stateFiles(t, queue, "pending") != 0 || stateFiles(t, queue, "completed") != 2 {
		t.Fatalf("queue states after expired head: pending=%d completed=%d",
			stateFiles(t, queue, "pending"), stateFiles(t, queue, "completed"))
	}
}

func TestQueueFailurePolicies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		sendErr error
		readErr error
		sends   int
	}{
		{"permanent error", &deliveryError{code: "potato_1002"}, nil, 1},
		{"retry exhausted", &deliveryError{code: "http_503", retry: true}, nil, 5},
		{"database unavailable", nil, errors.New("unavailable"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queue, source, sender, event, now := queueFixture(t)
			source.err, sender.err = tt.readErr, tt.sendErr
			if err := queue.Enqueue(event); err != nil {
				t.Fatal(err)
			}
			for attempt := range 6 {
				queue.processNext(t.Context(), now.Add(time.Duration(attempt)*3*time.Minute))
			}
			if len(sender.messages) != tt.sends || stateFiles(t, queue, "failed") != 1 {
				t.Fatalf("sends=%d, failed=%d", len(sender.messages), stateFiles(t, queue, "failed"))
			}
		})
	}
}

type observingSource struct {
	*fakeSource
	events  []Event
	codes   []string
	reasons []string
	panics  bool
}

func (s *observingSource) ReportNotificationFailure(event Event, code, reason string) {
	s.events = append(s.events, event)
	s.codes = append(s.codes, code)
	s.reasons = append(s.reasons, reason)
	if s.panics {
		panic("observer failed")
	}
}

func TestQueueReportsOnlyTerminalFailureWithoutChangingDelivery(t *testing.T) {
	for _, name := range []string{"permanent", "exhausted", "observer panic", "transport", "success", "disabled"} {
		t.Run(name, func(t *testing.T) {
			queue, source, sender, event, now := queueFixture(t)
			observer := &observingSource{fakeSource: source, panics: name == "observer panic"}
			queue.source = observer
			event.LogID = "0123456789abcdef0123456789abcdef"
			if name == "disabled" {
				source.disabled = true
			} else if name != "success" {
				sender.err = &deliveryError{code: "http_400", retry: name == "exhausted"}
			}
			if name == "transport" {
				sender.err = &deliveryError{code: "transport_failed", reason: "timeout"}
			}
			if err := queue.Enqueue(event); err != nil {
				t.Fatal(err)
			}
			queue.processNext(t.Context(), now)
			if name == "exhausted" {
				if len(observer.events) != 0 || stateFiles(t, queue, "pending") != 1 {
					t.Fatal("retryable failure was reported or archived prematurely")
				}
				for attempt := 1; attempt < 6; attempt++ {
					queue.processNext(t.Context(), now.Add(time.Duration(attempt)*3*time.Minute))
				}
			}
			if name == "success" || name == "disabled" {
				if len(observer.events) != 0 || stateFiles(t, queue, "completed") != 1 {
					t.Fatal("successful/skipped notification changed")
				}
				return
			}
			if name == "transport" && (len(observer.reasons) != 1 || observer.reasons[0] != "timeout") {
				t.Fatalf("transport reason lost: %v", observer.reasons)
			}
			expectedCode := "http_400"
			if name == "transport" {
				expectedCode = "transport_failed"
			}
			if len(observer.events) != 1 || observer.events[0].RecordID != event.RecordID ||
				observer.events[0].GroupID != event.GroupID || observer.events[0].LogID != event.LogID ||
				!observer.events[0].OccurredAt.Equal(event.OccurredAt) || observer.codes[0] != expectedCode ||
				stateFiles(t, queue, "failed") != 1 {
				t.Fatalf("events=%+v codes=%v failed=%d", observer.events, observer.codes, stateFiles(t, queue, "failed"))
			}
		})
	}
}

func TestQueueSkipsIneligibleExpiredAndChangedTargets(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ineligible", "expired", "changed", "unconfigured"} {
		t.Run(name, func(t *testing.T) {
			queue, source, sender, event, now := queueFixture(t)
			switch name {
			case "ineligible":
				source.snapshot.Text = ""
			case "expired":
				source.snapshot.ExpiresAt = now
			case "unconfigured":
				event.GroupID = 2
			}
			if err := queue.Enqueue(event); err != nil {
				t.Fatal(err)
			}
			if name == "changed" {
				queue.SetTargets(map[uint64][]Target{1: {{ChatID: 100, ChatType: 3}}})
			}
			queue.processNext(t.Context(), now)
			if len(sender.messages) != 0 || stateFiles(t, queue, "pending") != 0 {
				t.Fatal("ineligible notification sent or retained")
			}
		})
	}
}

func TestQueueResumesAtFailedChunk(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	source.snapshot.Text = strings.Repeat("1 张三 【新】视频 ", 250)
	wantParts := splitMessage(source.snapshot.Text)
	if len(wantParts) < 2 {
		t.Fatal("fixture must contain multiple parts")
	}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	sender.err = &deliveryError{code: "http_503", retry: true}
	queue.processNext(t.Context(), now.Add(time.Second))
	sender.err = nil
	restarted, err := NewQueue(queue.dir, queue.targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	for i := range len(wantParts) {
		restarted.processNext(t.Context(), now.Add(time.Duration(30+i)*time.Second))
	}
	if len(sender.messages) != len(wantParts)+1 || sender.messages[1] != sender.messages[2] {
		t.Fatalf("chunk retry messages = %#v", sender.messages)
	}
	if stateFiles(t, queue, "completed") != 1 {
		t.Fatal("chunked delivery did not complete")
	}
}

func TestQueueRunStops(t *testing.T) {
	t.Parallel()
	queue, _, _, _, _ := queueFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		queue.Run(ctx)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestQueueHonorsNotificationSwitch(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"disabled before send", "disabled during retry", "settings unavailable"} {
		t.Run(name, func(t *testing.T) {
			queue, source, sender, event, now := queueFixture(t)
			if err := queue.Enqueue(event); err != nil {
				t.Fatal(err)
			}
			wantSends := 0
			if name == "disabled during retry" {
				sender.err = &deliveryError{code: "http_503", retry: true}
				queue.processNext(t.Context(), now)
				sender.err = nil
				wantSends = 1
			}
			source.disabled = true
			if name == "settings unavailable" {
				source.policyErr = errors.New("database unavailable")
			}
			queue.processNext(t.Context(), now.Add(time.Minute))
			if len(sender.messages) != wantSends {
				t.Fatal("notification was sent while disabled or settings unavailable")
			}
			if name == "settings unavailable" {
				if stateFiles(t, queue, "pending") != 1 {
					t.Fatal("settings failure should retry")
				}
			} else if stateFiles(t, queue, "completed") != 1 {
				t.Fatal("disabled notification should be archived")
			}
		})
	}
}

func TestQueueRearmsInitialProgressWhenNotificationIsEnabled(t *testing.T) {
	t.Parallel()
	now := time.Now()
	source := &fakeSource{
		disabled: true,
		snapshot: Snapshot{
			Text:      "每日灵修\n1 张三",
			ExpiresAt: now.Add(time.Hour),
			Topic:     "daily",
			Version:   "daily:2026-09-09",
		},
	}
	sender := &fakeSender{}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{
		1: {{ChatID: 99, ChatType: 3}},
	}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueInitialBinding(1, Target{ChatID: 99, ChatType: 3}, now); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	if stateFiles(t, queue, "completed") != 1 {
		t.Fatal("disabled initial progress was not archived")
	}
	source.disabled = false
	if err := queue.WakeInitial(1, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now.Add(time.Minute))
	if len(sender.messages) != 1 || sender.messages[0] != source.snapshot.Text {
		t.Fatalf("messages = %#v", sender.messages)
	}
}

func TestQueueWakeInitialRefreshesPendingRetry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	target := Target{ChatID: 99, ChatType: 3}
	source := &fakeSource{snapshot: Snapshot{
		Text:      "旧的每日灵修",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-29",
	}}
	sender := &fakeSender{err: &deliveryError{code: "http_503", retry: true}}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{1: {target}}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueInitialBinding(1, target, now); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)

	source.snapshot.Text = "最新的每日灵修"
	sender.err = nil
	if err := queue.WakeInitial(1, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewQueue(queue.dir, queue.targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.Add(time.Second))
	if len(sender.messages) != 2 || sender.messages[1] != source.snapshot.Text {
		t.Fatalf("messages = %#v, want retry with latest snapshot", sender.messages)
	}
}

func TestQueueWakeInitialDuringSentStateSaveRemainsPending(t *testing.T) {
	t.Parallel()
	now := time.Now()
	target := Target{ChatID: 99, ChatType: 3}
	source := &fakeSource{snapshot: Snapshot{
		Text:      "旧的每日灵修",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-29",
	}}
	sender := &blockingSender{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	queue, err := NewQueue(t.TempDir(), map[uint64][]Target{1: {target}}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueInitialBinding(1, target, now); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		queue.processNext(t.Context(), now)
	}()
	<-sender.started

	queue.sent.mu.Lock()
	close(sender.release)
	dailyPath := filepath.Join(
		queue.dir,
		"pending",
		"00000000000000000000-initial-00000000000000000001-99-3-daily.json",
	)
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, readErr := os.ReadFile(dailyPath)
		var item job
		if readErr == nil && json.Unmarshal(data, &item) == nil && item.Status == "sent" {
			break
		}
		if time.Now().After(deadline) {
			queue.sent.mu.Unlock()
			t.Fatal("daily job did not reach sent-state save window")
		}
		time.Sleep(time.Millisecond)
	}
	if err := queue.WakeInitial(1, now.Add(time.Minute)); err != nil {
		queue.sent.mu.Unlock()
		t.Fatal(err)
	}
	queue.sent.mu.Unlock()
	<-done

	data, err := os.ReadFile(dailyPath)
	if err != nil {
		t.Fatalf("daily wake was lost: %v", err)
	}
	var pending job
	if err := json.Unmarshal(data, &pending); err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending" || len(pending.Messages) != 0 {
		t.Fatalf("daily wake state = status %q messages %#v", pending.Status, pending.Messages)
	}

	source.snapshot.Text = "最新的每日灵修"
	queue.processNext(t.Context(), now.Add(time.Minute))
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.messages) != 2 || sender.messages[1] != source.snapshot.Text {
		t.Fatalf("messages = %#v, want wake to enqueue latest snapshot", sender.messages)
	}
}

func TestQueueSkipsOlderNotificationVersion(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	target := queue.targets[event.GroupID][0]
	if err := queue.sent.Record(sentState{
		GroupID: event.GroupID,
		Target:  target,
		Topic:   "daily",
		Version: "daily:2026-09-10",
		Hash:    contentHash("每日灵修\n1 张三\n2 李四"),
		Content: "每日灵修\n1 张三\n2 李四",
		SentAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	source.snapshot.Text = "每日灵修\n1 张三"
	source.snapshot.Version = "daily:2026-09-09"
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	if len(sender.messages) != 0 {
		t.Fatalf("older notification sent: %#v", sender.messages)
	}
}

func TestQueueRechecksSentStateAfterReadFailure(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	target := queue.targets[event.GroupID][0]
	statePath := queue.sent.path(event.GroupID, target, "daily")
	if err := os.WriteFile(statePath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	if len(sender.messages) != 0 {
		t.Fatal("notification sent before last state was readable")
	}
	content := canonicalNotificationContent(source.snapshot.Text)
	if err := queue.sent.Record(sentState{
		GroupID: event.GroupID,
		Target:  target,
		Topic:   "daily",
		Version: source.snapshot.Version,
		Hash:    contentHash(content),
		Content: content,
		SentAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now.Add(11*time.Second))
	if len(sender.messages) != 0 {
		t.Fatalf("unchanged notification sent after state recovery: %#v", sender.messages)
	}
}
