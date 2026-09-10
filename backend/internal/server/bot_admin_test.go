package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	notificationdomain "agp/backend/internal/notification"
	userdomain "agp/backend/internal/user"
)

type fakeBotManager struct {
	chats       []notificationdomain.Chat
	bindings    []notificationdomain.Binding
	assignments []notificationdomain.Binding
	err         error
}

func (m *fakeBotManager) Chats(context.Context) ([]notificationdomain.Chat, error) {
	return m.chats, m.err
}

func (m *fakeBotManager) Bindings() []notificationdomain.Binding {
	return m.bindings
}

func (m *fakeBotManager) Assign(
	_ context.Context,
	target notificationdomain.Target,
	groupID uint64,
	_ time.Time,
) error {
	m.assignments = append(m.assignments, notificationdomain.Binding{Target: target, GroupID: groupID})
	return m.err
}

func TestBotManagementRequiresSuperAdmin(t *testing.T) {
	t.Parallel()
	app := &app{}
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/bot-management", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 2}))
	response := httptest.NewRecorder()
	app.requireSuper(app.handleBotManagement).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestBotManagementListsJoinedChatsAndBindings(t *testing.T) {
	t.Parallel()
	manager := &fakeBotManager{
		chats: []notificationdomain.Chat{
			{ChatID: 20, ChatType: 3, Title: "2026 bible study"},
		},
		bindings: []notificationdomain.Binding{
			{Target: notificationdomain.Target{ChatID: 20, ChatType: 3}, GroupID: 1},
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
		Configured bool `json:"configured"`
		Chats      []struct {
			ChatID  int64  `json:"chat_id"`
			GroupID uint64 `json:"group_id"`
		} `json:"chats"`
		StudyGroups []userdomain.Group `json:"study_groups"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Configured || len(payload.Chats) != 1 || payload.Chats[0].GroupID != 1 ||
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
	}{
		{"assign", `{"chat_id":20,"chat_type":3,"group_id":1}`, http.StatusOK, 1},
		{"unbind", `{"chat_id":20,"chat_type":3,"group_id":0}`, http.StatusOK, 1},
		{"unknown study group", `{"chat_id":20,"chat_type":3,"group_id":2}`, http.StatusBadRequest, 0},
		{"direct chat", `{"chat_id":20,"chat_type":1,"group_id":1}`, http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeBotManager{}
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
		})
	}
}
