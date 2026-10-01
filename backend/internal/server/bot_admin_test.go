package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	notificationdomain "agp/backend/internal/notification"
	userdomain "agp/backend/internal/user"
)

type fakeBotManager struct {
	robots      []notificationdomain.RobotStatus
	robotIDs    []string
	assignments []notificationdomain.Binding
	registered  []notificationdomain.RobotRegistration
	register    notificationdomain.RobotStatus
	err         error
	cleared     int
	clearedIDs  []string
}

func (m *fakeBotManager) Remove(string) error { return m.err }

func (m *fakeBotManager) ClearFailed(robotID string) (int, error) {
	m.clearedIDs = append(m.clearedIDs, robotID)
	return m.cleared, m.err
}

func (m *fakeBotManager) Robots(context.Context) []notificationdomain.RobotStatus {
	return m.robots
}

func (m *fakeBotManager) Assign(
	_ context.Context,
	robotID string,
	target notificationdomain.Target,
	groupID uint64,
	_ time.Time,
) error {
	m.robotIDs = append(m.robotIDs, robotID)
	m.assignments = append(m.assignments, notificationdomain.Binding{Target: target, GroupID: groupID})
	return m.err
}

func (m *fakeBotManager) Register(
	_ context.Context,
	req notificationdomain.RobotRegistration,
) (notificationdomain.RobotStatus, error) {
	m.registered = append(m.registered, req)
	return m.register, m.err
}

func (m *fakeBotManager) BindingGroupID(robotID string, chatID int64) uint64 {
	for _, robot := range m.robots {
		if robot.ID != robotID {
			continue
		}
		for _, binding := range robot.Bindings {
			if binding.ChatID == chatID {
				return binding.GroupID
			}
		}
	}
	return 0
}

func TestBotRobotRegistersWithoutExposingToken(t *testing.T) {
	t.Parallel()

	manager := &fakeBotManager{register: notificationdomain.RobotStatus{
		ID: "primary", Name: "主机器人", State: "healthy", Authenticated: true,
		Identity: notificationdomain.RobotIdentity{ID: 101, Username: "primary_bot"},
	}}
	app := &app{
		botManager: manager,
		audits:     auditdomain.NewService(notificationAuditRepository{}),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/super-admin/bot-robots", strings.NewReader(
		`{"id":"primary","name":"主机器人","token":"123:secret"}`,
	))
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID: 1, IsSuperAdmin: true,
	}))
	response := httptest.NewRecorder()
	app.handleBotRobot(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if len(manager.registered) != 1 || manager.registered[0].Token != "123:secret" {
		t.Fatalf("registered = %#v", manager.registered)
	}
	if strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response leaked token: %s", response.Body)
	}
}

func TestBotRobotMapsRegistrationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"invalid", notificationdomain.ErrInvalidRobotConfig, http.StatusBadRequest},
		{"auth", notificationdomain.ErrRobotAuthentication, http.StatusBadGateway},
		{"duplicate id", notificationdomain.ErrRobotAlreadyExists, http.StatusConflict},
		{"duplicate token", notificationdomain.ErrRobotTokenExists, http.StatusConflict},
		{"limit", notificationdomain.ErrRobotLimitExceeded, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &app{
				botManager: &fakeBotManager{err: tt.err},
				audits:     auditdomain.NewService(notificationAuditRepository{}),
			}
			request := httptest.NewRequest(http.MethodPost, "/api/super-admin/bot-robots", strings.NewReader(
				`{"token":"123:secret"}`,
			))
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
				ID: 1, IsSuperAdmin: true,
			}))
			response := httptest.NewRecorder()
			app.handleBotRobot(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}

