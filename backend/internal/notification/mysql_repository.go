package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CheckinSource struct {
	db       *sql.DB
	location *time.Location
}

func NewCheckinSource(db *sql.DB, location *time.Location) *CheckinSource {
	return &CheckinSource{db: db, location: location}
}

// Enabled is rechecked before every delivery, including frozen-message retries.
func (s *CheckinSource) Enabled(ctx context.Context, event Event) (bool, error) {
	var raw sql.NullString
	topic := event.Initial
	var err error
	if topic == "" {
		var taskType string
		err = s.db.QueryRowContext(ctx, `
			SELECT c.task_type,s.settings FROM checkin_records c
			LEFT JOIN group_settings s ON s.group_id=c.group_id
			WHERE c.group_id=? AND c.id=? AND c.logical_date=?
			  AND c.deleted_at IS NULL AND c.status='done' AND c.source='web'`,
			event.GroupID, event.RecordID, event.LogicalDate).Scan(&taskType, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		switch taskType {
		case "daily_devotion":
			topic = "daily"
		case "weekly_book", "weekly_video":
			topic = "weekly"
		}
	} else {
		err = s.db.QueryRowContext(ctx,
			`SELECT settings FROM group_settings WHERE group_id=?`, event.GroupID).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		}
	}
	if err != nil {
		return false, fmt.Errorf("load notification settings: %w", err)
	}
	if topic != "daily" && topic != "weekly" {
		return false, nil
	}
	var settings struct {
		Notifications struct {
			Daily  *bool `json:"daily_enabled"`
			Weekly *bool `json:"weekly_enabled"`
		} `json:"checkin_notifications"`
	}
	if raw.Valid && strings.TrimSpace(raw.String) != "" {
		if err := json.Unmarshal([]byte(raw.String), &settings); err != nil {
			return false, fmt.Errorf("decode notification settings: %w", err)
		}
	}
	enabled := settings.Notifications.Daily
	if topic == "weekly" {
		enabled = settings.Notifications.Weekly
	}
	return enabled == nil || *enabled, nil
}

func (s *CheckinSource) Snapshot(ctx context.Context, event Event) (Snapshot, error) {
	if event.Initial != "" {
		return s.initialSnapshot(ctx, event)
	}
	var taskType string
	var date, checkedAt time.Time
	var weekID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT task_type,logical_date,checkin_time,week_id
		FROM checkin_records
		WHERE group_id=? AND id=? AND logical_date=?
		  AND deleted_at IS NULL AND status='done' AND source='web'`,
		event.GroupID, event.RecordID, event.LogicalDate).
		Scan(&taskType, &date, &checkedAt, &weekID)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load notification checkin: %w", err)
	}
	today := checkedAt.In(s.location).Format("2006-01-02")
	logicalDate := date.Format("2006-01-02")
	start, end := today, today
	daily := taskType == "daily_devotion"
	if !daily {
		if taskType != "weekly_book" && taskType != "weekly_video" {
			return Snapshot{}, nil
		}
		var currentWeekID int64
		var weekStart, weekEnd time.Time
		err = s.db.QueryRowContext(ctx, `
			SELECT id,start_date,end_date FROM study_weeks
			WHERE group_id=? AND start_date<=? AND end_date>=?
			ORDER BY start_date DESC LIMIT 1`, event.GroupID, today, today).
			Scan(&currentWeekID, &weekStart, &weekEnd)
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, nil
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("load notification week: %w", err)
		}
		if !weekID.Valid || weekID.Int64 != currentWeekID {
			return Snapshot{}, nil
		}
		start, end = weekStart.Format("2006-01-02"), weekEnd.Format("2006-01-02")
	}
	if !eligible(taskType, logicalDate, today, start, end) {
		return Snapshot{}, nil
	}
	return s.periodSnapshot(ctx, event, start, end, weekID.Int64, daily)
}

func (s *CheckinSource) initialSnapshot(ctx context.Context, event Event) (Snapshot, error) {
	if event.OccurredAt.IsZero() || (event.Initial != "daily" && event.Initial != "weekly") {
		return Snapshot{}, errors.New("invalid initial notification event")
	}
	at := event.OccurredAt.In(s.location)
	today := at.Format("2006-01-02")
	if event.Initial == "daily" {
		return s.periodSnapshot(ctx, event, today, today, 0, true)
	}
	var weekID int64
	var start, end time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT id,start_date,end_date FROM study_weeks
		WHERE group_id=? AND start_date<=? AND end_date>=?
		ORDER BY start_date DESC LIMIT 1`, event.GroupID, today, today).
		Scan(&weekID, &start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{
			Text:      FormatCheckins(nil, 0, false),
			ExpiresAt: time.Date(at.Year(), at.Month(), at.Day()+1, 0, 0, 0, 0, s.location),
			Topic:     "weekly",
			Version:   "weekly:none:" + today,
		}, nil
	}
	if err != nil {
		return Snapshot{}, fmt.Errorf("load initial notification week: %w", err)
	}
	return s.periodSnapshot(ctx, event, start.Format("2006-01-02"), end.Format("2006-01-02"), weekID, false)
}

