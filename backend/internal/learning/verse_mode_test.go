package learning

import "testing"

func TestVerseCadenceCompletionBoundaries(t *testing.T) {
	for _, mode := range []string{"", "weekly", "daily"} {
		t.Run("mode="+mode, func(t *testing.T) {
			drafts := BuildTaskDrafts(WeekInput{VerseEnabled: true, VerseMode: mode, VerseRef: "罗马书 8:1", ReciteText: "如今"}, "")
			wantType := "weekly_verse"
			if mode == "daily" {
				wantType = "daily_verse"
			}
			if len(drafts) != 1 || drafts[0].TaskType != wantType {
				t.Fatalf("drafts=%+v", drafts)
			}
			taskID, weekID, otherWeek := uint64(12), uint64(4), uint64(5)
			raw := TaskMaps([]Task{{ID: taskID, WeekID: weekID, TaskType: drafts[0].TaskType, Title: drafts[0].Title, Content: drafts[0].Content, Enabled: true}})
			week := map[string]any{"id": weekID, "verse_enabled": true}
			settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false}}}}
			for _, date := range []string{"2026-09-22", "2026-09-23"} {
				tasks := buildTodayTasks(date, week, raw, settings, []TodayRecord{{ID: 1, TaskType: wantType, TaskID: &taskID, WeekID: &weekID, LogicalDate: "2026-09-22"}})
				wantDone := mode != "daily" || date == "2026-09-22"
				if len(tasks) != 1 || tasks[0].Completed != wantDone || tasks[0].Kind != "verse" {
					t.Fatalf("date=%s tasks=%+v", date, tasks)
				}
			}
			// Same-day completion from another week must never satisfy daily recitation.
			if mode == "daily" {
				tasks := buildTodayTasks("2026-09-22", week, raw, settings, []TodayRecord{{ID: 2, TaskType: wantType, WeekID: &otherWeek, LogicalDate: "2026-09-22"}})
				if tasks[0].Completed {
					t.Fatal("foreign week satisfied daily verse")
				}
			}
			week["verse_enabled"] = false
			if tasks := buildTodayTasks("2026-09-22", week, raw, settings, nil); len(tasks) != 0 {
				t.Fatalf("disabled verse shown: %+v", tasks)
			}
		})
	}
}
