package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) FindByID(ctx context.Context, id uint64) (*User, error) {
	var item User
	var defaultGroupID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id, username, display_name, is_super_admin, default_group_id, must_change_password, status FROM users WHERE id=?`, id).
		Scan(&item.ID, &item.Username, &item.DisplayName, &item.IsSuperAdmin, &defaultGroupID, &item.MustChangePassword, &item.Status)
	if err != nil {
		return nil, err
	}
	if defaultGroupID.Valid && defaultGroupID.Int64 > 0 {
		item.DefaultGroupID = uint64(defaultGroupID.Int64)
	}
	return &item, nil
}

func (r *MySQLRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	var item User
	var defaultGroupID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `SELECT id, username, display_name, password_hash, is_super_admin, default_group_id, must_change_password, status FROM users WHERE username = ?`, username).
		Scan(&item.ID, &item.Username, &item.DisplayName, &item.PasswordHash, &item.IsSuperAdmin, &defaultGroupID, &item.MustChangePassword, &item.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if defaultGroupID.Valid && defaultGroupID.Int64 > 0 {
		item.DefaultGroupID = uint64(defaultGroupID.Int64)
	}
	return &item, nil
}

func (r *MySQLRepository) ListAllGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT g.id,g.code,g.name,g.tenant_id,t.name,0 FROM study_groups g JOIN tenants t ON t.id=g.tenant_id ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (r *MySQLRepository) CreateGroup(ctx context.Context, name, passwordHash string, actorID uint64, at time.Time) (uint64, error) {
	name = strings.TrimSpace(name)
	if err := r.ensureGroupNameUnique(ctx, 0, name); err != nil {
		return 0, err
	}
	code, err := r.generateGroupCode(ctx)
	if err != nil {
		return 0, err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO study_groups (code,name,description,default_password_hash,auto_seed_ministry_catalog,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, code, name, "", passwordHash, false, actorID, at, at)
	if err != nil {
		return 0, err
	}
	return insertedID(res)
}

func (r *MySQLRepository) UpdateGroup(ctx context.Context, id uint64, name string, at time.Time) error {
	name = strings.TrimSpace(name)
	if err := r.ensureGroupNameUnique(ctx, id, name); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE study_groups SET name=?,updated_at=? WHERE id=? AND status=1`, name, at, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrGroupNotFound
	}
	return nil
}

