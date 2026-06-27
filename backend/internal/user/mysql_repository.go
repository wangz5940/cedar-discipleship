package user

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
	if err != nil {
		return nil, err
	}
	if defaultGroupID.Valid && defaultGroupID.Int64 > 0 {
		item.DefaultGroupID = uint64(defaultGroupID.Int64)
	}
	return &item, nil
}

func (r *MySQLRepository) ListAllGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,code,name FROM study_groups ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroups(rows)
}

func (r *MySQLRepository) CreateGroup(ctx context.Context, code, name, description, passwordHash string, actorID uint64, at time.Time) (uint64, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO study_groups (code,name,description,default_password_hash,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`, code, name, description, passwordHash, actorID, at, at)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
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
	rows, err := r.db.QueryContext(ctx, `SELECT g.id,g.code,g.name FROM study_groups g JOIN group_members m ON m.group_id=g.id WHERE m.user_id=? AND m.status=1 AND g.status=1 ORDER BY g.id`, userID)
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
		roles, err := r.ListRoles(ctx, member.UserID, groupID)
		if err != nil {
			return nil, err
		}
		member.Roles = roles
		members = append(members, member)
	}
	return members, rows.Err()
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
			return 0, fmt.Errorf("%w: %v", ErrUserCreateFailed, err)
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
	res, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash=?, must_change_password=1, updated_at=? WHERE is_super_admin=0`, passwordHash, at)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	return affected, nil
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
	res, err := tx.ExecContext(ctx, `UPDATE users u
		JOIN group_members m ON m.user_id=u.id AND m.group_id=?
		LEFT JOIN user_group_roles r ON r.user_id=u.id AND r.group_id=? AND r.role=?
		SET u.password_hash=?, u.must_change_password=1, u.updated_at=?
		WHERE u.is_super_admin=0
		  AND r.id IS NULL
		  AND (SELECT COUNT(*) FROM group_members gm WHERE gm.user_id=u.id AND gm.status=1)=1`, groupID, groupID, RoleGroupLeader, passwordHash, at)
	if err != nil {
		return 0, err
	}
	affected, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
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

func (r *MySQLRepository) CreateUserWithHash(ctx context.Context, username, displayName, namePinyin, passwordHash string, isSuperAdmin bool, actorID uint64, at time.Time) (uint64, error) {
	return createUserWithHash(ctx, r.db, username, displayName, namePinyin, passwordHash, isSuperAdmin, actorID, at)
}

func (r *MySQLRepository) AddMember(ctx context.Context, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	return addMember(ctx, r.db, groupID, userID, memberName, actorID, at)
}

func (r *MySQLRepository) UpdateLastLogin(ctx context.Context, userID uint64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, at, userID)
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

func (r *MySQLRepository) UpdatePassword(ctx context.Context, userID uint64, passwordHash string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`, passwordHash, at, userID)
	return err
}

func (r *MySQLRepository) Save(ctx context.Context, user *User) error {
	return nil
}

func (r *MySQLRepository) allGroups(ctx context.Context) ([]Group, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,code,name FROM study_groups WHERE status=1 ORDER BY id`)
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
		if err := rows.Scan(&group.ID, &group.Code, &group.Name); err != nil {
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
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func addMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	return addMember(ctx, tx, groupID, userID, memberName, actorID, at)
}

func addMember(ctx context.Context, execer execer, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	_, err := execer.ExecContext(ctx, `INSERT INTO group_members (group_id,user_id,member_name,joined_at,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=1, updated_at=VALUES(updated_at)`, groupID, userID, memberName, at, actorID, at, at)
	return err
}
