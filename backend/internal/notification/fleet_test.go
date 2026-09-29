package notification

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseRobotConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        string
		legacyToken  string
		legacyGroups string
		want         []RobotConfig
		wantErr      bool
	}{
		{
			name:         "disabled",
			legacyGroups: "",
		},
		{
			name:         "legacy default",
			legacyToken:  "123:secret",
			legacyGroups: `{"1":{"chat_id":99,"chat_type":3}}`,
			want: []RobotConfig{{
				ID: "default", Name: "默认机器人", Token: "123:secret",
				Groups: map[uint64]Target{1: {ChatID: 99, ChatType: 3}},
			}},
		},
		{
			name: "multiple robots sorted",
			value: `{
				"secondary":{"name":"备用机器人","token":"456:secret","groups":{"2":{"chat_id":99,"chat_type":2}}},
				"primary":{"name":"主机器人","token":"123:secret","groups":{"1":{"chat_id":99,"chat_type":3}}}
			}`,
			want: []RobotConfig{
				{
					ID: "primary", Name: "主机器人", Token: "123:secret",
					Groups: map[uint64]Target{1: {ChatID: 99, ChatType: 3}},
				},
				{
					ID: "secondary", Name: "备用机器人", Token: "456:secret",
					Groups: map[uint64]Target{2: {ChatID: 99, ChatType: 2}},
				},
			},
		},
		{
			name:    "invalid id",
			value:   `{"Bad ID":{"name":"bad","token":"123:secret"}}`,
			wantErr: true,
		},
		{
			name:    "duplicate token",
			value:   `{"one":{"name":"one","token":"123:secret"},"two":{"name":"two","token":"123:secret"}}`,
			wantErr: true,
		},
		{
			name:    "unknown field",
			value:   `{"one":{"name":"one","token":"123:secret","enabled":true}}`,
			wantErr: true,
		},
		{
			name:    "duplicate chat in one robot",
			value:   `{"one":{"name":"one","token":"123:secret","groups":{"1":{"chat_id":99,"chat_type":2},"2":{"chat_id":99,"chat_type":2}}}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRobotConfigs(tt.value, tt.legacyToken, tt.legacyGroups)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseRobotConfigs() error = %v, wantErr=%v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseRobotConfigs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFleetUsesIndependentRobotDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fleet, err := NewFleet(dir, []RobotConfig{
		{ID: "default", Name: "default", Token: "123:secret"},
		{ID: "secondary", Name: "secondary", Token: "456:secret"},
	}, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if fleet.robots["default"].manager.queue.dir != dir {
		t.Fatalf("default dir = %q, want %q", fleet.robots["default"].manager.queue.dir, dir)
	}
	if fleet.robots["secondary"].manager.queue.dir == dir {
		t.Fatal("secondary robot reused the legacy queue directory")
	}
}

func TestFleetScopesStatusAndBindingsByRobot(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":101,"first_name":"Bot","username":"test_bot"}}`)
		case "/getGroups":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"Groups":[],"SuperGroups":[{"PeerID":20,"PeerName":"Study"}]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fleet, err := NewFleet(t.TempDir(), []RobotConfig{
		{ID: "primary", Name: "primary", Token: "123:secret"},
		{ID: "secondary", Name: "secondary", Token: "456:secret"},
	}, &fakeSource{snapshot: Snapshot{Text: "current", ExpiresAt: time.Now().Add(time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	for _, robot := range fleet.robots {
		robot.client.identityEndpoint = server.URL + "/getMe"
		robot.client.groupsEndpoint = server.URL + "/getGroups"
	}
	target := Target{ChatID: 20, ChatType: 3}
	if err := fleet.Assign(t.Context(), "primary", target, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := fleet.Assign(t.Context(), "secondary", target, 2, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := fleet.BindingGroupID("primary", 20); got != 1 {
		t.Fatalf("primary group = %d, want 1", got)
	}
	if got := fleet.BindingGroupID("secondary", 20); got != 2 {
		t.Fatalf("secondary group = %d, want 2", got)
	}

	statuses := fleet.Robots(t.Context())
	if len(statuses) != 2 {
		t.Fatalf("statuses = %#v", statuses)
	}
	for _, status := range statuses {
		if !status.Authenticated || status.State != robotHealthy || len(status.Chats) != 1 {
			t.Fatalf("status = %#v", status)
		}
		if status.Source != robotSourceDeployment {
			t.Fatalf("status source = %q, want %q", status.Source, robotSourceDeployment)
		}
		if status.Chats[0].GroupID != fleet.BindingGroupID(status.ID, 20) {
			t.Fatalf("chat binding = %#v", status.Chats[0])
		}
	}
}

func TestFleetRegisterPersistsRobotAndKeepsTokenServerSide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":101,"first_name":"Primary Bot","username":"primary_bot"}}`)
		case "/getGroups":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"Groups":[],"SuperGroups":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fleet, err := NewFleet(t.TempDir(), nil, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	configureClient := func(client *PotatoClient) {
		client.identityEndpoint = server.URL + "/getMe"
		client.groupsEndpoint = server.URL + "/getGroups"
	}
	newClient := newPotatoClient
	t.Cleanup(func() { newPotatoClient = newClient })
	newPotatoClient = func(token string) (*PotatoClient, error) {
		client, err := newClient(token)
		if err != nil {
			return nil, err
		}
		configureClient(client)
		return client, nil
	}

	status, err := fleet.Register(t.Context(), RobotRegistration{Token: "123:secret"})
	if err != nil {
		t.Fatal(err)
	}
	if status.ID != "primary_bot" || status.Name != "Primary Bot" || !status.Authenticated ||
		status.Source != robotSourceRegistration {
		t.Fatalf("status = %#v", status)
	}
	data, err := os.ReadFile(filepath.Join(fleet.dir, "robots.json"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(fleet.dir, "robots.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("robots.json mode = %o, want 600", info.Mode().Perm())
	}
	var stored struct {
		Robots []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"token"`
		} `json:"robots"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Robots) != 1 || stored.Robots[0].Token != "123:secret" {
		t.Fatalf("stored = %#v", stored)
	}
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "secret") {
		t.Fatalf("status leaked token: %s", payload)
	}
}

func TestFleetRegisterRejectsDuplicateIDAndToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"id":101,"first_name":"Primary Bot","username":"primary_bot"}}`)
		case "/getGroups":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"Groups":[],"SuperGroups":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	newClient := newPotatoClient
	t.Cleanup(func() { newPotatoClient = newClient })
	newPotatoClient = func(token string) (*PotatoClient, error) {
		client, err := newClient(token)
		if err != nil {
			return nil, err
		}
		client.identityEndpoint = server.URL + "/getMe"
		client.groupsEndpoint = server.URL + "/getGroups"
		return client, nil
	}

	fleet, err := NewFleet(t.TempDir(), []RobotConfig{{ID: "existing", Name: "existing", Token: "999:secret"}}, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fleet.Register(t.Context(), RobotRegistration{ID: "existing", Token: "123:secret"}); err != ErrRobotAlreadyExists {
		t.Fatalf("duplicate ID error = %v", err)
	}
	if _, err := fleet.Register(t.Context(), RobotRegistration{ID: "newbot", Token: "999:secret"}); err != ErrRobotTokenExists {
		t.Fatalf("duplicate token error = %v", err)
	}
	if _, err := fleet.Register(t.Context(), RobotRegistration{ID: defaultRobotID, Token: "123:other"}); err != ErrInvalidRobotConfig {
		t.Fatalf("reserved default ID error = %v", err)
	}
}

func TestFleetRemoveRejectsDeploymentRobot(t *testing.T) {
	t.Parallel()

	for _, id := range []string{defaultRobotID, "secondary"} {
		t.Run(id, func(t *testing.T) {
			fleet, err := NewFleet(t.TempDir(), []RobotConfig{{
				ID: id, Name: id, Token: "123:secret",
			}}, &fakeSource{})
			if err != nil {
				t.Fatal(err)
			}
			if err := fleet.Remove(id); err != ErrRobotCannotRemove {
				t.Fatalf("Remove() error = %v, want %v", err, ErrRobotCannotRemove)
			}
			if fleet.robots[id] == nil {
				t.Fatal("deployment robot was removed")
			}
		})
	}
}

func TestFleetRemoveAllowsLegacyRegisteredDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewRobotConfigStore(filepath.Join(dir, "robots.json"))
	if err := store.Save([]RobotConfig{{
		ID: defaultRobotID, Name: "legacy", Token: "123:secret",
	}}); err != nil {
		t.Fatal(err)
	}
	fleet, err := NewFleet(dir, nil, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if fleet.robots[defaultRobotID].source != robotSourceRegistration {
		t.Fatalf("source = %q, want %q", fleet.robots[defaultRobotID].source, robotSourceRegistration)
	}
	if err := fleet.Remove(defaultRobotID); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewFleet(dir, nil, &fakeSource{})
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.robots) != 0 {
		t.Fatalf("removed robot restored after restart: %#v", restarted.robots)
	}
}
