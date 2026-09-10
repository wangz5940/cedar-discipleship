package notification

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