func TestBotRobotClearsFailedNotificationArchive(t *testing.T) {
	t.Parallel()

	manager := &fakeBotManager{cleared: 7}
	app := &app{
		botManager: manager,
		audits:     auditdomain.NewService(notificationAuditRepository{}),
	}
	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/super-admin/bot-robots/primary/failed-notifications",
		nil,
	)
	request.SetPathValue("id", "primary")
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID: 1, IsSuperAdmin: true,
	}))
	response := httptest.NewRecorder()

	app.handleBotNotificationFailuresDelete(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	if len(manager.clearedIDs) != 1 || manager.clearedIDs[0] != "primary" {
		t.Fatalf("cleared robot IDs = %#v", manager.clearedIDs)
	}
	var payload struct {
		Cleared int `json:"cleared"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Cleared != 7 {
		t.Fatalf("cleared = %d, want 7", payload.Cleared)
	}
}

func TestBotManagementRequiresPlatformOrTenantAdmin(t *testing.T) {
	t.Parallel()
	app := &app{}
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/bot-management", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 2}))
	response := httptest.NewRecorder()
	app.requireBotAdmin(app.handleBotManagement).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestTenantBotManagementShowsAllChatsButOnlyOwnGroups(t *testing.T) {
	t.Parallel()
	manager := &fakeBotManager{robots: []notificationdomain.RobotStatus{{
		ID: "primary", Name: "主机器人", State: "healthy", Authenticated: true,
		Chats: []notificationdomain.Chat{
			{ChatID: 20, ChatType: 3, Title: "本小家群", GroupID: 1},
			{ChatID: 21, ChatType: 3, Title: "其他小家群", GroupID: 2},
		},
		Bindings: []notificationdomain.Binding{
			{Target: notificationdomain.Target{ChatID: 20, ChatType: 3}, GroupID: 1},
			{Target: notificationdomain.Target{ChatID: 21, ChatType: 3}, GroupID: 2},
		},
	}}}
	app := &app{botManager: manager}
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/bot-management", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID: 3, IsTenantAdmin: true, CurrentGroupID: 1, CurrentTenantID: 10,
		Groups: []userdomain.Group{{ID: 1, TenantID: 10, Name: "本小家小组"}, {ID: 2, TenantID: 20, Name: "其他小家小组"}},
	}))
	response := httptest.NewRecorder()
	app.requireBotAdmin(app.handleBotManagement).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
	var payload struct {
		Robots      []notificationdomain.RobotStatus `json:"robots"`
		StudyGroups []userdomain.Group               `json:"study_groups"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Robots) != 1 || len(payload.Robots[0].Chats) != 2 || len(payload.StudyGroups) != 1 || payload.StudyGroups[0].ID != 1 {
		t.Fatalf("tenant view lost robots/chats or leaked groups: %+v", payload)
	}
	robot := payload.Robots[0]
	if robot.Chats[0].GroupID != 1 || robot.Chats[1].GroupID != 0 || !robot.Chats[1].BoundElsewhere || len(robot.Bindings) != 1 || robot.Bindings[0].GroupID != 1 {
		t.Fatalf("tenant view leaked foreign binding: %+v", robot)
	}
	if manager.robots[0].Chats[1].GroupID != 2 || len(manager.robots[0].Bindings) != 2 {
		t.Fatal("tenant response mutated shared robot state")
	}
}

