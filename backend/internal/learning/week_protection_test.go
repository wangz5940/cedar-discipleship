package learning

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

type saveWeekTestRepository struct {
	force   bool
	saveErr error
}

func (r *saveWeekTestRepository) CurrentWeek(context.Context, uint64, string) (*Week, error) {
	return nil, sql.ErrNoRows
}

func (r *saveWeekTestRepository) ListWeeks(context.Context, uint64) ([]Week, error) {
	return nil, nil
}

func (r *saveWeekTestRepository) ListTasks(context.Context, uint64, uint64) ([]Task, error) {
	return nil, nil
}

func (r *saveWeekTestRepository) ListCompletionRecords(context.Context, uint64, uint64, string, string) ([]TodayRecord, error) {
	return nil, nil
}

func (r *saveWeekTestRepository) LearningConfig(context.Context, uint64) (map[string]any, error) {
	return map[string]any{}, nil
}

func (r *saveWeekTestRepository) SaveLearningConfig(context.Context, uint64, map[string]any) error {
	return nil
}

func (r *saveWeekTestRepository) ExistingTaskTitle(context.Context, uint64, uint64, string) (string, error) {
	return "", sql.ErrNoRows
}

func (r *saveWeekTestRepository) SaveWeek(
	_ context.Context,
	_, _ uint64,
	_ WeekInput,
	_ []TaskDraft,
	force bool,
	_ time.Time,
) (uint64, error) {
	r.force = force
	return 7, r.saveErr
}

func (r *saveWeekTestRepository) DeleteWeek(context.Context, uint64, uint64) error {
	return nil
}

func TestServiceSaveWeekPassesForcePolicy(t *testing.T) {
	tests := []struct {
		name  string
		force bool
	}{
		{name: "protected update", force: false},
		{name: "forced update", force: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &saveWeekTestRepository{}
			service := NewService(repo, nil, nil)

			_, err := service.SaveWeek(context.Background(), 1, 7, WeekInput{}, tt.force, time.Time{})
			if err != nil {
				t.Fatalf("SaveWeek() error = %v", err)
			}
			if repo.force != tt.force {
				t.Fatalf("repository force = %t, want %t", repo.force, tt.force)
			}
		})
	}
}

func TestServiceSaveWeekReturnsCheckinConflict(t *testing.T) {
	repo := &saveWeekTestRepository{saveErr: ErrWeekHasCheckins}
	service := NewService(repo, nil, nil)

	_, err := service.SaveWeek(context.Background(), 1, 7, WeekInput{}, false, time.Time{})
	if !errors.Is(err, ErrWeekHasCheckins) {
		t.Fatalf("SaveWeek() error = %v, want %v", err, ErrWeekHasCheckins)
	}
}
