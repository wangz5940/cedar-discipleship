package learning

import (
	"context"
	"time"
)

type Repository interface {
	CurrentWeek(ctx context.Context, groupID uint64, date string) (*Week, error)
	ListWeeks(ctx context.Context, groupID uint64) ([]Week, error)
	ListTasks(ctx context.Context, groupID, weekID uint64) ([]Task, error)
	ListTodayRecords(ctx context.Context, groupID, userID uint64, from, to string) ([]TodayRecord, error)
	LearningConfig(ctx context.Context, groupID uint64) (map[string]any, error)
	SaveLearningConfig(ctx context.Context, groupID uint64, settings map[string]any) error
	ExistingTaskTitle(ctx context.Context, groupID, weekID uint64, taskType string) (string, error)
	SaveWeek(ctx context.Context, groupID, weekID uint64, input WeekInput, tasks []TaskDraft, now time.Time) (uint64, error)
	DeleteWeek(ctx context.Context, groupID, weekID uint64) error
}
