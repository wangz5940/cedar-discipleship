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

func TestDailyTaskEnabled(t *testing.T) {
	settings := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion":  map[string]any{"enabled": false},
				"scripture": map[string]any{"enabled": false},
			},
		},
	}
	if DailyTaskEnabled(settings) {
		t.Fatal("DailyTaskEnabled = true when both daily sections are disabled")
	}
	if tasks := buildTodayTasks("2026-07-17", nil, nil, settings, nil); len(tasks) != 0 {
		t.Fatalf("buildTodayTasks returned %d tasks, want none", len(tasks))
	}
}

func TestWeekTitleUsesLearningContent(t *testing.T) {
	title := WeekTitle(WeekInput{
		BookEnabled:  true,
		VideoEnabled: true,
		VerseEnabled: true,
		Readings: []TaskBinding{
			{Title: "《生命读经》第一篇"},
			{Title: "《生命读经》第二篇"},
		},
		Videos:   []TaskBinding{{Title: "本周交通视频"}},
		VerseRef: "罗马书 8:1",
	})
	want := "《生命读经》第一篇；《生命读经》第二篇；本周交通视频；罗马书 8:1"
	if title != want {
		t.Fatalf("WeekTitle() = %q, want %q", title, want)
	}
}

func TestWeekTitleIgnoresStaleManualTitle(t *testing.T) {
	title := WeekTitle(WeekInput{
		Title:       "手动周标题",
		BookEnabled: true,
		Readings:    []TaskBinding{{Title: "读物标题"}},
	})
	if title != "读物标题" {
		t.Fatalf("WeekTitle() = %q, want content title", title)
	}
}
