package statistics

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) DailySummary(ctx context.Context, groupID uint64, from, to string) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT task_type, COUNT(*) FROM checkin_records WHERE group_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL GROUP BY task_type`, groupID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := map[string]int{}
	for rows.Next() {
		var taskType string
		var count int
		if err := rows.Scan(&taskType, &count); err != nil {
			return nil, err
		}
		summary[taskType] = count
	}
	return summary, rows.Err()
}

func (r *MySQLRepository) Members(ctx context.Context, groupID uint64) ([]Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.id,u.id,u.username,u.display_name,m.member_name
		FROM group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND m.status=1
		ORDER BY m.member_name,u.username`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.MemberID, &member.UserID, &member.Username, &member.DisplayName, &member.MemberName); err != nil {
			return nil, err
		}
		member.MemberName = firstNonEmpty(member.MemberName, member.DisplayName)
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *MySQLRepository) MonthlyNonVideoTaskCounts(ctx context.Context, groupID uint64, from, to string) ([]TaskCount, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id,task_type,COUNT(*)
		FROM checkin_records
		WHERE group_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL
		  AND task_type IN ('daily_devotion','weekly_book','weekly_outline')
		GROUP BY user_id,task_type`, groupID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []TaskCount
	for rows.Next() {
		var count TaskCount
		if err := rows.Scan(&count.UserID, &count.TaskType, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func (r *MySQLRepository) MonthlyVideoCompletionCounts(ctx context.Context, groupID uint64, from, to string) ([]TaskCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT gm.user_id,'weekly_video',
		       COUNT(DISTINCT COALESCE(
		         CONCAT('asset:',current_asset.asset_id),
		         CONCAT('task:',current_task.id)
		       ))
		FROM group_members gm
		JOIN study_weeks current_week
		  ON current_week.group_id=gm.group_id
		 AND current_week.start_date<=?
		 AND current_week.end_date>=?
		JOIN study_tasks current_task
		  ON current_task.group_id=current_week.group_id
		 AND current_task.week_id=current_week.id
		 AND current_task.task_type='weekly_video'
		 AND current_task.enabled=1
		LEFT JOIN (
		  SELECT group_id,task_id,MIN(asset_id) AS asset_id
		  FROM task_assets
		  GROUP BY group_id,task_id
		) current_asset
		  ON current_asset.group_id=current_task.group_id
		 AND current_asset.task_id=current_task.id
		WHERE gm.group_id=? AND gm.status=1
		  AND EXISTS (
		    SELECT 1
		    FROM checkin_records c
		    WHERE c.group_id=gm.group_id
		      AND c.user_id=gm.user_id
		      AND c.task_type='weekly_video'
		      AND c.deleted_at IS NULL
		      AND (
		        c.task_id=current_task.id
		        OR (
		          current_asset.asset_id IS NULL
		          AND c.week_id=current_week.id
		        )
		        OR (
		          current_asset.asset_id IS NOT NULL
		          AND EXISTS (
		            SELECT 1
		            FROM task_assets checked_asset
		            WHERE checked_asset.group_id=c.group_id
		              AND checked_asset.task_id=c.task_id
		              AND checked_asset.asset_id=current_asset.asset_id
		          )
		        )
		      )
		  )
		GROUP BY gm.user_id`,
		to,
		from,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []TaskCount
	for rows.Next() {
		var count TaskCount
		if err := rows.Scan(&count.UserID, &count.TaskType, &count.Count); err != nil {
			return nil, err
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}

func (r *MySQLRepository) MemberCalendar(ctx context.Context, groupID, userID uint64, from, to string) ([]CalendarItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT logical_date, task_type, part FROM checkin_records WHERE group_id=? AND user_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL ORDER BY logical_date`, groupID, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CalendarItem
	for rows.Next() {
		var date time.Time
		var item CalendarItem
		if err := rows.Scan(&date, &item.TaskType, &item.Part); err != nil {
			return nil, err
		}
		item.Date = date.Format("2006-01-02")
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) LearningTotals(ctx context.Context, groupID, userID uint64) (*LearningTotals, error) {
	return &LearningTotals{}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
