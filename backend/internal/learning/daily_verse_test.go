package learning

import "testing"

func TestIndependentVerseFrequencyAndRanges(t *testing.T) {
	plan := map[string]any{"date": "2026-09-29", "end_date": "2026-10-05", "completion_mode": "weekly", "verse_ref": "约3:16", "recite_text": "神爱世人"}
	config := map[string]any{"enabled": true, "plans": []any{plan}}
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false}, "verse": config,
	}}}
	if err := ValidateDailyVerse(settings); err != nil {
		t.Fatal(err)
	}
	records := []TodayRecord{{ID: 7, TaskType: "daily_verse", LogicalDate: "2026-09-30", Detail: "约3:16"}}
	if tasks := buildTodayTasks("2026-10-04", nil, nil, settings, records); len(tasks) != 1 || !tasks[0].Completed || tasks[0].TaskID != 0 || tasks[0].WeekID != 0 {
		t.Fatalf("weekly independent tasks=%+v", tasks)
	}
	if tasks := buildTodayTasks("2026-10-06", nil, nil, settings, records); len(tasks) != 0 {
		t.Fatalf("outside range=%+v", tasks)
	}
	plan["completion_mode"] = "daily"
	if tasks := buildTodayTasks("2026-10-04", nil, nil, settings, records); len(tasks) != 1 || tasks[0].Completed {
		t.Fatalf("daily repetition=%+v", tasks)
	}
	config["plans"] = []any{plan, map[string]any{"date": "2026-10-04", "verse_ref": "诗23:1", "recite_text": "耶和华"}}
	if ValidateDailyVerse(settings) != ErrInvalidDailyVerse {
		t.Fatal("overlapping plans accepted")
	}
	config["plans"] = []any{plan}
	plan["completion_mode"], plan["end_date"] = "weekly", "2026-10-06"
	if ValidateDailyVerse(settings) != ErrInvalidDailyVerse {
		t.Fatal("eight-day weekly period accepted")
	}
}

func TestDailyVerseTasks(t *testing.T) {
	for _, mode := range []string{"combined", "separate"} {
		t.Run(mode, func(t *testing.T) {
			verse := map[string]any{
				"enabled": true,
				"plans": []any{
					map[string]any{"date": "2026-10-04", "verse_ref": "约翰福音 3:16", "recite_text": "神爱世人"},
					map[string]any{"date": "2026-10-05", "verse_ref": "诗篇 23:1", "recite_text": "耶和华是我的牧者"},
				},
			}
			daily := map[string]any{
				"checkin_mode": mode,
				"devotion":     map[string]any{"enabled": false},
				"scripture":    map[string]any{"enabled": false},
				"verse":        verse,
			}
			settings := map[string]any{"task_sections": map[string]any{"daily": daily}}
			taskID, weekID := uint64(11), uint64(7)
			week := map[string]any{"id": weekID, "verse_enabled": true}
			raw := []map[string]any{{
				"id": taskID, "task_type": "weekly_verse",
				"title": "周经文", "content": "周原文", "enabled": true,
			}}
			records := []TodayRecord{
				{ID: 1, TaskType: "weekly_verse", TaskID: &taskID, WeekID: &weekID, LogicalDate: "2026-10-04"},
				{ID: 2, TaskType: "daily_verse", LogicalDate: "2026-10-04"},
			}
			for _, tc := range []struct {
				name      string
				date      string
				title     string
				text      string
				completed bool
			}{
				{name: "completed date", date: "2026-10-04", title: "约翰福音 3:16", text: "神爱世人", completed: true},
				{name: "next date", date: "2026-10-05", title: "诗篇 23:1", text: "耶和华是我的牧者"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					tasks := buildTodayTasks(tc.date, week, raw, settings, records)
					if len(tasks) != 2 {
						t.Fatalf("daily and weekly verses must coexist: %+v", tasks)
					}
					got := tasks[0]
					if got.Type != "daily_verse" || got.Kind != "verse" || got.Title != tc.title ||
						got.Content != tc.text || got.Completed != tc.completed || got.WeekID != 0 || got.TaskID != 0 {
						t.Fatalf("daily verse = %+v", got)
					}
					if tasks[1].Type != "weekly_verse" || !tasks[1].Completed {
						t.Fatalf("weekly completion must last the entire week: %+v", tasks[1])
					}
					if !DailyTaskTypeEnabledOnDate(settings, "daily_verse", tc.date) {
						t.Fatal("configured daily verse was rejected")
					}
				})
			}
			if tasks := buildTodayTasks("2026-10-04", nil, nil, settings, records); len(tasks) != 1 || !tasks[0].Completed {
				t.Fatalf("daily verse must work without a week: %+v", tasks)
			}
			if tasks := buildTodayTasks("2026-10-06", week, raw, settings, records); len(tasks) != 1 || tasks[0].Type != "weekly_verse" {
				t.Fatalf("unconfigured day must only show weekly verse: %+v", tasks)
			}
			if DailyTaskTypeEnabledOnDate(settings, "daily_verse", "2026-10-06") {
				t.Fatal("unconfigured date accepted")
			}
			verse["enabled"] = false
			if tasks := buildTodayTasks("2026-10-04", week, raw, settings, records); len(tasks) != 1 || tasks[0].Type != "weekly_verse" {
				t.Fatalf("disabled daily verse changed weekly task: %+v", tasks)
			}
			if DailyTaskTypeEnabledOnDate(settings, "daily_verse", "2026-10-04") {
				t.Fatal("disabled daily verse accepted")
			}
		})
	}
}

func TestDailyVerseRecordIsolation(t *testing.T) {
	weekID, taskID := uint64(7), uint64(11)
	task := TodayTaskVO{Type: "daily_verse"}
	for _, tc := range []struct {
		name   string
		record TodayRecord
		done   bool
	}{
		{name: "same date", record: TodayRecord{TaskType: "daily_verse", LogicalDate: "2026-10-04"}, done: true},
		{name: "other date", record: TodayRecord{TaskType: "daily_verse", LogicalDate: "2026-10-03"}},
		{name: "weekly verse", record: TodayRecord{TaskType: "weekly_verse", LogicalDate: "2026-10-04", WeekID: &weekID}},
		{name: "legacy repeated weekly verse", record: TodayRecord{
			TaskType: "daily_verse", LogicalDate: "2026-10-04", WeekID: &weekID, TaskID: &taskID,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := matchingTodayRecord(task, []TodayRecord{tc.record}, "2026-10-04")
			if (record != nil) != tc.done {
				t.Fatalf("matched=%+v want done=%v", record, tc.done)
			}
		})
	}
}