func TestBotManagementListsJoinedChatsAndBindings(t *testing.T) {
	t.Parallel()
	manager := &fakeBotManager{
		robots: []notificationdomain.RobotStatus{
			{
				ID: "primary", Name: "主机器人", State: "healthy", Authenticated: true,
				Chats: []notificationdomain.Chat{
					{ChatID: 20, ChatType: 3, Title: "2026 bible study", GroupID: 1},
				},
				Bindings: []notificationdomain.Binding{
					{Target: notificationdomain.Target{ChatID: 20, ChatType: 3}, GroupID: 1},
				},
			},
		},
	}
	app := &app{botManager: manager}
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/bot-management", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID: 1, IsSuperAdmin: true, Groups: []userdomain.Group{{ID: 1, Name: "AGAPE A组"}},
	}))
	response := httptest.NewRecorder()
	app.handleBotManagement(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body)
	}
	var payload struct {
		Configured  bool                             `json:"configured"`
		Robots      []notificationdomain.RobotStatus `json:"robots"`
		StudyGroups []userdomain.Group               `json:"study_groups"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Configured || len(payload.Robots) != 1 || len(payload.Robots[0].Chats) != 1 ||
		payload.Robots[0].Chats[0].GroupID != 1 ||
		len(payload.StudyGroups) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestBotBindingValidatesAndAssigns(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCalls  int
		wantRobot  string
		managerErr error
	}{
		{"assign", `{"robot_id":"primary","chat_id":20,"chat_type":3,"group_id":1}`, http.StatusOK, 1, "primary", nil},
		{"legacy default robot", `{"chat_id":20,"chat_type":3,"group_id":1}`, http.StatusOK, 1, "default", nil},
		{"unbind", `{"robot_id":"primary","chat_id":20,"chat_type":3,"group_id":0}`, http.StatusOK, 1, "primary", nil},
		{"unknown robot", `{"robot_id":"missing","chat_id":20,"chat_type":3,"group_id":1}`, http.StatusNotFound, 1, "missing", notificationdomain.ErrRobotNotFound},
		{"authentication failed", `{"robot_id":"primary","chat_id":20,"chat_type":3,"group_id":1}`, http.StatusBadGateway, 1, "primary", notificationdomain.ErrRobotAuthentication},
		{"unknown study group", `{"robot_id":"primary","chat_id":20,"chat_type":3,"group_id":2}`, http.StatusBadRequest, 0, "", nil},
		{"direct chat", `{"robot_id":"primary","chat_id":20,"chat_type":1,"group_id":1}`, http.StatusBadRequest, 0, "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeBotManager{err: tt.managerErr}
			app := &app{
				botManager: manager,
				audits:     auditdomain.NewService(notificationAuditRepository{}),
			}
			request := httptest.NewRequest(http.MethodPut, "/api/super-admin/bot-bindings", strings.NewReader(tt.body))
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
				ID: 1, IsSuperAdmin: true, Groups: []userdomain.Group{{ID: 1, Name: "AGAPE A组"}},
			}))
			response := httptest.NewRecorder()
			app.handleBotBinding(response, request)
			if response.Code != tt.wantStatus || len(manager.assignments) != tt.wantCalls {
				t.Fatalf("status=%d calls=%d body=%s", response.Code, len(manager.assignments), response.Body)
			}
			if tt.wantCalls == 1 && manager.robotIDs[0] != tt.wantRobot {
				t.Fatalf("robot ID = %q, want %q", manager.robotIDs[0], tt.wantRobot)
			}
		})
	}
}

func TestTenantBotBindingOnlyAcceptsCurrentTenantGroup(t *testing.T) {
	t.Parallel()
	manager := &fakeBotManager{}
	app := &app{botManager: manager, audits: auditdomain.NewService(notificationAuditRepository{})}
	for _, tc := range []struct {
		groupID uint64
		want    int
	}{{1, http.StatusOK}, {2, http.StatusBadRequest}} {
		request := httptest.NewRequest(http.MethodPut, "/api/super-admin/bot-bindings", strings.NewReader(
			`{"robot_id":"primary","chat_id":20,"chat_type":3,"group_id":`+strconv.FormatUint(tc.groupID, 10)+`}`,
		))
		request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
			ID: 3, IsTenantAdmin: true, CurrentGroupID: 1, CurrentTenantID: 10,
			Groups: []userdomain.Group{{ID: 1, TenantID: 10}, {ID: 2, TenantID: 20}},
		}))
		response := httptest.NewRecorder()
		app.requireBotAdmin(app.handleBotBinding).ServeHTTP(response, request)
		if response.Code != tc.want {
			t.Fatalf("group %d: status=%d body=%s", tc.groupID, response.Code, response.Body)
		}
	}
	if len(manager.assignments) != 1 || manager.assignments[0].GroupID != 1 {
		t.Fatalf("tenant binding calls = %+v", manager.assignments)
	}
}
