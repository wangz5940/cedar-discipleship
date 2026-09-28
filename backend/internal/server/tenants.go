package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
)

type tenantItem struct {
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Status     int    `json:"status"`
	GroupCount int    `json:"group_count"`
}

func (a *app) requireTenantAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := mustUser(r)
		tenantID := pathUint64(r, "tenant_id")
		if tenantID == 0 || (!u.IsSuperAdmin && (!u.IsTenantAdmin || u.CurrentTenantID != tenantID)) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	}
}

func (a *app) requireCurrentTenantAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := mustUser(r)
		if !u.IsTenantAdmin || u.CurrentGroupID == 0 {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		next(w, r)
	}
}

func (a *app) handleTenants(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	query := `SELECT t.id,t.name,COALESCE(tm.role,''),t.status,
		(SELECT COUNT(*) FROM study_groups g WHERE g.tenant_id=t.id AND g.status=1) FROM tenants t
		LEFT JOIN tenant_members tm ON tm.tenant_id=t.id AND tm.user_id=? AND tm.status=1
		WHERE t.status=1 AND (tm.user_id IS NOT NULL OR ?) ORDER BY t.id`
	rows, err := a.db.QueryContext(r.Context(), query, u.ID, u.IsSuperAdmin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenants_failed")
		return
	}
	defer rows.Close()
	items := []tenantItem{}
	for rows.Next() {
		var item tenantItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Role, &item.Status, &item.GroupCount); err != nil {
			writeError(w, http.StatusInternalServerError, "tenants_failed")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "tenants_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": items})
}