func (r *MySQLRepository) DeleteGroup(ctx context.Context, id uint64, at time.Time) ([]string, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var groupID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM study_groups WHERE id=? FOR UPDATE`, id).Scan(&groupID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}

	resourcePaths, err := ownedResourcePathsTx(ctx, tx, id)
	if err != nil {
		return nil, err
	}

	statements := []string{
		`DELETE FROM ministry_attendance_records WHERE study_group_id=?`,
		`DELETE FROM ministry_attendance_dates WHERE study_group_id=?`,
		`DELETE FROM ministry_attendance_settings WHERE study_group_id=?`,
		`DELETE FROM ministry_content_deletions WHERE study_group_id=?`,
		`DELETE FROM ministry_share_pins WHERE study_group_id=?`,
		`DELETE FROM ministry_progress_assets WHERE study_group_id=?`,
		`DELETE FROM ministry_notifications WHERE study_group_id=?`,
		`DELETE FROM ministry_group_requests WHERE study_group_id=?`,
		`DELETE FROM ministry_group_members WHERE study_group_id=?`,
		`DELETE FROM ministry_shares WHERE study_group_id=?`,
		`DELETE FROM ministry_progress WHERE study_group_id=?`,
		`DELETE FROM ministry_groups WHERE study_group_id=?`,
		`DELETE FROM asset_import_events
			WHERE target_group_id=?
			   OR imported_asset_id IN (SELECT id FROM assets WHERE group_id=?)
			   OR source_asset_id IN (SELECT id FROM assets WHERE group_id=?)`,
		`DELETE FROM asset_dependencies WHERE consumer_group_id=? OR provider_group_id=?`,
		`DELETE FROM asset_share_grants
			WHERE owner_group_id=?
			   OR consumer_group_id=?
			   OR asset_id IN (SELECT id FROM assets WHERE group_id=?)`,
		`DELETE FROM task_assets
			WHERE group_id=?
			   OR asset_id IN (SELECT id FROM assets WHERE group_id=?)`,
		`DELETE FROM asset_bindings
			WHERE group_id=?
			   OR source_asset_id IN (SELECT id FROM assets WHERE group_id=?)`,
		`DELETE FROM assets WHERE group_id=?`,
		`DELETE FROM checkin_records WHERE group_id=?`,
		`DELETE FROM recite_attempts WHERE group_id=?`,
		`DELETE FROM feedbacks WHERE group_id=?`,
		`DELETE FROM study_tasks WHERE group_id=?`,
		`DELETE FROM study_weeks WHERE group_id=?`,
		`DELETE FROM group_settings WHERE group_id=?`,
		`DELETE FROM user_group_roles WHERE group_id=?`,
		`DELETE FROM group_members WHERE group_id=?`,
		`DELETE FROM login_logs WHERE group_id=?`,
		`DELETE FROM audit_logs WHERE group_id=?`,
		`UPDATE refresh_sessions SET current_group_id=NULL, updated_at=? WHERE current_group_id=?`,
		`UPDATE users SET default_group_id=NULL, updated_at=? WHERE default_group_id=?`,
		`DELETE FROM study_groups WHERE id=?`,
	}
	for _, query := range statements {
		args := groupDeleteArgs(query, id, at)
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return resourcePaths, nil
}

func ownedResourcePathsTx(ctx context.Context, tx *sql.Tx, groupID uint64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT a.storage_path
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
		WHERE a.group_id=? AND b.asset_kind='owned' AND a.storage_path LIKE 'team-%-resources/objects/%'`,
		groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func groupDeleteArgs(query string, groupID uint64, at time.Time) []any {
	count := strings.Count(query, "?")
	args := make([]any, 0, count)
	for i := 0; i < count; i++ {
		if strings.HasPrefix(query, "UPDATE ") && i == 0 {
			args = append(args, at)
			continue
		}
		args = append(args, groupID)
	}
	return args
}

func (r *MySQLRepository) ensureGroupNameUnique(ctx context.Context, ownID uint64, name string) error {
	tenantID := uint64(1)
	if ownID != 0 {
		if err := r.db.QueryRowContext(ctx, `SELECT tenant_id FROM study_groups WHERE id=?`, ownID).Scan(&tenantID); err != nil {
			return err
		}
	}
	var id uint64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM study_groups WHERE tenant_id=? AND name=? AND id<>? LIMIT 1`, tenantID, name, ownID).Scan(&id)
	if err == nil {
		return errors.New("group_name_exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

func (r *MySQLRepository) generateGroupCode(ctx context.Context) (string, error) {
	for range 16 {
		var raw [4]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", err
		}
		code := "group-" + hex.EncodeToString(raw[:])
		var id uint64
		err := r.db.QueryRowContext(ctx, `SELECT id FROM study_groups WHERE code=? LIMIT 1`, code).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return code, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("group_code_generate_failed")
}

func (r *MySQLRepository) ListUsers(ctx context.Context, limit int) ([]UserListItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, display_name, is_super_admin, status FROM users ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []UserListItem
	for rows.Next() {
		var item UserListItem
		if err := rows.Scan(&item.ID, &item.Username, &item.DisplayName, &item.IsSuperAdmin, &item.Status); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) ListGroups(ctx context.Context, userID uint64, isSuperAdmin bool) ([]Group, error) {
	if isSuperAdmin {
		return r.allGroups(ctx)
	}
	return r.ListMembershipGroups(ctx, userID)
}

func (r *MySQLRepository) ListMembershipGroups(ctx context.Context, userID uint64) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT g.id,g.code,g.name,g.tenant_id,t.name,
		(tm.role='admin') FROM study_groups g
		JOIN tenants t ON t.id=g.tenant_id AND t.status=1
		JOIN tenant_members tm ON tm.tenant_id=t.id AND tm.user_id=? AND tm.status=1
		LEFT JOIN group_members m ON m.group_id=g.id AND m.user_id=tm.user_id AND m.status=1
		WHERE g.status=1 AND (tm.role='admin' OR m.id IS NOT NULL) ORDER BY g.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (r *MySQLRepository) ListRoles(ctx context.Context, userID, groupID uint64) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT role FROM user_group_roles WHERE user_id=? AND group_id=?`, userID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	roles = append(roles, RoleMember)
	return roles, nil
}

func (r *MySQLRepository) ListMembers(ctx context.Context, groupID uint64) ([]Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.id,u.id,u.username,u.display_name,m.member_name,u.is_super_admin
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND m.status=1 ORDER BY m.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.MemberID, &member.UserID, &member.Username, &member.DisplayName, &member.MemberName, &member.IsSuperAdmin); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return members, nil
	}
	roleRows, err := r.db.QueryContext(ctx,
		`SELECT user_id,role FROM user_group_roles WHERE group_id=? ORDER BY user_id,role`, groupID)
	if err != nil {
		return nil, err
	}
	defer roleRows.Close()
	roles := make(map[uint64][]string)
	for roleRows.Next() {
		var userID uint64
		var role string
		if err := roleRows.Scan(&userID, &role); err != nil {
			return nil, err
		}
		roles[userID] = append(roles[userID], role)
	}
	if err := roleRows.Err(); err != nil {
		return nil, err
	}
	for i := range members {
		members[i].Roles = append(roles[members[i].UserID], RoleMember)
	}
	return members, nil
}

