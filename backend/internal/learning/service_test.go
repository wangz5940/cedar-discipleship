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

func TestBuildTodayTasksIncludesEnabledOutline(t *testing.T) {
	t.Parallel()

	tasks := buildTodayTasks(
		"2026-08-19",
		map[string]any{
			"id":              uint64(7),
			"outline_enabled": true,
		},
		[]map[string]any{{
			"id":        uint64(11),
			"task_type": "weekly_outline",
			"title":     "第三篇大纲",
			"enabled":   true,
			"assets": []map[string]any{{
				"id": uint64(23),
			}},
		}},
		map[string]any{},
		nil,
	)
	if len(tasks) != 2 {
		t.Fatalf("buildTodayTasks returned %d tasks, want devotion and outline", len(tasks))
	}
	outline := tasks[1]
	if outline.Type != "weekly_outline" || outline.Kind != "outline" {
		t.Fatalf("outline task = %+v", outline)
	}
	if outline.TaskID != 11 || outline.WeekID != 7 || len(outline.Assets) != 1 {
		t.Fatalf("outline target = %+v", outline)
	}
}

func TestBuildTodayTasksUsesAssetURLBeforeTaskContent(t *testing.T) {
	t.Parallel()

	tasks := buildTodayTasks(
		"2026-08-27",
		map[string]any{
			"id":            uint64(7),
			"video_enabled": true,
		},
		[]map[string]any{{
			"id":        uint64(31),
			"task_type": "weekly_video",
			"title":     "本周视频",
			"content":   "/Newtestament/video.mp4",
			"enabled":   true,
			"assets": []map[string]any{{
				"id":            uint64(145),
				"original_name": "video.mp4",
			}},
		}},
		map[string]any{},
		nil,
	)
	if len(tasks) != 2 {
		t.Fatalf("buildTodayTasks returned %d tasks, want devotion and video", len(tasks))
	}
	video := tasks[1]
	if video.Content != "/api/assets/145/download" {
		t.Fatalf("video content = %q, want asset download URL", video.Content)
	}
}

func TestMatchingTodayRecordWeeklyOutlineMatchesSameTaskAcrossDates(t *testing.T) {
	t.Parallel()

	taskID := uint64(11)
	weekID := uint64(7)
	record := matchingTodayRecord(TodayTaskVO{
		Type:   "weekly_outline",
		TaskID: taskID,
		WeekID: weekID,
	}, []TodayRecord{{
		ID:          102,
		TaskType:    "weekly_outline",
		TaskID:      &taskID,
		WeekID:      &weekID,
		LogicalDate: "2026-08-17",
	}}, "2026-08-19")
	if record == nil || record.ID != 102 {
		t.Fatalf("matchingTodayRecord returned %+v, want record 102", record)
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

func TestSplitWeekTaskBindingsUsesAssetURLBeforeTaskContent(t *testing.T) {
	t.Parallel()

	_, videos, _ := SplitWeekTaskBindings([]map[string]any{{
		"id":        uint64(31),
		"task_type": "weekly_video",
		"title":     "本周视频",
		"content":   "/Newtestament/video.mp4",
		"assets": []map[string]any{{
			"id":            uint64(145),
			"original_name": "video.mp4",
		}},
	}})
	if len(videos) != 1 {
		t.Fatalf("videos length = %d, want 1", len(videos))
	}
	if videos[0].URL != "/api/assets/145/download" {
		t.Fatalf("video URL = %q, want asset download URL", videos[0].URL)
	}
}
