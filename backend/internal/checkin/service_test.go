package checkin

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestServiceCreateWeeklyVideoAndVerseIdempotent(t *testing.T) {
	tests := []struct {
		name     string
		taskType string
	}{
		{name: "weekly video", taskType: "weekly_video"},
		{name: "weekly verse", taskType: "weekly_verse"},
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

type fakeRepository struct {
	existingWeeklyTaskID  uint64
	existingWeeklyTaskErr error
	weeklyTaskType        string
	createID              uint64
	createCalled          bool
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
	return r.createID, nil
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