func (r *MySQLRepository) PersonalSettings(ctx context.Context, userID, groupID uint64) (PersonalSettings, error) {
	var settings PersonalSettings
	err := r.db.QueryRowContext(ctx, `SELECT member_name,COALESCE(NULLIF(mobile_view_mode,''),?)
		FROM group_members
		WHERE user_id=? AND group_id=? AND status=1`,
		MobileViewMasonry, userID, groupID,
	).Scan(&settings.MemberName, &settings.MobileViewMode)
	if errors.Is(err, sql.ErrNoRows) {
		return PersonalSettings{}, ErrMemberNotFound
	}
	return settings, err
}

func (r *MySQLRepository) UpdatePersonalSettings(
	ctx context.Context,
	userID, groupID uint64,
	settings PersonalSettings,
	at time.Time,
) error {
	result, err := r.db.ExecContext(ctx, `UPDATE group_members
		SET member_name=?,mobile_view_mode=?,updated_at=?
		WHERE user_id=? AND group_id=? AND status=1`,
		settings.MemberName, settings.MobileViewMode, at, userID, groupID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected > 0 {
		return err
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM group_members WHERE user_id=? AND group_id=? AND status=1
	)`, userID, groupID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrMemberNotFound
	}
	return nil
}

func (r *MySQLRepository) CreateMember(ctx context.Context, groupID, actorID uint64, input CreateMemberInput) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	userID := input.UserID
	if input.CreateUser {
		hash, err := groupDefaultPasswordHashTx(ctx, tx, groupID)
		if err != nil {
			return 0, ErrGroupDefaultPasswordMissing
		}
		userID, err = createUserWithHashTx(ctx, tx, input.Username, input.DisplayName, firstNonEmpty(input.NamePinyin, input.Username), hash, false, actorID, now)
		if err != nil {
			if isDuplicateKeyError(err) {
				return 0, ErrUsernameExists
			}
			return 0, fmt.Errorf("%w: %v", ErrUserCreateFailed, err)
		}
		if err := ensureTenantMemberTx(ctx, tx, groupID, userID, now); err != nil {
			return 0, err
		}
	} else {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM tenant_members tm JOIN study_groups g ON g.tenant_id=tm.tenant_id
			WHERE g.id=? AND tm.user_id=? AND tm.status=1 AND NOT EXISTS (
				SELECT 1 FROM tenant_members admin WHERE admin.user_id=tm.user_id AND admin.role='admin' AND admin.status=1
			))`, groupID, userID).Scan(&exists); err != nil || !exists {
			return 0, ErrMemberAddFailed
		}
	}
	if userID == 0 {
		return 0, ErrUserIDRequired
	}
	if err := addMemberTx(ctx, tx, groupID, userID, input.DisplayName, actorID, now); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrMemberAddFailed, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *MySQLRepository) AdminMember(ctx context.Context, groupID, memberID uint64) (*AdminMember, error) {
	var member AdminMember
	if err := r.db.QueryRowContext(ctx, `SELECT u.id,u.is_super_admin
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.id=? AND m.group_id=? AND m.status=1`, memberID, groupID).Scan(&member.UserID, &member.IsSuperAdmin); err != nil {
		return nil, err
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM user_group_roles WHERE group_id=? AND user_id=? AND role=?`, groupID, member.UserID, RoleGroupLeader).Scan(&member.LeaderCount); err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *MySQLRepository) RemoveMember(ctx context.Context, groupID, memberID, userID uint64, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE group_members SET status=0, updated_at=? WHERE id=? AND group_id=?`, at, memberID, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_group_roles WHERE group_id=? AND user_id=?`, groupID, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET default_group_id=NULL, updated_at=? WHERE id=? AND default_group_id=?`, at, userID, groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) SetRole(ctx context.Context, groupID, userID uint64, role string, grant bool, at time.Time) error {
	if grant {
		_, err := r.db.ExecContext(ctx, `INSERT IGNORE INTO user_group_roles (group_id,user_id,role,created_at) VALUES (?,?,?,?)`, groupID, userID, role, at)
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM user_group_roles WHERE group_id=? AND user_id=? AND role=?`, groupID, userID, role)
	return err
}

func (r *MySQLRepository) ResetNonSuperPasswords(ctx context.Context, passwordHash string, at time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ids, err := passwordTargetsTx(ctx, tx, `SELECT id FROM users WHERE is_super_admin=0 ORDER BY id FOR UPDATE`)
	if err != nil {
		return 0, err
	}
	affected, err := resetPasswordsTx(ctx, tx, ids, passwordHash, at)
	if err != nil {
		return 0, err
	}
	return affected, tx.Commit()
}

func (r *MySQLRepository) SetGroupDefaultPassword(ctx context.Context, groupID uint64, passwordHash string, at time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE study_groups SET default_password_hash=?, updated_at=? WHERE id=?`, passwordHash, at, groupID); err != nil {
		return 0, err
	}
	ids, err := passwordTargetsTx(ctx, tx, `SELECT u.id FROM users u
		JOIN group_members m ON m.user_id=u.id AND m.group_id=? AND m.status=1
		LEFT JOIN user_group_roles r ON r.user_id=u.id AND r.group_id=? AND r.role=?
		WHERE u.is_super_admin=0
		  AND r.id IS NULL
		  AND (SELECT COUNT(*) FROM group_members gm WHERE gm.user_id=u.id AND gm.status=1)=1
		ORDER BY u.id FOR UPDATE`, groupID, groupID, RoleGroupLeader)
	if err != nil {
		return 0, err
	}
	affected, err := resetPasswordsTx(ctx, tx, ids, passwordHash, at)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

func passwordTargetsTx(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]uint64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Callers lock the selected users before changing credentials or sessions.
func resetPasswordsTx(ctx context.Context, tx *sql.Tx, ids []uint64, passwordHash string, at time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := []any{passwordHash, at}
	for _, id := range ids {
		args = append(args, id)
	}
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?,must_change_password=1,updated_at=?
		WHERE id IN (`+marks+`)`, args...)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	args[0] = at
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=?,updated_at=?
		WHERE revoked_at IS NULL AND user_id IN (`+marks+`)`, args...); err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *MySQLRepository) GroupDefaultPasswordHash(ctx context.Context, groupID uint64) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT default_password_hash FROM study_groups WHERE id=?`, groupID).Scan(&hash)
	return hash, err
}

func (r *MySQLRepository) HasSuperAdmin(ctx context.Context) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_super_admin = 1`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *MySQLRepository) BootstrapSuperAdmin(ctx context.Context, username, displayName, namePinyin, passwordHash string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO users (username, display_name, name_pinyin, password_hash, is_super_admin, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, 1, ?, ?)`, username, displayName, namePinyin, passwordHash, at, at)
	return err
}

func (r *MySQLRepository) CreateUserWithMembership(
	ctx context.Context,
	username, displayName, namePinyin, passwordHash string,
	isSuperAdmin bool,
	groupID uint64,
	role string,
	actorID uint64,
	at time.Time,
) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if groupID > 0 {
		var existingGroupID uint64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM study_groups WHERE id=?`, groupID).Scan(&existingGroupID); err != nil {
			return 0, err
		}
	}
	id, err := createUserWithHashTx(ctx, tx, username, displayName, namePinyin, passwordHash, isSuperAdmin, actorID, at)
	if err != nil {
		return 0, err
	}
	if groupID > 0 {
		if err := ensureTenantMemberTx(ctx, tx, groupID, id, at); err != nil {
			return 0, err
		}
		if err := addMemberTx(ctx, tx, groupID, id, displayName, actorID, at); err != nil {
			return 0, err
		}
		if role != "" {
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_group_roles (group_id,user_id,role,created_at) VALUES (?,?,?,?)`, groupID, id, role, at); err != nil {
				return 0, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MySQLRepository) AddMember(ctx context.Context, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureTenantMemberTx(ctx, tx, groupID, userID, at); err != nil {
		return err
	}
	if err := addMember(ctx, tx, groupID, userID, memberName, actorID, at); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureTenantMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, at time.Time) error {
	var tenantAdmin bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM tenant_members WHERE user_id=? AND role='admin' AND status=1
	)`, userID).Scan(&tenantAdmin); err != nil {
		return err
	}
	if tenantAdmin {
		return ErrMemberAddFailed
	}
	res, err := tx.ExecContext(ctx, `INSERT IGNORE INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		SELECT g.tenant_id,u.id,'member',1,?,? FROM study_groups g JOIN users u ON u.id=? AND u.status=1
		WHERE g.id=? AND g.status=1`, at, at, userID, groupID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM tenant_members tm
			JOIN study_groups g ON g.tenant_id=tm.tenant_id
			WHERE g.id=? AND tm.user_id=? AND tm.status=1)`, groupID, userID).Scan(&active); err != nil || !active {
			return ErrMemberAddFailed
		}
	}
	return nil
}

func (r *MySQLRepository) UpdateLastLogin(ctx context.Context, userID uint64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, at, userID)
	return err
}

func (r *MySQLRepository) CreateLoginLog(ctx context.Context, log LoginLog, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO login_logs
		(user_id, group_id, username, success, failure_reason, ip, user_agent, created_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		nullableID(log.UserID),
		nullableID(log.GroupID),
		log.Username,
		log.Success,
		log.FailureReason,
		log.IP,
		log.UserAgent,
		at,
	)
	return err
}

func (r *MySQLRepository) UpdateDefaultGroup(ctx context.Context, userID, groupID uint64, at time.Time) error {
	if groupID == 0 {
		_, err := r.db.ExecContext(ctx, `UPDATE users SET default_group_id=NULL, updated_at=? WHERE id=?`, at, userID)
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE users SET default_group_id=?, updated_at=? WHERE id=?`, groupID, at, userID)
	return err
}

