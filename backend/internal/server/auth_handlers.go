package server

import (
	"net/http"
	"strings"
	"time"
)

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	username := strings.TrimSpace(strings.ToLower(req.Username))
	remote := clientIP(r)
	if a.loginLimiter.blocked(remote, username) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	user, groups, currentGroupID, err := a.users.LoginUser(r.Context(), username)
	if err != nil || !verifyPassword(req.Password, user.PasswordHash) {
		a.loginLimiter.fail(remote, username)
		writeError(w, http.StatusUnauthorized, "invalid_username_or_password")
		return
	}
	a.loginLimiter.success(remote, username)
	token, err := a.signToken(tokenClaims{UserID: user.ID, CurrentGroupID: currentGroupID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed")
		return
	}
	_ = a.users.RecordLogin(r.Context(), user.ID, time.Now().UTC())
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
			"is_super_admin": user.IsSuperAdmin, "default_group_id": nullableUint64Value(user.DefaultGroupID),
			"must_change_password": user.MustChangePassword,
			"current_group_id":     currentGroupID, "study_groups": groups,
		},
	})
}

func (a *app) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": mustUser(r)})
}

func (a *app) handleSwitchGroup(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		GroupID uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !containsGroup(u.Groups, req.GroupID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	token, err := a.signToken(tokenClaims{UserID: u.ID, CurrentGroupID: req.GroupID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (a *app) handleSetDefaultGroup(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		GroupID uint64 `json:"group_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.GroupID == 0 {
		if err := a.users.SetDefaultGroup(r.Context(), u.ID, 0, time.Now().UTC()); err != nil {
			writeError(w, http.StatusInternalServerError, "default_group_failed")
			return
		}
		u.DefaultGroupID = 0
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
		return
	}
	if !containsGroup(u.Groups, req.GroupID) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if err := a.users.SetDefaultGroup(r.Context(), u.ID, req.GroupID, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "default_group_failed")
		return
	}
	u.DefaultGroupID = req.GroupID
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

func (a *app) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password_too_short")
		return
	}
	oldHash, err := a.users.PasswordHash(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_not_found")
		return
	}
	if !verifyPassword(req.OldPassword, oldHash) {
		writeError(w, http.StatusUnauthorized, "invalid_password")
		return
	}
	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	if err := a.users.UpdatePassword(r.Context(), u.ID, hash, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "password_save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	members, _ := a.listMembers(groupID)
	date := queryDate(r, "date", time.Now().In(a.location))
	week, _ := a.currentWeekAt(groupID, date)
	var tasks []map[string]any
	if week != nil {
		if weekID, ok := week["id"].(uint64); ok {
			tasks, _ = a.weekTasks(groupID, weekID)
		}
	}
	learningConfig, _ := a.groupLearningConfig(groupID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":            u,
		"members":         members,
		"current_week":    week,
		"current_tasks":   tasks,
		"learning_config": learningConfig,
	})
}
