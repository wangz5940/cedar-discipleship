package learning

import (
	"context"
	"testing"
	"time"
)

type calendarRepository struct {
	statisticsTasksRepository
	records []TodayRecord
	queries int
}

func (r *calendarRepository) ListCompletionRecords(_ context.Context, groupID, userID uint64, from, to string) ([]TodayRecord, error) {
	r.queries++
	if groupID != 1 || userID != 2 {
		panic("calendar lost account scope")
	}
	var items []TodayRecord
	for _, record := range r.records {
		if from <= record.LogicalDate && record.LogicalDate <= to {
			items = append(items, record)
		}
	}
	return items, nil
}

func TestCalendarMatchesActualTasksRatherThanRecordCount(t *testing.T) {
	taskID, weekID := uint64(10), uint64(1)
	repo := &calendarRepository{
		statisticsTasksRepository: statisticsTasksRepository{
			weeks: []Week{{ID: weekID, StartDate: "2026-09-29", EndDate: "2026-10-05", VideoEnabled: true}},
			tasks: []Task{
				{ID: taskID, WeekID: weekID, TaskType: "weekly_video", Title: "第一课", Enabled: true, Assets: []TaskAsset{{ID: 7}}},
				{ID: 11, WeekID: weekID, TaskType: "weekly_video", Title: "第二课", Enabled: true, Assets: []TaskAsset{{ID: 8}}},
			},
		},
		records: []TodayRecord{
			{ID: 1, TaskType: "weekly_video", TaskID: &taskID, WeekID: &weekID, LogicalDate: "2026-09-30"},
			{ID: 2, TaskType: "weekly_video", TaskID: &taskID, WeekID: &weekID, LogicalDate: "2026-10-01"},
			{ID: 3, TaskType: "weekly_outline", WeekID: &weekID, LogicalDate: "2026-10-01"},
		},
	}
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false},
	}}}
	got, err := NewService(repo).CalendarProgress(t.Context(), 1, 2, "2026-10", settings, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2026-10-01", "2026-10-05"} {
		if got[date].Completed != 1 || got[date].Total != 2 {
			t.Fatalf("%s progress=%+v", date, got[date])
		}
	}
	if got["2026-10-06"].Completed != 0 || repo.queries != 2 {
		t.Fatalf("outside week=%+v queries=%d", got["2026-10-06"], repo.queries)
	}
	// A later completion must not backfill a day before the member performed it.
	repo.records = []TodayRecord{{ID: 4, TaskType: "weekly_video", TaskID: &taskID, WeekID: &weekID, LogicalDate: "2026-10-03"}}
	got, err = NewService(repo).CalendarProgress(t.Context(), 1, 2, "2026-10", settings, time.UTC)
	if err != nil || got["2026-10-01"].Completed != 0 || got["2026-10-03"].Completed != 1 {
		t.Fatalf("calendar completion date mismatch: %v err=%v", got, err)
	}
}
