package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	userdomain "agp/backend/internal/user"
)

func (a *app) handleAdminLearningConfig(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	settings, err := a.groupLearningConfig(groupID)
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
	if err := a.upsertGroupLearningConfig(groupID, settings); err != nil {
		writeError(w, http.StatusInternalServerError, "learning_config_save_failed")
		return
	}
	a.audit(groupID, u.ID, "save_learning_config", "group_settings", groupID, nil, map[string]any{"keys": len(settings)}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": settings})
}

func (a *app) handleAdminMembers(w http.ResponseWriter, r *http.Request) {
	a.handleMembers(w, r)
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
		NamePinyin  string `json:"name_pinyin"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	userID, err := a.users.CreateMember(r.Context(), groupID, u.ID, userdomain.CreateMemberInput{
		CreateUser:  req.CreateUser,
		UserID:      req.UserID,
		DisplayName: req.DisplayName,
		Username:    req.Username,
		NamePinyin:  req.NamePinyin,
	})
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
	if errors.Is(err, userdomain.ErrMemberAddFailed) {
		writeError(w, http.StatusConflict, "member_add_failed")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID})
}

func (a *app) handleAdminRemoveMember(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	memberID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
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
	a.audit(groupID, u.ID, "remove_member", "group_members", memberID, nil, map[string]any{"user_id": targetUserID}, r)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
