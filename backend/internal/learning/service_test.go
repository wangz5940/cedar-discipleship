package learning

import "testing"

func TestMatchingTodayRecordWeeklyVideoMatchesSameTaskAcrossDates(t *testing.T) {
	taskID := uint64(11)
	weekID := uint64(7)
	record := matchingTodayRecord(TodayTaskVO{
		Type:   "weekly_video",
		TaskID: taskID,
		WeekID: weekID,
	}, []TodayRecord{
		{
			ID:          99,
			TaskType:    "weekly_video",
			TaskID:      &taskID,
			WeekID:      &weekID,
			LogicalDate: "2026-06-24",
		},
	}, "2026-06-27")
	if record == nil {
		t.Fatal("matchingTodayRecord returned nil, want existing weekly video record")
	}
	if record.ID != 99 {
		t.Fatalf("record ID = %d, want 99", record.ID)
	}
}

func TestMatchingTodayRecordWeeklyVideoFallsBackToWeek(t *testing.T) {
	weekID := uint64(7)
	record := matchingTodayRecord(TodayTaskVO{
		Type:   "weekly_video",
		WeekID: weekID,
	}, []TodayRecord{
		{
			ID:          100,
			TaskType:    "weekly_video",
			WeekID:      &weekID,
			LogicalDate: "2026-06-28",
		},
	}, "2026-06-27")
	if record == nil {
		t.Fatal("matchingTodayRecord returned nil, want existing weekly video record in same week")
	}
	if record.ID != 100 {
		t.Fatalf("record ID = %d, want 100", record.ID)
	}
}

func TestMatchingTodayRecordDailyDevotionStillRequiresDate(t *testing.T) {
	record := matchingTodayRecord(TodayTaskVO{
		Type: "daily_devotion",
	}, []TodayRecord{
		{
			ID:          101,
			TaskType:    "daily_devotion",
			LogicalDate: "2026-06-26",
		},
	}, "2026-06-27")
	if record != nil {
		t.Fatalf("matchingTodayRecord returned record ID %d, want nil for daily task on another date", record.ID)
	}
}
