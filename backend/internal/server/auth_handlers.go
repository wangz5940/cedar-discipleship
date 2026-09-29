package server

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	userdomain "agp/backend/internal/user"
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
		a.recordLoginLog(r, userdomain.LoginLog{
			Username:      username,
			Success:       false,
			FailureReason: "too_many_attempts",
		})
		writeError(w, http.StatusTooManyRequests, "too_many_attempts")
		return
	}
	user, groups, currentGroupID, err := a.users.LoginUser(r.Context(), username)
	if err != nil || !verifyPassword(req.Password, user.PasswordHash) {
		a.loginLimiter.fail(remote, username)
		var userID uint64
		if user != nil {
			userID = user.ID
		}
		a.recordLoginLog(r, userdomain.LoginLog{
			UserID:        userID,
			GroupID:       currentGroupID,
			Username:      username,
			Success:       false,
			FailureReason: "invalid_username_or_password",
		})
		writeError(w, http.StatusUnauthorized, "invalid_username_or_password")
		return
	}
	a.loginLimiter.success(remote, username)
	sessionID, err := a.issueRefreshSession(r.Context(), w, r, user.ID, currentGroupID, user.PasswordHash)
	if errors.Is(err, userdomain.ErrPasswordChanged) {
		writeError(w, http.StatusUnauthorized, "invalid_username_or_password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session_failed")
		return
	}
	token, err := a.signToken(tokenClaims{UserID: user.ID, CurrentGroupID: currentGroupID, SessionID: sessionID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed")
		return
	}
	_ = a.users.RecordLogin(r.Context(), user.ID, time.Now().UTC())
	a.recordLoginLog(r, userdomain.LoginLog{
		UserID:   user.ID,
		GroupID:  currentGroupID,
		Username: username,
		Success:  true,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  loginUserResponse(user, groups, currentGroupID),
	})
}