func (r *MySQLRepository) PasswordHash(ctx context.Context, userID uint64) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash)
	return hash, err
}

func (r *MySQLRepository) UpdatePassword(ctx context.Context, userID uint64, oldHash, passwordHash string, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?,must_change_password=0,updated_at=?
		WHERE id=? AND BINARY password_hash=?`, passwordHash, at, userID, oldHash)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrPasswordChanged
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_sessions SET revoked_at=?,updated_at=?
		WHERE user_id=? AND revoked_at IS NULL`, at, at, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) allGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT g.id,g.code,g.name,g.tenant_id,t.name,0 FROM study_groups g JOIN tenants t ON t.id=g.tenant_id WHERE g.status=1 AND t.status=1 ORDER BY g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func scanGroups(rows *sql.Rows) ([]Group, error) {
	var groups []Group
	for rows.Next() {
		var group Group
		if err := rows.Scan(&group.ID, &group.Code, &group.Name, &group.TenantID, &group.TenantName, &group.TenantAdmin); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func groupDefaultPasswordHashTx(ctx context.Context, tx *sql.Tx, groupID uint64) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT default_password_hash FROM study_groups WHERE id=?`, groupID).Scan(&hash)
	return hash, err
}

func createUserWithHashTx(ctx context.Context, tx *sql.Tx, username, displayName, namePinyin, passwordHash string, isSuperAdmin bool, actorID uint64, at time.Time) (uint64, error) {
	return createUserWithHash(ctx, tx, username, displayName, namePinyin, passwordHash, isSuperAdmin, actorID, at)
}

func createUserWithHash(ctx context.Context, execer execer, username, displayName, namePinyin, passwordHash string, isSuperAdmin bool, actorID uint64, at time.Time) (uint64, error) {
	res, err := execer.ExecContext(ctx, `INSERT INTO users (username,display_name,name_pinyin,password_hash,is_super_admin,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?)`, username, displayName, namePinyin, passwordHash, isSuperAdmin, actorID, at, at)
	if err != nil {
		return 0, err
	}
	return insertedID(res)
}

func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func addMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	return addMember(ctx, tx, groupID, userID, memberName, actorID, at)
}

func addMember(ctx context.Context, execer execer, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	_, err := execer.ExecContext(ctx, `INSERT INTO group_members (group_id,user_id,member_name,joined_at,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=1, updated_at=VALUES(updated_at)`, groupID, userID, memberName, at, actorID, at, at)
	return err
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func insertedID(result sql.Result) (uint64, error) {
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid insert id %d", id)
	}
	return uint64(id), nil
}
