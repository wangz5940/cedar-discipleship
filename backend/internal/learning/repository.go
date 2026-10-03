package learning

import (
	"context"
	"time"
)

type Repository interface {
	CurrentWeek(ctx context.Context, groupID uint64, date string) (*Week, error)
	ListWeeks(ctx context.Context, groupID uint64) ([]Week, error)
	ListTasks(ctx context.Context, groupID, weekID uint64) ([]Task, error)
	ListTasksForWeeks(ctx context.Context, groupID uint64, weekIDs []uint64) ([]Task, error)
	ListCompletionRecords(ctx context.Context, groupID, userID uint64, from, to string) ([]TodayRecord, error)
	LearningConfig(ctx context.Context, groupID uint64) (map[string]any, error)
	SaveLearningConfig(ctx context.Context, groupID uint64, settings map[string]any) error
	SaveActiveMemberRule(ctx context.Context, groupID uint64, rule map[string]any) error
	SaveResourceDownloadEnabled(ctx context.Context, groupID uint64, enabled bool) error
	ExistingTaskTitle(ctx context.Context, groupID, weekID uint64, taskType string) (string, error)
	SaveWeek(ctx context.Context, groupID, weekID uint64, input WeekInput, tasks []TaskDraft, force bool, now time.Time) (uint64, error)
	DeleteWeek(ctx context.Context, groupID, weekID uint64) error
}