func (a *app) handleRefreshSession(w http.ResponseWriter, r *http.Request) {
	refreshToken, csrfToken, err := refreshCredentials(r)
	if err != nil {
		logAuthFailure(r, "refresh_credentials", err)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	session, err := a.refreshSession(r.Context(), refreshToken, csrfToken)
	if err != nil {
		logAuthFailure(r, "refresh_session", err)
		clearAuthCookies(w, r)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := a.users.CurrentUser(r.Context(), session.UserID, session.CurrentGroupID)
	if err != nil {
		logAuthFailure(r, "refresh_current_user", err)
		clearAuthCookies(w, r)
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	token, err := a.signToken(tokenClaims{
		UserID:         user.ID,
		CurrentGroupID: user.CurrentGroupID,
		SessionID:      session.ID,
		GroupVersion:   session.GroupVersion,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed")
		return
	}
	setAuthCookies(w, r, refreshToken, csrfToken, session.ExpiresAt)
	slog.InfoContext(r.Context(), "session refreshed",
		"actor_user_id", user.ID,
		"group_id", user.CurrentGroupID,
		"session_id", session.ID,
		"client_ip", clientIP(r),
	)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (a *app) handleLogout(w http.ResponseWriter, r *http.Request) {
	refreshCookie, cookieErr := r.Cookie(refreshCookieName)
	_, _, csrfErr := refreshCredentials(r)
	if cookieErr == nil && csrfErr != nil {
		writeError(w, http.StatusForbidden, "csrf_required")
		return
	}
	if cookieErr == nil {
		if err := a.revokeRefreshSession(r.Context(), refreshCookie.Value); err != nil {
			writeError(w, http.StatusInternalServerError, "logout_failed")
			return
		}
	}
	clearAuthCookies(w, r)
	slog.InfoContext(r.Context(), "session logout",
		"session_present", cookieErr == nil,
		"client_ip", clientIP(r),
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func loginUserResponse(user *userdomain.User, groups []userdomain.Group, currentGroupID uint64) map[string]any {
	return map[string]any{
		"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
		"is_super_admin": user.IsSuperAdmin, "default_group_id": nullableUint64Value(user.DefaultGroupID),
		"must_change_password": user.MustChangePassword,
		"current_group_id":     currentGroupID, "study_groups": groups,
		"current_tenant_id": currentTenantID(groups, currentGroupID),
		"is_tenant_admin":   currentTenantAdmin(groups, currentGroupID),
	}
}

func currentTenantID(groups []userdomain.Group, groupID uint64) uint64 {
	for _, group := range groups {
		if group.ID == groupID {
			return group.TenantID
		}
	}
	return 0
}

func currentTenantAdmin(groups []userdomain.Group, groupID uint64) bool {
	for _, group := range groups {
		if group.ID == groupID {
			return group.TenantAdmin
		}
	}
	return false
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
	claims, err := a.verifyToken(bearerToken(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	user, err := a.users.CurrentUser(r.Context(), u.ID, req.GroupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user_failed")
		return
	}
	groupVersion, err := a.updateRefreshSessionGroup(
		r.Context(),
		claims.SessionID,
		u.ID,
		claims.GroupVersion,
		req.GroupID,
	)
	if err != nil {
		if errors.Is(err, errRefreshSessionGroupChanged) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "session_failed")
		return
	}
	token, err := a.signToken(tokenClaims{
		UserID:         u.ID,
		CurrentGroupID: req.GroupID,
		SessionID:      claims.SessionID,
		GroupVersion:   groupVersion,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
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
		a.auditChanges(
			u.CurrentGroupID,
			u.ID,
			"update_default_group",
			"users",
			u.ID,
			map[string]any{"default_group_id": u.DefaultGroupID},
			map[string]any{"default_group_id": uint64(0)},
			r,
		)
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
	a.auditChanges(
		req.GroupID,
		u.ID,
		"update_default_group",
		"users",
		u.ID,
		map[string]any{"default_group_id": u.DefaultGroupID},
		map[string]any{"default_group_id": req.GroupID},
		r,
	)
	u.DefaultGroupID = req.GroupID
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": u})
}

func (a *app) handleUpdatePersonalSettings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req userdomain.PersonalSettings
	if !readJSON(w, r, &req) {
		return
	}
	settings, err := a.users.UpdatePersonalSettings(r.Context(), u.ID, groupID, req, time.Now().UTC())
	switch {
	case errors.Is(err, userdomain.ErrMemberNameRequired),
		errors.Is(err, userdomain.ErrMemberNameTooLong),
		errors.Is(err, userdomain.ErrInvalidMobileViewMode):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, userdomain.ErrMemberNotFound):
		writeError(w, http.StatusForbidden, "forbidden")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "personal_settings_failed")
	default:
		a.auditChanges(
			groupID,
			u.ID,
			"update_personal_settings",
			"group_members",
			u.ID,
			userdomain.PersonalSettings{
				MemberName:     u.MemberName,
				MobileViewMode: u.MobileViewMode,
			},
			settings,
			r,
		)
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
	}
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
	if err := a.users.UpdatePassword(r.Context(), u.ID, oldHash, hash, time.Now().UTC()); err != nil {
		if errors.Is(err, userdomain.ErrPasswordChanged) {
			writeError(w, http.StatusUnauthorized, "invalid_password")
			return
		}
		writeError(w, http.StatusInternalServerError, "password_save_failed")
		return
	}
	a.audit(
		u.CurrentGroupID,
		u.ID,
		"change_password",
		"users",
		u.ID,
		nil,
		map[string]any{"password_changed": true, "sessions_revoked": true},
		r,
	)
	clearAuthCookies(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	members, err := a.listMembers(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bootstrap_failed")
		return
	}
	date := queryDate(r, "date", time.Now().In(a.location))
	week, err := a.currentWeekAt(r.Context(), groupID, date)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "bootstrap_failed")
		return
	}
	var tasks []map[string]any
	if week != nil {
		if weekID, ok := week["id"].(uint64); ok {
			tasks, err = a.weekTasks(r.Context(), groupID, weekID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "bootstrap_failed")
				return
			}
		}
	}
	learningConfig, err := a.groupLearningConfig(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bootstrap_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":            u,
		"members":         members,
		"current_week":    week,
		"current_tasks":   tasks,
		"learning_config": learningConfig,
	})
}
