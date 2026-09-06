package statistics

import "context"

type Repository interface {
	DailySummary(ctx context.Context, groupID uint64, from, to string) (map[string]int, error)
	Members(ctx context.Context, groupID uint64) ([]Member, error)
	MonthlyNonVideoTaskCounts(ctx context.Context, groupID uint64, from, to string) ([]TaskCount, error)
	MonthlyVideoCompletionCounts(ctx context.Context, groupID uint64, from, to string) ([]TaskCount, error)
	MemberCalendar(ctx context.Context, groupID, userID uint64, from, to string) ([]CalendarItem, error)
	LearningTotals(ctx context.Context, groupID, userID uint64) (*LearningTotals, error)
}
