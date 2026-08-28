package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	userdomain "agp/backend/internal/user"
)

func (a *app) handleSuperListGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.users.AllGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "groups_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"study_groups": groups})
}

func (a *app) handleSuperCreateGroup(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	password := randomPassword(8)
	hash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	id, err := a.users.CreateGroup(r.Context(), req.Name, hash, u.ID, time.Now().UTC())
	if errors.Is(err, userdomain.ErrGroupNameRequired) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "default_password": password})
}

func (a *app) handleSuperUpdateGroup(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	err := a.users.UpdateGroup(r.Context(), groupID, req.Name, time.Now().UTC())
	if errors.Is(err, userdomain.ErrGroupNameRequired) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, userdomain.ErrGroupNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	a.audit(groupID, u.ID, "update_group", "study_groups", groupID, nil, map[string]any{
		"name": req.Name,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleSuperSetGroupDefaultPassword(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
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
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "affected_users": affected})
}

func (a *app) handleSuperListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.users.ListUsers(r.Context(), 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "users_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *app) handleSuperCreateUser(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		Username     string `json:"username"`
		DisplayName  string `json:"display_name"`
		NamePinyin   string `json:"name_pinyin"`
		Password     string `json:"password"`
		GroupID      uint64 `json:"group_id"`
		Role         string `json:"role"`
		IsSuperAdmin bool   `json:"is_super_admin"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var hash string
	initialPassword := ""
	if req.Password == "" && req.GroupID > 0 {
		var err error
		hash, err = a.groupDefaultPasswordHash(r.Context(), req.GroupID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "password_required")
			return
		}
	} else {
		password := req.Password
		if password == "" {
			password = randomPassword(8)
			initialPassword = password
		}
		var err error
		hash, err = hashPassword(password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password_failed")
			return
		}
	}
	id, err := a.users.CreateUserWithMembership(
		r.Context(),
		req.Username,
		req.DisplayName,
		req.NamePinyin,
		hash,
		req.IsSuperAdmin,
		req.GroupID,
		req.Role,
		u.ID,
		time.Now().UTC(),
	)
	if errors.Is(err, userdomain.ErrUsernameDisplayNameRequired) || errors.Is(err, userdomain.ErrInvalidRole) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "user_create_failed")
		return
	}
	resp := map[string]any{"id": id}
	if initialPassword != "" {
		resp["initial_password"] = initialPassword
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (a *app) handleSuperResetAllPasswords(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		Password string `json:"password"`
		Confirm  string `json:"confirm"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Confirm != "RESET_ALL_NON_SUPER_ADMINS" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "confirmation_required")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	affected, err := a.users.ResetNonSuperPasswords(r.Context(), hash, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reset_failed")
		return
	}
	a.audit(0, u.ID, "reset_all_passwords", "users", 0, nil, map[string]any{"affected": affected}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "affected_users": affected})
}

func (a *app) handleSuperAddGroupMember(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var req struct {
		UserID     uint64 `json:"user_id"`
		MemberName string `json:"member_name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := a.addMember(r.Context(), groupID, req.UserID, req.MemberName, u.ID); err != nil {
		writeError(w, http.StatusConflict, "member_add_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (a *app) handleSuperSetLeader(w http.ResponseWriter, r *http.Request) {
	a.superSetLeaderRole(w, r, true)
}

func (a *app) handleSuperUnsetLeader(w http.ResponseWriter, r *http.Request) {
	a.superSetLeaderRole(w, r, false)
}

func (a *app) superSetLeaderRole(w http.ResponseWriter, r *http.Request, grant bool) {
	groupID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var userID uint64
	if grant {
		var req struct {
			UserID uint64 `json:"user_id"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		userID = req.UserID
	} else {
		userID, _ = strconv.ParseUint(r.PathValue("user_id"), 10, 64)
	}
	if err := a.users.SetUserRole(r.Context(), groupID, userID, userdomain.RoleGroupLeader, grant, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "role_save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
