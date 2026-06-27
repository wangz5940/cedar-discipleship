package learning

import (
	"context"
	"database/sql"
	"encoding/json"
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

func (r *MySQLRepository) CurrentWeek(ctx context.Context, groupID uint64, date string) (*Week, error) {
	var week Week
	var start, end time.Time
	var recite sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id,start_date,end_date,title,verse_ref,recite_text,book_enabled,video_enabled,verse_enabled,outline_enabled
		FROM study_weeks WHERE group_id=? AND start_date <= ? AND end_date >= ? ORDER BY start_date DESC LIMIT 1`, groupID, date, date).
		Scan(&week.ID, &start, &end, &week.Title, &week.VerseRef, &recite, &week.BookEnabled, &week.VideoEnabled, &week.VerseEnabled, &week.OutlineEnabled)
	if err != nil {
		return nil, err
	}
	week.GroupID = groupID
	week.StartDate = start.Format("2006-01-02")
	week.EndDate = end.Format("2006-01-02")
	week.ReciteText = recite.String
	return &week, nil
}

func (r *MySQLRepository) ListWeeks(ctx context.Context, groupID uint64) ([]Week, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,start_date,end_date,title,verse_ref,recite_text,book_enabled,video_enabled,verse_enabled,outline_enabled
		FROM study_weeks WHERE group_id=? ORDER BY start_date,id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var weeks []Week
	for rows.Next() {
		var week Week
		var start, end time.Time
		var recite sql.NullString
		if err := rows.Scan(&week.ID, &start, &end, &week.Title, &week.VerseRef, &recite, &week.BookEnabled, &week.VideoEnabled, &week.VerseEnabled, &week.OutlineEnabled); err != nil {
			return nil, err
		}
		week.GroupID = groupID
		week.StartDate = start.Format("2006-01-02")
		week.EndDate = end.Format("2006-01-02")
		week.ReciteText = recite.String
		weeks = append(weeks, week)
	}
	return weeks, rows.Err()
}

func (r *MySQLRepository) ListTasks(ctx context.Context, groupID, weekID uint64) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,task_type,title,COALESCE(content,''),required,enabled
		FROM study_tasks WHERE group_id=? AND week_id=? AND enabled=1 ORDER BY sort_order,id`, groupID, weekID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		if err := rows.Scan(&task.ID, &task.TaskType, &task.Title, &task.Content, &task.Required, &task.Enabled); err != nil {
			return nil, err
		}
		task.GroupID = groupID
		task.WeekID = weekID
		assets, err := r.taskAssets(ctx, groupID, task.ID)
		if err != nil {
			return nil, err
		}
		task.Assets = assets
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *MySQLRepository) ListTodayRecords(ctx context.Context, groupID, userID uint64, from, to string) ([]TodayRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,user_id,task_id,week_id,logical_date,checkin_time,task_type,part,detail,note
		FROM checkin_records
		WHERE group_id=? AND user_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL
		ORDER BY logical_date DESC, id DESC`, groupID, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []TodayRecord
	for rows.Next() {
		var record TodayRecord
		var taskID, weekID sql.NullInt64
		var logicalDate, checkinTime time.Time
		var note sql.NullString
		if err := rows.Scan(&record.ID, &record.UserID, &taskID, &weekID, &logicalDate, &checkinTime, &record.TaskType, &record.Part, &record.Detail, &note); err != nil {
			return nil, err
		}
		record.TaskID = nullableUint64Ptr(taskID)
		record.WeekID = nullableUint64Ptr(weekID)
		record.LogicalDate = logicalDate.Format("2006-01-02")
		record.CheckinTime = checkinTime.Format(time.RFC3339)
		record.Note = note.String
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *MySQLRepository) LearningConfig(ctx context.Context, groupID uint64) (map[string]any, error) {
	var raw sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT settings FROM group_settings WHERE group_id=?`, groupID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw.String), &settings); err != nil {
		return map[string]any{}, nil
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

func (r *MySQLRepository) SaveLearningConfig(ctx context.Context, groupID uint64, settings map[string]any) error {
	return UpsertLearningConfigTx(ctx, r.db, groupID, settings)
}

func (r *MySQLRepository) ExistingTaskTitle(ctx context.Context, groupID, weekID uint64, taskType string) (string, error) {
	var title sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT title FROM study_tasks WHERE group_id=? AND week_id=? AND task_type=? ORDER BY sort_order,id LIMIT 1`, groupID, weekID, taskType).Scan(&title)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(title.String), nil
}

func (r *MySQLRepository) SaveWeek(ctx context.Context, groupID, weekID uint64, input WeekInput, tasks []TaskDraft, now time.Time) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	id := weekID
	if id == 0 {
		id, err = InsertWeekTx(ctx, tx, groupID, input, now)
		if err != nil {
			return 0, err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE study_weeks SET start_date=?,end_date=?,title=?,verse_ref=?,recite_text=?,book_enabled=?,video_enabled=?,verse_enabled=?,outline_enabled=?,updated_at=? WHERE id=? AND group_id=?`,
		input.StartDate, input.EndDate, input.Title, input.VerseRef, input.ReciteText, input.BookEnabled, input.VideoEnabled, input.VerseEnabled, input.OutlineEnabled, now, id, groupID); err != nil {
		return 0, err
	}
	if err := ReplaceWeekTasksTx(ctx, tx, groupID, id, tasks, now); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func (r *MySQLRepository) DeleteWeek(ctx context.Context, groupID, weekID uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := DeleteWeekTasksTx(ctx, tx, groupID, weekID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM study_weeks WHERE id=? AND group_id=?`, weekID, groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func InsertWeekTx(ctx context.Context, tx *sql.Tx, groupID uint64, input WeekInput, now time.Time) (uint64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO study_weeks (group_id,start_date,end_date,title,verse_ref,recite_text,book_enabled,video_enabled,verse_enabled,outline_enabled,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		groupID, input.StartDate, input.EndDate, input.Title, input.VerseRef, input.ReciteText, input.BookEnabled, input.VideoEnabled, input.VerseEnabled, input.OutlineEnabled, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func (r *MySQLRepository) taskAssets(ctx context.Context, groupID, taskID uint64) ([]TaskAsset, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.category,a.title,a.original_name,ta.usage_type
		FROM task_assets ta JOIN assets a ON a.id=ta.asset_id
		WHERE ta.group_id=? AND ta.task_id=?
		ORDER BY ta.sort_order,ta.id`, groupID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []TaskAsset
	for rows.Next() {
		var asset TaskAsset
		if err := rows.Scan(&asset.ID, &asset.Category, &asset.Title, &asset.OriginalName, &asset.UsageType); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func DeleteAllWeeksTx(ctx context.Context, tx *sql.Tx, groupID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE ta FROM task_assets ta
		JOIN study_tasks st ON st.id=ta.task_id
		WHERE st.group_id=?`, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM study_tasks WHERE group_id=?`, groupID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM study_weeks WHERE group_id=?`, groupID)
	return err
}

type configExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func UpsertLearningConfigTx(ctx context.Context, execer configExecer, groupID uint64, settings map[string]any) error {
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	now := nowSQL()
	_, err = execer.ExecContext(ctx, `INSERT INTO group_settings (group_id,settings,created_at,updated_at)
		VALUES (?,?,?,?)
		ON DUPLICATE KEY UPDATE settings=VALUES(settings), updated_at=VALUES(updated_at)`,
		groupID, string(payload), now, now)
	return err
}

func DeleteWeekTasksTx(ctx context.Context, tx *sql.Tx, groupID, weekID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE ta FROM task_assets ta
		JOIN study_tasks st ON st.id=ta.task_id
		WHERE st.group_id=? AND st.week_id=?`, groupID, weekID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM study_tasks WHERE group_id=? AND week_id=?`, groupID, weekID)
	return err
}

func ReplaceWeekTasksTx(ctx context.Context, tx *sql.Tx, groupID, weekID uint64, tasks []TaskDraft, now time.Time) error {
	if err := DeleteWeekTasksTx(ctx, tx, groupID, weekID); err != nil {
		return err
	}
	for _, task := range tasks {
		taskID, err := insertStudyTaskTx(ctx, tx, groupID, weekID, task, now)
		if err != nil {
			return err
		}
		if task.AssetID > 0 {
			if err := linkTaskAssetTx(ctx, tx, groupID, taskID, task.AssetID, task.UsageType, task.SortOrder, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertStudyTaskTx(ctx context.Context, tx *sql.Tx, groupID, weekID uint64, task TaskDraft, now time.Time) (uint64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO study_tasks (group_id,week_id,task_type,title,content,required,enabled,sort_order,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, groupID, weekID, task.TaskType, task.Title, task.Content, true, true, task.SortOrder, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func linkTaskAssetTx(ctx context.Context, tx *sql.Tx, groupID, taskID, assetID uint64, usageType string, sortOrder int, now time.Time) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM assets WHERE id=? AND group_id=?`, assetID, groupID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return errors.New("asset_not_found")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO task_assets (group_id,task_id,asset_id,usage_type,sort_order,created_at)
		VALUES (?,?,?,?,?,?)`, groupID, taskID, assetID, usageType, sortOrder, now)
	return err
}

func nullableUint64Ptr(v sql.NullInt64) *uint64 {
	if !v.Valid || v.Int64 <= 0 {
		return nil
	}
	u := uint64(v.Int64)
	return &u
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}
