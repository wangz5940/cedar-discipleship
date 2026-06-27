package backup

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode"

	"agp/backend/internal/learning"
)

const (
	roleGroupAdmin  = "group_admin"
	roleGroupLeader = "group_leader"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id,c.logical_date,c.checkin_time,c.task_type,c.part,c.detail,COALESCE(c.note,''),c.is_retro,u.username,COALESCE(m.member_name,u.display_name)
		FROM checkin_records c
		JOIN users u ON u.id=c.user_id
		LEFT JOIN group_members m ON m.group_id=c.group_id AND m.user_id=c.user_id AND m.status=1
		WHERE c.group_id=? AND c.deleted_at IS NULL
		ORDER BY c.logical_date DESC,c.id DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CheckinDetail
	for rows.Next() {
		var item CheckinDetail
		var checkinTime time.Time
		if err := rows.Scan(&item.ID, &item.LogicalDate, &checkinTime, &item.TaskType, &item.Part, &item.Detail, &item.Note, &item.IsRetro, &item.Username, &item.MemberName); err != nil {
			return nil, err
		}
		item.CheckinTime = checkinTime.In(loc).Format("2006-01-02 15:04:05")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) DailySummaries(ctx context.Context, groupID uint64) (int, []DailySummary, error) {
	var activeMembers int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM group_members WHERE group_id=? AND status=1`, groupID).Scan(&activeMembers); err != nil {
		return 0, nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT logical_date,
		COUNT(*) AS total_checkins,
		COUNT(DISTINCT user_id) AS checked_members,
		SUM(CASE WHEN task_type='daily_devotion' THEN 1 ELSE 0 END) AS devotion_count,
		SUM(CASE WHEN task_type='weekly_book' THEN 1 ELSE 0 END) AS book_count,
		SUM(CASE WHEN task_type='weekly_video' THEN 1 ELSE 0 END) AS video_count,
		SUM(CASE WHEN task_type='weekly_verse' THEN 1 ELSE 0 END) AS verse_count
		FROM checkin_records
		WHERE group_id=? AND deleted_at IS NULL
		GROUP BY logical_date
		ORDER BY logical_date DESC`, groupID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	var items []DailySummary
	for rows.Next() {
		var item DailySummary
		if err := rows.Scan(&item.LogicalDate, &item.TotalCheckins, &item.CheckedMembers, &item.DevotionCount, &item.BookCount, &item.VideoCount, &item.VerseCount); err != nil {
			return 0, nil, err
		}
		items = append(items, item)
	}
	return activeMembers, items, rows.Err()
}

func (r *MySQLRepository) FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT f.created_at,COALESCE(u.username,''),f.name,f.contact,f.message,f.page,f.user_agent
		FROM feedbacks f
		LEFT JOIN users u ON u.id=f.user_id
		LEFT JOIN group_members gm ON gm.user_id=f.user_id AND gm.group_id=? AND gm.status=1
		WHERE f.group_id=? OR (f.group_id IS NULL AND gm.id IS NOT NULL)
		ORDER BY f.id DESC`, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FeedbackExport
	for rows.Next() {
		var item FeedbackExport
		var created time.Time
		if err := rows.Scan(&created, &item.Username, &item.Name, &item.Contact, &item.Message, &item.Page, &item.UserAgent); err != nil {
			return nil, err
		}
		item.CreatedAt = created.In(loc).Format("2006-01-02 15:04:05")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) GroupInfo(ctx context.Context, groupID uint64) (*GroupInfo, error) {
	var item GroupInfo
	item.ID = groupID
	err := r.db.QueryRowContext(ctx, `SELECT code,name,description FROM study_groups WHERE id=?`, groupID).Scan(&item.Code, &item.Name, &item.Description)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) BackupMembers(ctx context.Context, groupID uint64) ([]Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,u.display_name,u.name_pinyin
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND m.status=1
		ORDER BY m.member_name,u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roleMap, err := r.memberRoleMap(ctx, groupID)
	if err != nil {
		return nil, err
	}
	var items []Member
	for rows.Next() {
		var item Member
		if err := rows.Scan(&item.Username, &item.DisplayName, &item.NamePinyin); err != nil {
			return nil, err
		}
		item.Roles = roleMap[item.Username]
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupCheckins(ctx context.Context, groupID uint64) ([]Checkin, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,c.logical_date,c.checkin_time,c.task_type,c.part,c.detail,COALESCE(c.note,''),c.is_retro
		FROM checkin_records c JOIN users u ON u.id=c.user_id
		WHERE c.group_id=? AND c.deleted_at IS NULL
		ORDER BY c.logical_date,c.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Checkin
	for rows.Next() {
		var item Checkin
		var checkinTime time.Time
		if err := rows.Scan(&item.Username, &item.LogicalDate, &checkinTime, &item.TaskType, &item.Part, &item.Detail, &item.Note, &item.IsRetro); err != nil {
			return nil, err
		}
		item.CheckinTime = checkinTime.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupFeedbacks(ctx context.Context, groupID uint64) ([]Feedback, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT COALESCE(u.username,''),f.name,f.contact,f.message,f.page,f.user_agent,f.created_at
		FROM feedbacks f
		LEFT JOIN users u ON u.id=f.user_id
		LEFT JOIN group_members gm ON gm.user_id=f.user_id AND gm.group_id=? AND gm.status=1
		WHERE f.group_id=? OR (f.group_id IS NULL AND gm.id IS NOT NULL)
		ORDER BY f.id`, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Feedback
	for rows.Next() {
		var item Feedback
		var created time.Time
		if err := rows.Scan(&item.Username, &item.Name, &item.Contact, &item.Message, &item.Page, &item.UserAgent, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = created.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) BackupAssets(ctx context.Context, groupID uint64) ([]Asset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT category,title,original_name,storage_path,mime_type,file_size FROM assets WHERE group_id=? ORDER BY category,title,id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Asset
	for rows.Next() {
		var item Asset
		if err := rows.Scan(&item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := learning.DeleteAllWeeksTx(ctx, tx, groupID); err != nil {
		return err
	}
	for _, week := range weeks {
		weekID, err := learning.InsertWeekTx(ctx, tx, groupID, week, now)
		if err != nil {
			return err
		}
		if err := learning.ReplaceWeekTasksTx(ctx, tx, groupID, weekID, learning.BuildTaskDrafts(week, ""), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLRepository) ImportLocalBackup(ctx context.Context, groupID, actorID uint64, payload Payload, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := learning.UpsertLearningConfigTx(ctx, tx, groupID, payload.Settings); err != nil {
		return err
	}
	roleAssignments, err := r.importBackupMembersTx(ctx, tx, groupID, actorID, payload.Members)
	if err != nil {
		return err
	}
	if err := r.replaceRolesTx(ctx, tx, groupID, roleAssignments, now); err != nil {
		return err
	}
	if err := learning.DeleteAllWeeksTx(ctx, tx, groupID); err != nil {
		return err
	}
	for _, week := range payload.Weeks {
		weekID, err := learning.InsertWeekTx(ctx, tx, groupID, week, now)
		if err != nil {
			return err
		}
		if err := learning.ReplaceWeekTasksTx(ctx, tx, groupID, weekID, learning.BuildTaskDrafts(week, ""), now); err != nil {
			return err
		}
	}
	userIDs, err := r.usernameMapFromRolesTx(ctx, tx, roleAssignments)
	if err != nil {
		return err
	}
	if err := r.replaceCheckinsTx(ctx, tx, groupID, actorID, userIDs, payload.Checkins, now); err != nil {
		return err
	}
	if err := r.replaceFeedbacksTx(ctx, tx, groupID, userIDs, payload.Feedbacks, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) memberRoleMap(ctx context.Context, groupID uint64) (map[string][]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT u.username,r.role
		FROM user_group_roles r JOIN users u ON u.id=r.user_id
		WHERE r.group_id=? AND r.role IN (?,?)
		ORDER BY u.username,r.role`, groupID, roleGroupAdmin, roleGroupLeader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roleMap := map[string][]string{}
	for rows.Next() {
		var username, role string
		if err := rows.Scan(&username, &role); err != nil {
			return nil, err
		}
		roleMap[username] = append(roleMap[username], role)
	}
	return roleMap, rows.Err()
}

func (r *MySQLRepository) importBackupMembersTx(ctx context.Context, tx *sql.Tx, groupID, actorID uint64, members []Member) (map[uint64][]string, error) {
	roleAssignments := map[uint64][]string{}
	for _, member := range members {
		userID, err := ensureGroupMemberUserTx(ctx, tx, groupID, member, actorID)
		if err != nil {
			return nil, err
		}
		roleAssignments[userID] = append([]string{}, member.Roles...)
	}
	return roleAssignments, nil
}

func ensureGroupMemberUserTx(ctx context.Context, tx *sql.Tx, groupID uint64, member Member, actorID uint64) (uint64, error) {
	username := normalizeUsername(member.Username)
	if username == "" {
		return 0, errors.New("username_required")
	}
	displayName := firstNonEmpty(strings.TrimSpace(member.DisplayName), username)
	namePinyin := firstNonEmpty(strings.TrimSpace(member.NamePinyin), username)
	var userID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username=?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		hash, err := groupDefaultPasswordHashTx(ctx, tx, groupID)
		if err != nil {
			return 0, err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO users (username,display_name,name_pinyin,password_hash,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?)`, username, displayName, namePinyin, hash, actorID, nowSQL(), nowSQL())
		if err != nil {
			return 0, err
		}
		id64, _ := res.LastInsertId()
		userID = uint64(id64)
	} else if err != nil {
		return 0, err
	}
	if err := addMemberTx(ctx, tx, groupID, userID, displayName, actorID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET display_name=?, name_pinyin=?, updated_at=? WHERE id=?`, displayName, namePinyin, nowSQL(), userID); err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *MySQLRepository) replaceRolesTx(ctx context.Context, tx *sql.Tx, groupID uint64, roleAssignments map[uint64][]string, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_group_roles WHERE group_id=? AND role IN (?,?)`, groupID, roleGroupAdmin, roleGroupLeader); err != nil {
		return err
	}
	for userID, roles := range roleAssignments {
		for _, role := range roles {
			role = strings.TrimSpace(role)
			if role != roleGroupAdmin && role != roleGroupLeader {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO user_group_roles (group_id,user_id,role,created_at) VALUES (?,?,?,?)`, groupID, userID, role, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *MySQLRepository) usernameMapFromRolesTx(ctx context.Context, tx *sql.Tx, roleAssignments map[uint64][]string) (map[string]uint64, error) {
	userIDs := map[string]uint64{}
	for userID := range roleAssignments {
		var username string
		if err := tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id=?`, userID).Scan(&username); err == nil {
			userIDs[username] = userID
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	return userIDs, nil
}

func (r *MySQLRepository) replaceCheckinsTx(ctx context.Context, tx *sql.Tx, groupID, actorID uint64, userIDs map[string]uint64, checkins []Checkin, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `UPDATE checkin_records SET deleted_at=?, active_key=id, updated_at=? WHERE group_id=? AND deleted_at IS NULL`, nowSQL(), nowSQL(), groupID); err != nil {
		return err
	}
	for _, checkin := range checkins {
		userID := userIDs[normalizeUsername(checkin.Username)]
		if userID == 0 {
			continue
		}
		checkinTime := parseTimeOrNow(checkin.CheckinTime, now)
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO checkin_records (group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			groupID, userID, nil, nil, checkin.LogicalDate, checkinTime, checkin.TaskType, "done", checkin.IsRetro, checkin.Detail, checkin.Note, truncate(checkin.Part, 64), "import", actorID, checkinTime, checkinTime); err != nil {
			return err
		}
	}
	return nil
}

func (r *MySQLRepository) replaceFeedbacksTx(ctx context.Context, tx *sql.Tx, groupID uint64, userIDs map[string]uint64, feedbacks []Feedback, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM feedbacks WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, feedback := range feedbacks {
		var userID any
		if id := userIDs[normalizeUsername(feedback.Username)]; id > 0 {
			userID = id
		}
		createdAt := parseTimeOrNow(feedback.CreatedAt, now)
		if _, err := tx.ExecContext(ctx, `INSERT INTO feedbacks (group_id,user_id,name,contact,message,page,user_agent,created_at)
			VALUES (?,?,?,?,?,?,?,?)`, groupID, userID, feedback.Name, feedback.Contact, feedback.Message, feedback.Page, feedback.UserAgent, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func groupDefaultPasswordHashTx(ctx context.Context, tx *sql.Tx, groupID uint64) (string, error) {
	var hash string
	err := tx.QueryRowContext(ctx, `SELECT default_password_hash FROM study_groups WHERE id=?`, groupID).Scan(&hash)
	return hash, err
}

func addMemberTx(ctx context.Context, tx *sql.Tx, groupID, userID uint64, memberName string, actorID uint64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO group_members (group_id,user_id,member_name,joined_at,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE status=1, updated_at=VALUES(updated_at)`, groupID, userID, memberName, nowSQL(), actorID, nowSQL(), nowSQL())
	return err
}

func normalizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseTimeOrNow(value string, fallback time.Time) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04:05.000"} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return fallback
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}