func (s *CheckinSource) periodSnapshot(ctx context.Context, event Event, start, end string, weekID int64, daily bool) (Snapshot, error) {
	endDate, err := time.ParseInLocation("2006-01-02", end, s.location)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse notification period: %w", err)
	}
	where := "c.task_type='daily_devotion'"
	period := "c.logical_date BETWEEN ? AND ?"
	args := []any{event.GroupID, start, end}
	cutoff := "c.id<=?"
	var cutoffValue any = event.RecordID
	if event.Initial != "" {
		cutoff = "c.checkin_time<=?"
		cutoffValue = event.OccurredAt.UTC().Format("2006-01-02 15:04:05.000")
	}
	if !daily {
		where = "c.task_type IN ('weekly_book','weekly_video')"
		// Videos retain completion when the same asset is assigned again this week.
		period = `((c.logical_date BETWEEN ? AND ? AND c.week_id=?)
			OR (c.task_type='weekly_video' AND EXISTS (
				SELECT 1 FROM task_assets checked_ta
				JOIN task_assets current_ta
				  ON current_ta.group_id=checked_ta.group_id
				 AND current_ta.asset_id=checked_ta.asset_id
				JOIN study_tasks current_task
				  ON current_task.id=current_ta.task_id
				 AND current_task.group_id=current_ta.group_id
				 AND current_task.task_type='weekly_video' AND current_task.enabled=1
				WHERE checked_ta.group_id=c.group_id AND checked_ta.task_id=c.task_id
				  AND current_task.week_id=?
			)))`
		args = append(args, weekID, weekID)
	}
	args = append(args, cutoffValue)
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.user_id,
		       COALESCE(NULLIF(m.member_name,''),NULLIF(u.display_name,''),u.username),
		       c.task_type,COALESCE(t.title,''),COALESCE(t.content,'')
		FROM checkin_records c
		JOIN users u ON u.id=c.user_id
		JOIN group_members m ON m.group_id=c.group_id AND m.user_id=c.user_id AND m.status=1
		LEFT JOIN study_tasks t ON t.id=c.task_id AND t.group_id=c.group_id AND t.week_id=c.week_id
		WHERE c.group_id=? AND `+period+` AND `+cutoff+`
		  AND c.deleted_at IS NULL AND c.status='done' AND `+where+`
		ORDER BY c.checkin_time,c.id`, args...)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load notification summary: %w", err)
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var entry Entry
		var title, content string
		if err := rows.Scan(&entry.RecordID, &entry.UserID, &entry.Name, &entry.TaskType, &title, &content); err != nil {
			return Snapshot{}, fmt.Errorf("scan notification summary: %w", err)
		}
		entry.BookName = bookName(title, content)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("read notification summary: %w", err)
	}
	topic := "weekly"
	if daily {
		topic = "daily"
	}
	return Snapshot{
		Text:      FormatCheckins(entries, event.RecordID, daily),
		ExpiresAt: endDate.AddDate(0, 0, 1),
		Topic:     topic,
		Version:   topic + ":" + end,
	}, nil
}

func bookName(title, content string) string {
	var metadata struct {
		BookName string `json:"book_name"`
	}
	if json.Unmarshal([]byte(content), &metadata) == nil && strings.TrimSpace(metadata.BookName) != "" {
		return metadata.BookName
	}
	return title
}
