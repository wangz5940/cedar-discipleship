package notification

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type stoppingSource struct {
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (s *stoppingSource) Enabled(ctx context.Context, _ Event) (bool, error) {
	close(s.started)
	<-ctx.Done()
	close(s.canceled)
	<-s.release
	return false, ctx.Err()
}

func (*stoppingSource) Snapshot(context.Context, Event) (Snapshot, error) {
	return Snapshot{}, nil
}

func TestFleetRemoveWaitsForWorkerWithoutBlockingFleet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		newClient := newPotatoClient
		defer func() { newPotatoClient = newClient }()
		newPotatoClient = func(token string) (*PotatoClient, error) {
			client, err := newClient(token)
			if err == nil {
				client.client = &http.Client{Transport: robotReadTransport{}}
			}
			return client, err
		}
		source := &stoppingSource{
			started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{}),
		}
		dir := t.TempDir()
		store := NewRobotConfigStore(filepath.Join(dir, "robots.json"))
		if err := store.Save([]RobotConfig{{
			ID: "secondary", Name: "secondary", Token: "123:secret",
		}}); err != nil {
			t.Fatal(err)
		}
		if _, err := NewBindingStore(
			filepath.Join(dir, "robots", "secondary", "bindings.json"),
			map[uint64]Target{1: {ChatID: 99, ChatType: 3}},
		); err != nil {
			t.Fatal(err)
		}
		fleet, err := NewFleet(dir, nil, source)
		if err != nil {
			t.Fatal(err)
		}
		robot := fleet.robots["secondary"]
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := sync.OnceFunc(func() { close(source.release) })
		defer release()
		go fleet.Run(ctx)
		if err := fleet.EnqueueInitial(time.Now()); err != nil {
			t.Fatal(err)
		}
		<-source.started
		removed := make(chan error, 1)
		go func() { removed <- fleet.Remove("secondary") }()
		synctest.Wait()
		select {
		case err := <-removed:
			t.Fatalf("Remove returned before worker exit: %v", err)
		default:
		}
		select {
		case <-source.canceled:
		default:
			t.Fatal("Remove did not cancel the worker")
		}
		// Removal must keep the ID reserved, but release the fleet lock.
		fleet.mu.RLock()
		reserved := fleet.robots["secondary"] != nil
		fleet.mu.RUnlock()
		if !reserved {
			t.Fatal("worker directory can be reused before exit")
		}
		if len(fleet.robotList()) != 0 {
			t.Fatal("removing robot still accepts fleet work")
		}
		if err := robot.manager.Assign(t.Context(), Target{ChatID: 99, ChatType: 3}, 0, time.Now()); err != ErrRobotNotFound {
			t.Fatalf("stale manager accepted assignment: %v", err)
		}
		req := RobotRegistration{ID: "secondary", Name: "secondary", Token: "123:secret"}
		if _, err := fleet.Register(t.Context(), req); err != ErrRobotAlreadyExists {
			t.Fatalf("registration reused directory before worker exit: %v", err)
		}
		release()
		if err := <-removed; err != nil {
			t.Fatal(err)
		}
		if ctx.Err() != nil {
			t.Fatal("removing one robot canceled the fleet")
		}
		fleet.source = &fakeSource{}
		if _, err := fleet.Register(t.Context(), req); err != nil {
			t.Fatalf("registration after worker exit: %v", err)
		}
		fleet.mu.RLock()
		replacement := fleet.robots["secondary"]
		fleet.mu.RUnlock()
		if replacement == robot || !replacement.running {
			t.Fatal("replacement worker was not started")
		}
	})
}

type robotReadTransport struct{}

func (robotReadTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `{"ok":true,"result":{"Groups":[],"SuperGroups":[]}}`
	if strings.HasSuffix(request.URL.Path, "/getMe") {
		body = `{"ok":true,"result":{"id":1,"first_name":"Bot","username":"secondary"}}`
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestManagerSerializesBindingPublication(t *testing.T) {
	one, two := Target{ChatID: 1, ChatType: 3}, Target{ChatID: 2, ChatType: 3}
	manager, err := NewManager(t.TempDir(), map[uint64]Target{1: one, 2: two}, &fakeSource{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.queue.mu.Lock()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { first <- manager.Assign(t.Context(), one, 0, time.Now()) }()
	waitForAssignmentsBlocked(t, 1)
	go func() { second <- manager.Assign(t.Context(), two, 0, time.Now()) }()
	waitForAssignmentsBlocked(t, 2)
	bindings := manager.Bindings()
	manager.queue.mu.Unlock()
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].Target != two {
		t.Fatalf("second update persisted before first publication: %#v", bindings)
	}
	if !reflect.DeepEqual(manager.store.Targets(), manager.queue.targetsSnapshot()) {
		t.Fatal("persisted bindings and delivery targets differ")
	}
}

// Mutex waits are not durable blocks in synctest. Observe both goroutines at
// their actual lock barrier instead of assuming that a sleep orders them.
func waitForAssignmentsBlocked(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 128*1024)
		n := runtime.Stack(buf, true)
		count := 0
		for _, stack := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, "TestManagerSerializesBindingPublication.func") &&
				strings.Contains(stack, "[sync.Mutex.Lock]") {
				count++
			}
		}
		if count == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("assignments did not reach lock barrier")
}