func (a *app) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		writeError(w, http.StatusBadRequest, "name_required")
		return
	}
	now := time.Now().UTC()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO tenants(name,status,created_by,created_at,updated_at) VALUES(?,1,?,?,?)`, req.Name, mustUser(r).ID, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	tenantID, err := insertedID(res)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": tenantID})
}

func (a *app) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		writeError(w, http.StatusBadRequest, "name_required")
		return
	}
	tenantID := pathUint64(r, "tenant_id")
	res, err := a.db.ExecContext(r.Context(), `UPDATE tenants SET name=?,updated_at=? WHERE id=? AND status=1`, req.Name, time.Now().UTC(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_update_failed")
		return
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		writeError(w, http.StatusNotFound, "tenant_not_found")
		return
	}
	a.audit(0, mustUser(r).ID, "update_tenant", "tenants", tenantID, nil, map[string]any{"name": req.Name}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := pathUint64(r, "tenant_id")
	if tenantID == 1 {
		writeError(w, http.StatusConflict, "original_tenant_protected")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		return
	}
	defer tx.Rollback()
	var lockedID uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM tenants WHERE id=? AND status=1 FOR UPDATE`, tenantID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "tenant_not_found")
		} else {
			writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		}
		return
	}
	var hasGroups bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM study_groups WHERE tenant_id=?)`, tenantID).Scan(&hasGroups); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		return
	}
	if hasGroups {
		writeError(w, http.StatusConflict, "tenant_has_groups")
		return
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(r.Context(), `UPDATE tenant_members SET status=0,updated_at=? WHERE tenant_id=?`, now, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE tenants SET status=0,updated_at=? WHERE id=?`, now, tenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_delete_failed")
		return
	}
	a.audit(0, mustUser(r).ID, "delete_tenant", "tenants", tenantID, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleMoveGroupTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID uint64 `json:"tenant_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.TenantID == 0 {
		writeError(w, http.StatusBadRequest, "tenant_id_required")
		return
	}
	groupID := pathUint64(r, "group_id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	defer tx.Rollback()
	var sourceTenantID uint64
	var groupName string
	if err := tx.QueryRowContext(r.Context(), `SELECT g.tenant_id,g.name FROM study_groups g
		JOIN tenants t ON t.id=g.tenant_id AND t.status=1
		WHERE g.id=? AND g.status=1 FOR UPDATE`, groupID).Scan(&sourceTenantID, &groupName); err != nil {
		writeError(w, http.StatusNotFound, "group_not_found")
		return
	}
	if sourceTenantID == req.TenantID {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var targetID uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM tenants WHERE id=? AND status=1 FOR UPDATE`, req.TenantID).Scan(&targetID); err != nil {
		writeError(w, http.StatusNotFound, "tenant_not_found")
		return
	}
	var duplicate bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM study_groups WHERE tenant_id=? AND name=? AND status=1)`, targetID, groupName).Scan(&duplicate); err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	if duplicate {
		writeError(w, http.StatusConflict, "group_name_exists")
		return
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		SELECT ?,m.user_id,'member',1,?,? FROM group_members m JOIN users u ON u.id=m.user_id AND u.status=1
		WHERE m.group_id=? AND m.status=1
		ON DUPLICATE KEY UPDATE role=IF(tenant_members.status=1,tenant_members.role,'member'),status=1,updated_at=VALUES(updated_at)`,
		targetID, now, now, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE study_groups SET tenant_id=?,updated_at=? WHERE id=?`, targetID, now, groupID); err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE tenant_members tm
		JOIN group_members moved ON moved.user_id=tm.user_id AND moved.group_id=? AND moved.status=1
		SET tm.status=0,tm.updated_at=?
		WHERE tm.tenant_id=? AND tm.role='member' AND tm.status=1
		  AND NOT EXISTS (
			SELECT 1 FROM group_members gm
			JOIN study_groups g ON g.id=gm.group_id AND g.tenant_id=? AND g.status=1
			WHERE gm.user_id=tm.user_id AND gm.status=1
		  )`, groupID, now, sourceTenantID, sourceTenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	a.audit(groupID, mustUser(r).ID, "move_group_tenant", "study_groups", groupID, map[string]any{"tenant_id": sourceTenantID}, map[string]any{"tenant_id": targetID}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleTenantGroups(w http.ResponseWriter, r *http.Request) {
	tenantID := pathUint64(r, "tenant_id")
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,code,name FROM study_groups WHERE tenant_id=? AND status=1 ORDER BY id`, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "groups_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uint64
		var code, name string
		if err := rows.Scan(&id, &code, &name); err != nil {
			writeError(w, http.StatusInternalServerError, "groups_failed")
			return
		}
		items = append(items, map[string]any{"id": id, "code": code, "name": name})
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "groups_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"study_groups": items})
}

func (a *app) handleTenantCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 128 {
		writeError(w, http.StatusBadRequest, "group_name_required")
		return
	}
	tenantID := pathUint64(r, "tenant_id")
	var duplicate bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM study_groups WHERE tenant_id=? AND name=? AND status=1)`, tenantID, req.Name).Scan(&duplicate); err != nil {
		writeError(w, http.StatusInternalServerError, "group_create_failed")
		return
	}
	if duplicate {
		writeError(w, http.StatusConflict, "group_name_exists")
		return
	}
	password := randomPassword(12)
	hash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	code, err := randomURLToken(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_code_failed")
		return
	}
	now := time.Now().UTC()
	res, err := a.db.ExecContext(r.Context(), `INSERT INTO study_groups(tenant_id,code,name,description,default_password_hash,auto_seed_ministry_catalog,created_by,created_at,updated_at)
		SELECT id,?,?,'',?,0,?,?,? FROM tenants WHERE id=? AND status=1`, "group-"+strings.ToLower(code), req.Name, hash, mustUser(r).ID, now, now, tenantID)
	if err != nil {
		writeError(w, http.StatusConflict, "group_create_failed")
		return
	}
	if affected, err := res.RowsAffected(); err != nil || affected != 1 {
		writeError(w, http.StatusNotFound, "tenant_not_found")
		return
	}
	id, err := insertedID(res)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_create_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "default_password": password})
}

func (a *app) tenantGroupID(w http.ResponseWriter, r *http.Request) uint64 {
	groupID := pathUint64(r, "group_id")
	var same bool
	err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM study_groups WHERE id=? AND tenant_id=? AND status=1)`, groupID, pathUint64(r, "tenant_id")).Scan(&same)
	if err != nil || !same {
		writeError(w, http.StatusNotFound, "group_not_found")
		return 0
	}
	return groupID
}

func (a *app) handleTenantUpdateGroup(w http.ResponseWriter, r *http.Request) {
	groupID := a.tenantGroupID(w, r)
	if groupID == 0 {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if err := a.users.UpdateGroup(r.Context(), groupID, req.Name, time.Now().UTC()); err != nil {
		writeError(w, http.StatusConflict, "group_update_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleTenantDeleteGroup(w http.ResponseWriter, r *http.Request) {
	groupID := a.tenantGroupID(w, r)
	if groupID == 0 {
		return
	}
	paths, err := a.users.DeleteGroup(r.Context(), groupID, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_delete_failed")
		return
	}
	if err := a.deleteOwnedResourceFiles(r.Context(), paths); err != nil {
		writeError(w, http.StatusInternalServerError, "group_resource_delete_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleTenantAdmins(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT u.id,u.username,u.display_name
		FROM tenant_members tm JOIN users u ON u.id=tm.user_id
		WHERE tm.tenant_id=? AND tm.role='admin' AND tm.status=1 AND u.status=1 ORDER BY u.id`, pathUint64(r, "tenant_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "admins_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uint64
		var username, displayName string
		if err := rows.Scan(&id, &username, &displayName); err != nil {
			writeError(w, http.StatusInternalServerError, "admins_failed")
			return
		}
		items = append(items, map[string]any{"user_id": id, "username": username, "display_name": displayName})
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "admins_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admins": items})
}

