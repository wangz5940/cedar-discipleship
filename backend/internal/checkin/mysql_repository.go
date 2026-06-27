package checkin

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) FindExistingWeeklyBook(ctx context.Context, groupID, userID, taskID, weekID uint64, part, detail string) (uint64, error) {
	if taskID > 0 {
		var id uint64
		err := r.db.QueryRowContext(ctx, `SELECT id FROM checkin_records
			WHERE group_id=? AND user_id=? AND task_id=? AND task_type='weekly_book' AND deleted_at IS NULL
			ORDER BY logical_date,id LIMIT 1`, groupID, userID, taskID).Scan(&id)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return id, err
		}
	}
	if weekID == 0 {
		return 0, sql.ErrNoRows
	}
	title := strings.TrimSpace(firstNonEmpty(part, detail))
	if title == "" {
		return 0, sql.ErrNoRows
	}
	var start, end time.Time
	if err := r.db.QueryRowContext(ctx, `SELECT start_date,end_date FROM study_weeks WHERE group_id=? AND id=?`, groupID, weekID).Scan(&start, &end); err != nil {
		return 0, err
	}
	var id uint64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM checkin_records
		WHERE group_id=? AND user_id=? AND task_type='weekly_book'
		  AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL
		  AND (part=? OR detail=?)
		ORDER BY logical_date,id LIMIT 1`,
		groupID, userID, start.Format("2006-01-02"), end.Format("2006-01-02"), title, title).Scan(&id)
	return id, err
}

func (r *MySQLRepository) FindExistingWeeklyTask(ctx context.Context, groupID, userID, taskID, weekID uint64, taskType string) (uint64, error) {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return 0, sql.ErrNoRows
	}
	if taskID > 0 {
		var id uint64
		err := r.db.QueryRowContext(ctx, `SELECT id FROM checkin_records
			WHERE group_id=? AND user_id=? AND task_id=? AND task_type=? AND deleted_at IS NULL
			ORDER BY logical_date,id LIMIT 1`, groupID, userID, taskID, taskType).Scan(&id)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return id, err
		}
	}
	if weekID == 0 {
		return 0, sql.ErrNoRows
	}
	var id uint64
	err := r.db.QueryRowContext(ctx, `SELECT id FROM checkin_records
		WHERE group_id=? AND user_id=? AND week_id=? AND task_type=? AND deleted_at IS NULL
		ORDER BY logical_date,id LIMIT 1`, groupID, userID, weekID, taskType).Scan(&id)
	return id, err
}

func (r *MySQLRepository) Create(ctx context.Context, record *Record, actorID uint64) (uint64, error) {
	now := nowSQL()
	res, err := r.db.ExecContext(ctx, `INSERT INTO checkin_records (group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,status,is_retro,detail,note,part,source,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		record.GroupID, record.UserID, nullableID(record.TaskID), nullableID(record.WeekID), record.LogicalDate, now, record.TaskType, "done", record.IsRetro, record.Detail, record.Note, truncate(record.Part, 64), "web", actorID, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func (r *MySQLRepository) DeleteOwn(ctx context.Context, groupID, userID, recordID uint64) error {
	now := nowSQL()
	_, err := r.db.ExecContext(ctx, `UPDATE checkin_records SET deleted_at=?, active_key=id, updated_at=? WHERE id=? AND group_id=? AND user_id=? AND deleted_at IS NULL`, now, now, recordID, groupID, userID)
	return err
}

func (r *MySQLRepository) DeleteAny(ctx context.Context, groupID, recordID uint64) error {
	now := nowSQL()
	_, err := r.db.ExecContext(ctx, `UPDATE checkin_records SET deleted_at=?, active_key=id, updated_at=? WHERE id=? AND group_id=? AND deleted_at IS NULL`, now, now, recordID, groupID)
	return err
}

func (r *MySQLRepository) List(ctx context.Context, groupID uint64, from, to string, userID uint64, limit int) ([]Record, error) {
	args := []any{groupID, from, to}
	where := "group_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL"
	if userID > 0 {
		where += " AND user_id=?"
		args = append(args, userID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,task_id,week_id,logical_date,checkin_time,task_type,part,detail,note FROM checkin_records WHERE `+where+` ORDER BY logical_date DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Record
	for rows.Next() {
		var item Record
		var taskID, weekID sql.NullInt64
		var logicalDate, checkinTime time.Time
		var note sql.NullString
		if err := rows.Scan(&item.ID, &item.UserID, &taskID, &weekID, &logicalDate, &checkinTime, &item.TaskType, &item.Part, &item.Detail, &note); err != nil {
			return nil, err
		}
		item.GroupID = groupID
		if taskID.Valid && taskID.Int64 > 0 {
			item.TaskID = uint64(taskID.Int64)
		}
		if weekID.Valid && weekID.Int64 > 0 {
			item.WeekID = uint64(weekID.Int64)
		}
		item.LogicalDate = logicalDate.Format("2006-01-02")
		item.CheckinTime = checkinTime.Format(time.RFC3339)
		item.Note = note.String
		items = append(items, item)
	}
	return items, rows.Err()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}

func truncate(s string, n int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= n {
		return string(rs)
	}
	return string(rs[:n])
}
