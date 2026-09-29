package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	userdomain "agp/backend/internal/user"
)

type personalSettingsHandlerRepository struct {
	userdomain.Repository
	userID   uint64
	groupID  uint64
	settings userdomain.PersonalSettings
}

func (r *personalSettingsHandlerRepository) UpdatePersonalSettings(
	_ context.Context,
	userID, groupID uint64,
	settings userdomain.PersonalSettings,
	_ time.Time,
) error {
	r.userID = userID
	r.groupID = groupID
	r.settings = settings
	return nil
}

func TestHandleUpdatePersonalSettingsAllowsOrdinaryMemberForCurrentGroup(t *testing.T) {
	t.Parallel()

	repo := &personalSettingsHandlerRepository{}
	auditRepo := &serverAuditRepository{}
	a := &app{
		users:  userdomain.NewService(repo),
		audits: auditdomain.NewService(auditRepo),
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/personal-settings",
		bytes.NewBufferString(`{"member_name":"  本组名字  ","mobile_view_mode":"stacked"}`),
	)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID:             23,
		Username:       "unchanged-account",
		CurrentGroupID: 7,
		MemberName:     "旧名字",
		MobileViewMode: userdomain.MobileViewMasonry,
		Roles:          []string{userdomain.RoleMember},
	}))
	recorder := httptest.NewRecorder()

	a.handleUpdatePersonalSettings(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if repo.userID != 23 || repo.groupID != 7 {
		t.Fatalf("update scope = user %d group %d, want user 23 group 7", repo.userID, repo.groupID)
	}
	if repo.settings.MemberName != "本组名字" || repo.settings.MobileViewMode != userdomain.MobileViewStacked {
		t.Fatalf("saved settings = %+v", repo.settings)
	}
	var payload struct {
		Settings userdomain.PersonalSettings `json:"settings"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Settings != repo.settings {
		t.Fatalf("response settings = %+v, want %+v", payload.Settings, repo.settings)
	}
	if len(auditRepo.logs) != 1 || auditRepo.logs[0].Action != "update_personal_settings" {
		t.Fatalf("audit logs = %#v", auditRepo.logs)
	}
	if auditRepo.logs[0].BeforeJSON != `{"member_name":"旧名字","mobile_view_mode":"masonry"}` ||
		auditRepo.logs[0].AfterJSON != `{"member_name":"本组名字","mobile_view_mode":"stacked"}` {
		t.Fatalf("audit changes = %s -> %s", auditRepo.logs[0].BeforeJSON, auditRepo.logs[0].AfterJSON)
	}
}

func TestHandleUpdatePersonalSettingsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	repo := &personalSettingsHandlerRepository{}
	a := &app{users: userdomain.NewService(repo)}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/personal-settings",
		bytes.NewBufferString(`{"member_name":"","mobile_view_mode":"masonry"}`),
	)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID:             23,
		CurrentGroupID: 7,
	}))
	recorder := httptest.NewRecorder()

	a.handleUpdatePersonalSettings(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "member_name_required" {
		t.Fatalf("error = %q, want member_name_required", payload["error"])
	}
	if repo.userID != 0 {
		t.Fatal("repository called for invalid input")
	}
}
