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

func (r *MySQLRepository) MonthlyTaskCounts(ctx context.Context, groupID uint64, from, to string) ([]TaskCount, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id,task_type,COUNT(*)
		FROM checkin_records
		WHERE group_id=? AND logical_date BETWEEN ? AND ? AND deleted_at IS NULL
		  AND task_type IN ('daily_devotion','weekly_book','weekly_video','weekly_verse')
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
