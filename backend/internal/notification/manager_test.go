package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type blockingSnapshotSource struct {
	snapshot Snapshot
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (*blockingSnapshotSource) Enabled(context.Context, Event) (bool, error) {
	return true, nil
}

func (s *blockingSnapshotSource) Snapshot(ctx context.Context, _ Event) (Snapshot, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
		return s.snapshot, nil
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	}
}

func TestManagerAssignsEachChatToOneStudyGroup(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"ok":true,
			"result":{"Groups":[],"SuperGroups":[{"PeerID":20,"PeerName":"2026 bible study"}]}
		}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL
	manager, err := NewManager(
		t.TempDir(),
		nil,
		&fakeSource{snapshot: Snapshot{Text: "current", ExpiresAt: time.Now().Add(time.Hour)}},
		client,
	)
	if err != nil {
		t.Fatal(err)
	}
	target := Target{ChatID: 20, ChatType: 3}
	if err := manager.Assign(t.Context(), target, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Assign(t.Context(), target, 2, time.Now()); err != nil {
		t.Fatal(err)
	}
	bindings := manager.Bindings()
	if len(bindings) != 1 || bindings[0].GroupID != 2 || bindings[0].Target != target {
		t.Fatalf("bindings = %#v", bindings)
	}
	if got := manager.store.Targets(); len(got[1]) != 0 || len(got[2]) != 1 {
		t.Fatalf("targets = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(manager.queue.dir, "pending")); err != nil {
		t.Fatal(err)
	}
}

func TestManagerRebindPreventsOldGroupSend(t *testing.T) {
	t.Parallel()
	now := time.Now()
	target := Target{ChatID: 20, ChatType: 3}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"ok":true,
			"result":{"Groups":[],"SuperGroups":[{"PeerID":20,"PeerName":"target"}]}
		}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL
	source := &blockingSnapshotSource{
		snapshot: Snapshot{
			Text:      "旧小组进度",
			ExpiresAt: now.Add(time.Hour),
			Topic:     "daily",
			Version:   "daily:2026-09-29",
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager, err := NewManager(t.TempDir(), map[uint64]Target{1: target}, source, client)
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{}
	manager.queue.sender = sender
	if err := manager.Enqueue(Event{
		RecordID: 1, GroupID: 1, LogicalDate: "2026-09-29", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	processed := make(chan struct{})
	go func() {
		defer close(processed)
		manager.queue.processNext(t.Context(), now)
	}()
	<-source.started
	assignErr := manager.Assign(t.Context(), target, 2, now)
	if assignErr == nil {
		assignErr = manager.Assign(t.Context(), target, 1, now)
	}
	close(source.release)
	<-processed
	if assignErr != nil {
		t.Fatal(assignErr)
	}
	if len(sender.messages) != 0 {
		t.Fatalf("old group sent after rebind: %#v", sender.messages)
	}
}

func TestManagerAssignWaitsForActiveTargetSend(t *testing.T) {
	t.Parallel()
	now := time.Now()
	target := Target{ChatID: 20, ChatType: 3}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{
			"ok":true,
			"result":{"Groups":[],"SuperGroups":[{"PeerID":20,"PeerName":"target"}]}
		}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL
	source := &fakeSource{snapshot: Snapshot{
		Text:      "旧小组进度",
		ExpiresAt: now.Add(time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-29",
	}}
	manager, err := NewManager(t.TempDir(), map[uint64]Target{1: target}, source, client)
	if err != nil {
		t.Fatal(err)
	}
	sender := &blockingSender{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	manager.queue.sender = sender
	if err := manager.Enqueue(Event{
		RecordID: 1, GroupID: 1, LogicalDate: "2026-09-29", OccurredAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	processed := make(chan struct{})
	go func() {
		defer close(processed)
		manager.queue.processNext(t.Context(), now)
	}()
	<-sender.started
	assigned := make(chan error, 1)
	go func() {
		assigned <- manager.Assign(t.Context(), target, 2, now)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		bindings := manager.store.Bindings()
		if len(bindings) == 1 && bindings[0].GroupID == 2 {
			break
		}
		if time.Now().After(deadline) {
			close(sender.release)
			<-processed
			t.Fatal("binding store was not updated")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case err := <-assigned:
		close(sender.release)
		<-processed
		t.Fatalf("Assign returned before active send completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(sender.release)
	<-processed
	if err := <-assigned; err != nil {
		t.Fatal(err)
	}
}

func TestManagerRejectsChatOutsideBotGroups(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true,"result":{"Groups":[],"SuperGroups":[]}}`)
	}))
	defer server.Close()
	client, err := NewPotatoClient("123:secret")
	if err != nil {
		t.Fatal(err)
	}
	client.groupsEndpoint = server.URL
	manager, err := NewManager(t.TempDir(), nil, &fakeSource{}, client)
	if err != nil {
		t.Fatal(err)
	}
	err = manager.Assign(t.Context(), Target{ChatID: 20, ChatType: 3}, 1, time.Now())
	if !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("Assign() error = %v, want ErrChatNotFound", err)
	}
}
