package learning

import (
	"fmt"
	"testing"
)

func TestTodayContentTitleByDate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		date  string
		today string
		want  string
	}{
		{name: "today", date: "2026-09-06", today: "2026-09-06", want: "今日学习"},
		{name: "past", date: "2026-09-05", today: "2026-09-06", want: "学习回顾"},
		{name: "future", date: "2026-09-07", today: "2026-09-06", want: "学习预览"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := todayContentTitle(tt.date, tt.today); got != tt.want {
				t.Fatalf("todayContentTitle(%q, %q) = %q, want %q", tt.date, tt.today, got, tt.want)
			}
		})
	}
}

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

func TestBuildTodayTasksCarriesWeeklyVideoCompletionAcrossWeeksByAsset(t *testing.T) {
	previousTaskID := uint64(31)
	previousWeekID := uint64(7)
	tasks := buildTodayTasks(
		"2026-09-06",
		map[string]any{
			"id":            uint64(8),
			"video_enabled": true,
		},
		[]map[string]any{{
			"id":        uint64(32),
			"task_type": "weekly_video",
			"title":     "重复安排的视频",
			"enabled":   true,
			"assets": []map[string]any{{
				"id": uint64(27),
			}},
		}},
		map[string]any{
			"task_sections": map[string]any{
				"daily": map[string]any{
					"devotion":  map[string]any{"enabled": false},
					"scripture": map[string]any{"enabled": false},
				},
			},
		},
		[]TodayRecord{{
			ID:          101,
			TaskType:    "weekly_video",
			TaskID:      &previousTaskID,
			WeekID:      &previousWeekID,
			LogicalDate: "2026-09-01",
			AssetID:     27,
		}},
	)
	if len(tasks) != 1 {
		t.Fatalf("buildTodayTasks returned %d tasks, want one video", len(tasks))
	}
	if !tasks[0].Completed {
		t.Fatal("weekly video with the same asset should stay completed across weeks")
	}
	if tasks[0].Record != nil {
		t.Fatalf("carried completion record = %+v, want nil to keep prior-week history immutable", tasks[0].Record)
	}
}

func TestMatchingTodayRecordWeeklyVideoDoesNotMatchDifferentAsset(t *testing.T) {
	previousTaskID := uint64(31)
	for _, previousWeekID := range []uint64{7, 8} {
		t.Run(fmt.Sprintf("record week %d", previousWeekID), func(t *testing.T) {
			record := matchingTodayRecord(TodayTaskVO{
				Type:   "weekly_video",
				TaskID: 32,
				WeekID: 8,
				Assets: []map[string]any{{
					"id": uint64(28),
				}},
			}, []TodayRecord{{
				ID:          101,
				TaskType:    "weekly_video",
				TaskID:      &previousTaskID,
				WeekID:      &previousWeekID,
				LogicalDate: "2026-09-01",
				AssetID:     27,
			}}, "2026-09-06")
			if record != nil {
				t.Fatalf("matchingTodayRecord returned %+v for a different video asset", record)
			}
		})
	}
}

func TestBuildGroupTaskCompletionsUsesSharedTaskSemantics(t *testing.T) {
	previousVideoTaskID := uint64(31)
	previousWeekID := uint64(7)
	currentVideoTaskID := uint64(32)
	currentWeekID := uint64(8)
	records := []TodayRecord{
		{
			ID:          101,
			UserID:      2,
			TaskType:    "weekly_video",
			TaskID:      &previousVideoTaskID,
			WeekID:      &previousWeekID,
			LogicalDate: "2026-09-01",
			AssetID:     27,
		},
		{
			ID:          102,
			UserID:      3,
			TaskType:    "weekly_video",
			TaskID:      &previousVideoTaskID,
			WeekID:      &previousWeekID,
			LogicalDate: "2026-09-01",
			AssetID:     28,
		},
		{
			ID:          103,
			UserID:      3,
			TaskType:    "daily_devotion",
			LogicalDate: "2026-09-06",
		},
	}
	items := buildGroupTaskCompletions(
		"2026-09-06",
		map[string]any{
			"id":            currentWeekID,
			"video_enabled": true,
		},
		[]map[string]any{{
			"id":        currentVideoTaskID,
			"task_type": "weekly_video",
			"title":     "重复安排的视频",
			"enabled":   true,
			"assets": []map[string]any{{
				"id": uint64(27),
			}},
		}},
		map[string]any{},
		records,
	)
	if len(items) != 2 {
		t.Fatalf("buildGroupTaskCompletions returned %d items, want 2", len(items))
	}
	video := items[0]
	if video.UserID != 2 || video.TaskID != currentVideoTaskID || !video.Completed || !video.Inherited {
		t.Fatalf("video completion = %+v", video)
	}
	if video.Record != nil {
		t.Fatalf("inherited video record = %+v, want nil", video.Record)
	}
	daily := items[1]
	if daily.UserID != 3 || daily.TaskType != "daily_devotion" || daily.Inherited {
		t.Fatalf("daily completion = %+v", daily)
	}
	if daily.Record == nil || daily.Record.ID != 103 {
		t.Fatalf("daily record = %+v, want record 103", daily.Record)
	}
}

func TestBuildGroupTaskCompletionsKeepsCurrentVideoRecordEditable(t *testing.T) {
	taskID := uint64(32)
	weekID := uint64(8)
	items := buildGroupTaskCompletions(
		"2026-09-06",
		map[string]any{
			"id":            weekID,
			"video_enabled": true,
		},
		[]map[string]any{{
			"id":        taskID,
			"task_type": "weekly_video",
			"title":     "本周视频",
			"enabled":   true,
			"assets": []map[string]any{{
				"id": uint64(27),
			}},
		}},
		map[string]any{},
		[]TodayRecord{{
			ID:          104,
			UserID:      2,
			TaskType:    "weekly_video",
			TaskID:      &taskID,
			WeekID:      &weekID,
			LogicalDate: "2026-09-06",
			AssetID:     27,
		}},
	)
	if len(items) != 1 || items[0].Record == nil || items[0].Record.ID != 104 || items[0].Inherited {
		t.Fatalf("current video completion = %+v", items)
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
