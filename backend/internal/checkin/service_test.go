package checkin

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestServiceCreateDailyRetryReturnsExistingRecord(t *testing.T) {
	for _, taskType := range []string{"daily_devotion", "daily_scripture"} {
		t.Run(taskType, func(t *testing.T) {
			repo := &fakeRepository{existingDailyID: 42}
			id, existing, err := NewService(repo).Create(context.Background(), &Record{
				GroupID: 1, UserID: 2, TaskType: taskType, LogicalDate: "2026-09-23",
			}, 2)
			if err != nil || !existing || id != 42 || repo.createCalled {
				t.Fatalf("id=%d existing=%t err=%v createCalled=%t", id, existing, err, repo.createCalled)
			}
		})
	}
}

func TestServiceCreateNewDailyRecord(t *testing.T) {
	repo := &fakeRepository{existingDailyErr: sql.ErrNoRows, createID: 44}
	id, existing, err := NewService(repo).Create(context.Background(), &Record{
		GroupID: 1, UserID: 2, TaskType: "daily_devotion", LogicalDate: "2026-09-23",
	}, 2)
	if err != nil || existing || id != 44 || !repo.createCalled {
		t.Fatalf("id=%d existing=%t err=%v createCalled=%t", id, existing, err, repo.createCalled)
	}
}

func TestServiceCreateDailyConcurrentRetry(t *testing.T) {
	repo := &fakeRepository{
		existingDailyErr: sql.ErrNoRows,
		createErr:        &mysql.MySQLError{Number: 1062},
	}
	repo.onCreate = func() { repo.existingDailyErr = nil; repo.existingDailyID = 43 }
	id, existing, err := NewService(repo).Create(context.Background(), &Record{
		GroupID: 1, UserID: 2, TaskType: "daily_devotion", LogicalDate: "2026-09-23",
	}, 2)
	if err != nil || !existing || id != 43 {
		t.Fatalf("id=%d existing=%t err=%v", id, existing, err)
	}
}

func TestServiceCreateDailyPreservesStorageErrors(t *testing.T) {
	want := errors.New("storage unavailable")
	repo := &fakeRepository{existingDailyErr: want}
	_, _, err := NewService(repo).Create(context.Background(), &Record{
		GroupID: 1, UserID: 2, TaskType: "daily_devotion", LogicalDate: "2026-09-23",
	}, 2)
	if !errors.Is(err, want) || repo.createCalled {
		t.Fatalf("err=%v createCalled=%t", err, repo.createCalled)
	}
}

func TestServiceCreateWeeklyTaskIdempotent(t *testing.T) {
	tests := []struct {
		name     string
		taskType string
	}{
		{name: "weekly video", taskType: "weekly_video"},
		{name: "weekly verse", taskType: "weekly_verse"},
		{name: "weekly outline", taskType: "weekly_outline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepository{
				existingWeeklyTaskID: 88,
			}
			service := NewService(repo)

			id, existing, err := service.Create(context.Background(), &Record{
				GroupID:  1,
				UserID:   2,
				TaskID:   3,
				WeekID:   4,
				TaskType: tt.taskType,
			}, 2)
			if err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			if id != 88 {
				t.Fatalf("id = %d, want 88", id)
			}
			if !existing {
				t.Fatal("existing = false, want true")
			}
			if repo.createCalled {
				t.Fatal("Create should not insert when an existing weekly task checkin is found")
			}
			if repo.weeklyTaskType != tt.taskType {
				t.Fatalf("weeklyTaskType = %q, want %q", repo.weeklyTaskType, tt.taskType)
			}
		})
	}
}

func TestServiceCreateWeeklyTaskCreatesWhenNotFound(t *testing.T) {
	repo := &fakeRepository{
		existingWeeklyTaskErr: sql.ErrNoRows,
		createID:              99,
	}
	service := NewService(repo)

	id, existing, err := service.Create(context.Background(), &Record{
		GroupID:  1,
		UserID:   2,
		TaskID:   3,
		WeekID:   4,
		TaskType: "weekly_video",
	}, 2)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if id != 99 {
		t.Fatalf("id = %d, want 99", id)
	}
	if existing {
		t.Fatal("existing = true, want false")
	}
	if !repo.createCalled {
		t.Fatal("Create should insert when no existing weekly task checkin is found")
	}
}

func TestServiceCreateWeeklyTaskReturnsLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	repo := &fakeRepository{
		existingWeeklyTaskErr: wantErr,
	}
	service := NewService(repo)

	_, _, err := service.Create(context.Background(), &Record{
		GroupID:  1,
		UserID:   2,
		TaskID:   3,
		WeekID:   4,
		TaskType: "weekly_verse",
	}, 2)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if repo.createCalled {
		t.Fatal("Create should not insert when weekly task lookup fails")
	}
}

func TestServiceCreateRejectsInvalidWeeklyTarget(t *testing.T) {
	repo := &fakeRepository{
		validateWeeklyTargetErr: sql.ErrNoRows,
	}
	service := NewService(repo)

	_, _, err := service.Create(context.Background(), &Record{
		GroupID:     1,
		UserID:      2,
		TaskID:      3,
		WeekID:      4,
		LogicalDate: "2026-07-17",
		TaskType:    "weekly_video",
	}, 2)
	if !errors.Is(err, ErrInvalidWeeklyTarget) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidWeeklyTarget)
	}
	if repo.createCalled {
		t.Fatal("Create should not insert an invalid weekly target")
	}
}

type fakeRepository struct {
	existingDailyID         uint64
	existingDailyErr        error
	createErr               error
	onCreate                func()
	validateWeeklyTargetErr error
	existingWeeklyTaskID    uint64
	existingWeeklyTaskErr   error
	weeklyTaskType          string
	createID                uint64
	createCalled            bool
}

func (r *fakeRepository) FindExistingDaily(ctx context.Context, groupID, userID uint64, taskType, logicalDate string) (uint64, error) {
	if r.existingDailyErr != nil {
		return 0, r.existingDailyErr
	}
	return r.existingDailyID, nil
}

func (r *fakeRepository) ValidateWeeklyTarget(ctx context.Context, groupID, taskID, weekID uint64, taskType, logicalDate string) error {
	return r.validateWeeklyTargetErr
}

func (r *fakeRepository) FindExistingWeeklyBook(ctx context.Context, groupID, userID, taskID, weekID uint64, part, detail string) (uint64, error) {
	return 0, sql.ErrNoRows
}

func (r *fakeRepository) FindExistingWeeklyTask(ctx context.Context, groupID, userID, taskID, weekID uint64, taskType string) (uint64, error) {
	r.weeklyTaskType = taskType
	if r.existingWeeklyTaskErr != nil {
		return 0, r.existingWeeklyTaskErr
	}
	return r.existingWeeklyTaskID, nil
}

func (r *fakeRepository) Create(ctx context.Context, record *Record, actorID uint64) (uint64, error) {
	r.createCalled = true
	if r.onCreate != nil {
		r.onCreate()
	}
	return r.createID, r.createErr
}

func (r *fakeRepository) DeleteOwn(ctx context.Context, groupID, userID, recordID uint64) error {
	return nil
}

func (r *fakeRepository) DeleteAny(ctx context.Context, groupID, recordID uint64) error {
	return nil
}

func (r *fakeRepository) List(ctx context.Context, groupID uint64, from, to string, userID uint64, limit int) ([]Record, error) {
	return nil, nil
}