func (a *app) handleCreateTenantAdmin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Username == "" || len([]rune(req.Username)) > 64 || req.DisplayName == "" || len([]rune(req.DisplayName)) > 128 {
		writeError(w, http.StatusBadRequest, "username_display_name_required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password_too_short")
		return
	}
	for _, char := range req.Username {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '_' && char != '-' && char != '.' {
			writeError(w, http.StatusBadRequest, "invalid_username")
			return
		}
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "password_failed")
		return
	}
	tenantID := pathUint64(r, "tenant_id")
	now := time.Now().UTC()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_create_failed")
		return
	}
	defer tx.Rollback()
	var lockedTenantID uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM tenants WHERE id=? AND status=1 FOR UPDATE`, tenantID).Scan(&lockedTenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "tenant_not_found")
		} else {
			writeError(w, http.StatusInternalServerError, "tenant_admin_create_failed")
		}
		return
	}
	res, err := tx.ExecContext(r.Context(), `INSERT INTO users(username,display_name,name_pinyin,password_hash,is_super_admin,created_by,created_at,updated_at)
		VALUES(?,?,?,?,0,?,?,?)`, req.Username, req.DisplayName, req.Username, hash, mustUser(r).ID, now, now)
	if err != nil {
		writeError(w, http.StatusConflict, "user_create_failed")
		return
	}
	adminID, err := insertedID(res)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_create_failed")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES(?,?,'admin',1,?,?)`, tenantID, adminID, now, now); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_create_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_create_failed")
		return
	}
	a.audit(0, mustUser(r).ID, "create_tenant_admin", "users", adminID, nil, map[string]any{"tenant_id": tenantID}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": adminID})
}

func (a *app) handleUpdateTenantAdmin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" || len([]rune(req.DisplayName)) > 128 {
		writeError(w, http.StatusBadRequest, "display_name_required")
		return
	}
	if req.Password != "" && len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password_too_short")
		return
	}
	var hash string
	if req.Password != "" {
		var err error
		hash, err = hashPassword(req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password_failed")
			return
		}
	}
	tenantID, adminID := pathUint64(r, "tenant_id"), pathUint64(r, "user_id")
	now := time.Now().UTC()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_update_failed")
		return
	}
	defer tx.Rollback()
	var lockedID uint64
	err = tx.QueryRowContext(r.Context(), `SELECT u.id FROM users u
		JOIN tenant_members tm ON tm.user_id=u.id
		WHERE u.id=? AND u.status=1 AND u.is_super_admin=0
		AND tm.tenant_id=? AND tm.role='admin' AND tm.status=1 FOR UPDATE`, adminID, tenantID).Scan(&lockedID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "tenant_admin_not_found")
		} else {
			writeError(w, http.StatusInternalServerError, "tenant_admin_update_failed")
		}
		return
	}
	if hash == "" {
		_, err = tx.ExecContext(r.Context(), `UPDATE users SET display_name=?,updated_at=? WHERE id=?`, req.DisplayName, now, adminID)
	} else {
		_, err = tx.ExecContext(r.Context(), `UPDATE users SET display_name=?,password_hash=?,must_change_password=0,updated_at=? WHERE id=?`, req.DisplayName, hash, now, adminID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_update_failed")
		return
	}
	if hash != "" {
		if _, err := tx.ExecContext(r.Context(), `UPDATE refresh_sessions SET revoked_at=?,updated_at=? WHERE user_id=? AND revoked_at IS NULL`, now, now, adminID); err != nil {
			writeError(w, http.StatusInternalServerError, "tenant_admin_update_failed")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_admin_update_failed")
		return
	}
	a.audit(0, mustUser(r).ID, "update_tenant_admin", "users", adminID, nil, map[string]any{"tenant_id": tenantID, "display_name": req.DisplayName, "password_changed": hash != ""}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
