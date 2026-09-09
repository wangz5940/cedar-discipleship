package notification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	calls    int
	snapshot Snapshot
	err      error
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

func queueFixture(t *testing.T) (*Queue, *fakeSource, *fakeSender, Event, time.Time) {
	t.Helper()
	now := time.Now()
	source := &fakeSource{snapshot: Snapshot{Text: "1 张三 【新】视频", ExpiresAt: now.Add(time.Hour)}}
	sender := &fakeSender{}
	queue, err := NewQueue(t.TempDir(), map[uint64]Target{1: {ChatID: 99, ChatType: 2}}, source, sender)
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

func TestQueuePersistsAndDeduplicates(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
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

func TestQueueRetryFreezesNewMarker(t *testing.T) {
	t.Parallel()
	queue, source, sender, event, now := queueFixture(t)
	sender.err = &deliveryError{code: "potato_1007", retry: true}
	if err := queue.Enqueue(event); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
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
				queue.targets[1] = Target{ChatID: 100, ChatType: 3}
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
