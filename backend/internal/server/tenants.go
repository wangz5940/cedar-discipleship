package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
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

func (a *app) handleTenantLeader(w http.ResponseWriter, r *http.Request) {
	groupID := a.tenantGroupID(w, r)
	if groupID == 0 {
		return
	}
	userID := pathUint64(r, "user_id")
	if userID == 0 {
		writeError(w, http.StatusBadRequest, "user_id_required")
		return
	}
	var member bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=? AND user_id=? AND status=1)`, groupID, userID).Scan(&member); err != nil || !member {
		writeError(w, http.StatusNotFound, "member_not_found")
		return
	}
	if err := a.users.SetUserRole(r.Context(), groupID, userID, "group_leader", r.Method == http.MethodPut, time.Now().UTC()); err != nil {
		writeError(w, http.StatusInternalServerError, "role_save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		Name        string `json:"name"`
		GroupName   string `json:"group_name"`
		AdminUserID uint64 `json:"admin_user_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.GroupName = strings.TrimSpace(req.GroupName)
	if req.Name == "" || len([]rune(req.Name)) > 128 || len([]rune(req.GroupName)) > 128 {
		writeError(w, http.StatusBadRequest, "name_required")
		return
	}
	var password, hash, code string
	if req.GroupName != "" {
		password = randomPassword(12)
		var err error
		hash, err = hashPassword(password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "password_failed")
			return
		}
		code, err = randomURLToken(12)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "group_code_failed")
			return
		}
	}
	now := time.Now().UTC()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO tenants(name,status,created_by,created_at,updated_at) VALUES(?,1,?,?,?)`, req.Name, mustUser(r).ID, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	tenantID, err := insertedID(res)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	var groupID uint64
	if req.GroupName != "" {
		res, err = tx.ExecContext(r.Context(), `INSERT INTO study_groups(tenant_id,code,name,description,default_password_hash,auto_seed_ministry_catalog,created_by,created_at,updated_at)
			VALUES(?,?,?,'',?,0,?,?,?)`, tenantID, "group-"+strings.ToLower(code), req.GroupName, hash, mustUser(r).ID, now, now)
		if err != nil {
			writeError(w, http.StatusConflict, "group_create_failed")
			return
		}
		groupID, err = insertedID(res)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "group_create_failed")
			return
		}
	}
	if req.AdminUserID != 0 {
		res, err = tx.ExecContext(r.Context(), `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
			SELECT ?,u.id,'admin',1,?,? FROM users u WHERE u.id=? AND u.status=1`, tenantID, now, now, req.AdminUserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tenant_admin_failed")
			return
		}
		count, _ := res.RowsAffected()
		if count != 1 {
			writeError(w, http.StatusBadRequest, "admin_user_not_found")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "tenant_create_failed")
		return
	}
	result := map[string]any{"id": tenantID}
	if groupID != 0 {
		result["group_id"] = groupID
		result["default_password"] = password
	}
	writeJSON(w, http.StatusCreated, result)
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
	var linked bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(
		SELECT 1 FROM asset_dependencies WHERE status='active' AND
			((consumer_group_id=? AND provider_group_id<>?) OR (provider_group_id=? AND consumer_group_id<>?))
		UNION ALL
		SELECT 1 FROM asset_share_grants WHERE status='active' AND consumer_group_id IS NOT NULL AND
			((owner_group_id=? AND consumer_group_id<>?) OR (consumer_group_id=? AND owner_group_id<>?))
		UNION ALL
		SELECT 1 FROM asset_bindings b JOIN assets source ON source.id=b.source_asset_id
			WHERE b.deleted_at IS NULL AND
			((b.group_id=? AND source.group_id<>?) OR (source.group_id=? AND b.group_id<>?))
	)`, groupID, groupID, groupID, groupID, groupID, groupID, groupID, groupID, groupID, groupID, groupID, groupID).Scan(&linked)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "group_move_failed")
		return
	}
	if linked {
		writeError(w, http.StatusConflict, "group_has_cross_group_resources")
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

func (a *app) handleTenantMembers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.QueryContext(r.Context(), `SELECT u.id,u.username,u.display_name,tm.role
		FROM tenant_members tm JOIN users u ON u.id=tm.user_id
		WHERE tm.tenant_id=? AND tm.status=1 ORDER BY u.id`, pathUint64(r, "tenant_id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "members_failed")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id uint64
		var username, displayName, role string
		if err := rows.Scan(&id, &username, &displayName, &role); err != nil {
			writeError(w, http.StatusInternalServerError, "members_failed")
			return
		}
		items = append(items, map[string]any{"user_id": id, "username": username, "display_name": displayName, "role": role})
	}
	if rows.Err() != nil {
		writeError(w, http.StatusInternalServerError, "members_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": items})
}

func (a *app) handleTenantSetMember(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID uint64 `json:"user_id"`
		Role   string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.UserID == 0 || (req.Role != "admin" && req.Role != "member") {
		writeError(w, http.StatusBadRequest, "invalid_member")
		return
	}
	u := mustUser(r)
	tenantID := pathUint64(r, "tenant_id")
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	defer tx.Rollback()
	var lockedTenantID uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM tenants WHERE id=? AND status=1 FOR UPDATE`, tenantID).Scan(&lockedTenantID); err != nil {
		writeError(w, http.StatusNotFound, "tenant_not_found")
		return
	}
	if !u.IsSuperAdmin {
		var existing bool
		if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM tenant_members WHERE tenant_id=? AND user_id=? AND status=1)`, tenantID, req.UserID).Scan(&existing); err != nil || !existing {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}
	var accountExists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=? AND status=1)`, req.UserID).Scan(&accountExists); err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	if !accountExists {
		writeError(w, http.StatusNotFound, "member_not_found")
		return
	}
	if req.Role == "member" {
		var adminCount int
		var priorRole string
		if err := tx.QueryRowContext(r.Context(), `SELECT role FROM tenant_members WHERE tenant_id=? AND user_id=? AND status=1`, tenantID, req.UserID).Scan(&priorRole); err == nil && priorRole == "admin" {
			if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tenant_members WHERE tenant_id=? AND role='admin' AND status=1`, tenantID).Scan(&adminCount); err != nil || adminCount <= 1 {
				writeError(w, http.StatusConflict, "last_tenant_admin")
				return
			}
		}
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES(?,?,?,1,?,?)
		ON DUPLICATE KEY UPDATE role=VALUES(role),status=1,updated_at=VALUES(updated_at)`, tenantID, req.UserID, req.Role, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "member_save_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleTenantRemoveMember(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	tenantID := pathUint64(r, "tenant_id")
	userID := pathUint64(r, "user_id")
	if userID == u.ID {
		writeError(w, http.StatusBadRequest, "cannot_remove_self")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "member_remove_failed")
		return
	}
	defer tx.Rollback()
	var lockedTenantID uint64
	if err := tx.QueryRowContext(r.Context(), `SELECT id FROM tenants WHERE id=? AND status=1 FOR UPDATE`, tenantID).Scan(&lockedTenantID); err != nil {
		writeError(w, http.StatusNotFound, "tenant_not_found")
		return
	}
	var role string
	if err := tx.QueryRowContext(r.Context(), `SELECT role FROM tenant_members WHERE tenant_id=? AND user_id=? AND status=1 FOR UPDATE`, tenantID, userID).Scan(&role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "member_not_found")
		} else {
			writeError(w, http.StatusInternalServerError, "member_remove_failed")
		}
		return
	}
	if role == "admin" {
		var count int
		if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM tenant_members WHERE tenant_id=? AND role='admin' AND status=1`, tenantID).Scan(&count); err != nil || count <= 1 {
			writeError(w, http.StatusConflict, "last_tenant_admin")
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE tenant_members SET status=0,updated_at=? WHERE tenant_id=? AND user_id=?`, time.Now().UTC(), tenantID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "member_remove_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "member_remove_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
