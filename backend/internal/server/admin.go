package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	learningdomain "agp/backend/internal/learning"
	userdomain "agp/backend/internal/user"
)

func (a *app) handleAdminLearningConfig(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	settings, err := a.groupLearningConfig(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "learning_config_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *app) handleAdminSaveLearningConfig(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var settings map[string]any
	if !readJSON(w, r, &settings) {
		return
	}
	if settings == nil {
		settings = map[string]any{}
	}
	before, after, err := a.upsertGroupLearningConfig(r.Context(), groupID, settings)
	if errors.Is(err, learningdomain.ErrLearningConfigConflict) {
		writeError(w, http.StatusConflict, "learning_config_conflict")
		return
	}
	if errors.Is(err, learningdomain.ErrInvalidDailyVerse) {
		writeError(w, http.StatusBadRequest, "invalid_daily_verse")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "learning_config_save_failed")
		return
	}
	if notifier, ok := a.notifications.(interface {
		WakeInitial(uint64, time.Time) error
	}); ok {
		if err := notifier.WakeInitial(groupID, time.Now().UTC()); err != nil {
			slog.ErrorContext(r.Context(), "initial notification wake failed",
				"group_id", groupID, "error", err)
		}
	}
	a.auditChanges(groupID, u.ID, "save_learning_config", "group_settings", groupID, before, after, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": settings})
}

func (a *app) handleAdminCreateMember(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		CreateUser  bool   `json:"create_user"`
		UserID      uint64 `json:"user_id"`
		DisplayName string `json:"display_name"`
		Username    string `json:"username"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	userID, err := a.users.CreateMember(r.Context(), groupID, u.ID, userdomain.CreateMemberInput{
		CreateUser:  req.CreateUser,
		TenantID:    u.CurrentTenantID,
		UserID:      req.UserID,
		DisplayName: req.DisplayName,
		Username:    req.Username,
	})
	var usernameConflict *userdomain.UsernameConflictError
	if errors.As(err, &usernameConflict) {
		w.Header().Set("X-AGP-Error-Code", "username_exists")
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":         "username_exists",
			"existing_user": usernameConflict.ExistingUser,
		})
		return
	}
	if errors.Is(err, userdomain.ErrUsernameDisplayNameRequired) {
		writeError(w, http.StatusBadRequest, "username_display_name_required")
		return
	}
	if errors.Is(err, userdomain.ErrUserIDRequired) {
		writeError(w, http.StatusBadRequest, "user_id_required")
		return
	}
	if errors.Is(err, userdomain.ErrGroupDefaultPasswordMissing) {
		writeError(w, http.StatusInternalServerError, "group_default_password_missing")
		return
	}
	if errors.Is(err, userdomain.ErrUserCreateFailed) {
		writeError(w, http.StatusConflict, "user_create_failed")
		return
	}
	if errors.Is(err, userdomain.ErrUsernameExists) {
		writeError(w, http.StatusConflict, "username_exists")
		return
	}
	if errors.Is(err, userdomain.ErrMemberAddFailed) {
		writeError(w, http.StatusConflict, "member_add_failed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	a.audit(groupID, u.ID, "add_member", "group_members", userID, nil, map[string]any{
		"user_id":      userID,
		"username":     req.Username,
		"display_name": req.DisplayName,
		"created_user": req.CreateUser,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID})
}

func (a *app) handleAdminRemoveMember(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	memberID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	before := a.memberAuditValue(r.Context(), groupID, memberID)
	targetUserID, err := a.users.RemoveMember(r.Context(), groupID, memberID, u.ID, u.IsSuperAdmin, time.Now().UTC())
	if errors.Is(err, userdomain.ErrMemberNotFound) {
		writeError(w, http.StatusNotFound, "member_not_found")
		return
	}
	if errors.Is(err, userdomain.ErrCannotRemoveSelf) {
		writeError(w, http.StatusBadRequest, "cannot_remove_self")
		return
	}
	if errors.Is(err, userdomain.ErrCannotRemoveSuperAdmin) {
		writeError(w, http.StatusForbidden, "cannot_remove_super_admin")
		return
	}
	if errors.Is(err, userdomain.ErrCannotRemoveGroupLeader) {
		writeError(w, http.StatusForbidden, "cannot_remove_group_leader")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_remove_failed")
		return
	}
	a.audit(groupID, u.ID, "remove_member", "group_members", memberID, before, map[string]any{
		"user_id": targetUserID,
		"deleted": true,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleAdminSetGroupDefaultPassword(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	affected, err := a.setGroupDefaultPassword(groupID, req.Password, false, u.ID, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "affected_users": affected, "message": "多小组成员、组长和超级管理员账号不会被本组默认密码覆盖。"})
}

func (a *app) handleGrantGroupAdmin(w http.ResponseWriter, r *http.Request) {
	a.setRole(w, r, roleGroupAdmin, true)
}

func (a *app) handleRevokeGroupAdmin(w http.ResponseWriter, r *http.Request) {
	a.setRole(w, r, roleGroupAdmin, false)
}

func (a *app) setRole(w http.ResponseWriter, r *http.Request, role string, grant bool) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	memberID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	member := a.memberAuditValue(r.Context(), groupID, memberID)
	err := a.users.SetRole(r.Context(), groupID, memberID, u.ID, u.IsSuperAdmin, role, grant, time.Now().UTC())
	if errors.Is(err, userdomain.ErrMemberNotFound) {
		writeError(w, http.StatusNotFound, "member_not_found")
		return
	}
	if errors.Is(err, userdomain.ErrCannotManageSelf) {
		writeError(w, http.StatusBadRequest, "cannot_manage_self")
		return
	}
	if errors.Is(err, userdomain.ErrCannotManageSuperAdmin) {
		writeError(w, http.StatusForbidden, "cannot_manage_super_admin")
		return
	}
	if errors.Is(err, userdomain.ErrCannotManageGroupLeader) {
		writeError(w, http.StatusForbidden, "cannot_manage_group_leader")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_role_save_failed")
		return
	}
	action := "grant_group_role"
	before, after := false, true
	if !grant {
		action = "revoke_group_role"
		before, after = true, false
	}
	if member != nil {
		before = stringSliceContains(member["roles"], role)
		after = grant
	}
	a.auditChanges(
		groupID,
		u.ID,
		action,
		"user_group_roles",
		memberID,
		map[string]any{"role": role, "granted": before},
		map[string]any{"role": role, "granted": after},
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) memberAuditValue(ctx context.Context, groupID, memberID uint64) map[string]any {
	members, err := a.users.Members(ctx, groupID)
	if err != nil {
		slog.WarnContext(ctx, "member audit snapshot failed",
			"group_id", groupID, "member_id", memberID, "error", err)
		return nil
	}
	for _, member := range members {
		if member.MemberID == memberID {
			return map[string]any{
				"member_id":   member.MemberID,
				"user_id":     member.UserID,
				"username":    member.Username,
				"member_name": member.MemberName,
				"roles":       member.Roles,
			}
		}
	}
	return nil
}

func stringSliceContains(value any, target string) bool {
	items, ok := value.([]string)
	if !ok {
		return false
	}
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (a *app) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	items, err := a.audits.ListByGroup(r.Context(), groupID, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *app) handleAllAuditLogs(w http.ResponseWriter, r *http.Request) {
	items, err := a.audits.ListAll(r.Context(), 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
