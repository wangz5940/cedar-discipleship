package learning

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSeparateDailyCompletion(t *testing.T) {
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"checkin_mode": "separate",
		"devotion":     map[string]any{"enabled": true},
		"scripture":    map[string]any{"enabled": true, "type": "checkin"},
	}}}
	records := []TodayRecord{{ID: 1, UserID: 2, TaskType: "daily_scripture", LogicalDate: "2026-09-20"}}
	tasks := buildTodayTasks("2026-09-20", nil, nil, settings, records)
	if len(tasks) != 2 || tasks[0].Type != "daily_devotion" || tasks[0].Completed ||
		tasks[1].Type != "daily_scripture" || !tasks[1].Completed {
		t.Fatalf("independent daily tasks = %+v", tasks)
	}
	items := buildGroupTaskCompletions("2026-09-20", nil, nil, settings, records)
	if len(items) != 1 || items[0].TaskType != "daily_scripture" {
		t.Fatalf("group completions = %+v", items)
	}
	if DailyTaskTypeEnabled(map[string]any{}, "daily_scripture") {
		t.Fatal("combined mode must not accept a separate scripture record")
	}
	settings["task_sections"].(map[string]any)["daily"].(map[string]any)["scripture"] = map[string]any{"enabled": false}
	if DailyTaskTypeEnabled(settings, "daily_scripture") {
		t.Fatal("disabled scripture must reject checkins")
	}
}

func TestRetiredAggregateWeekUsesIndividualBooks(t *testing.T) {
	weekID := uint64(7)
	input := WeekInput{
		Title: "本周两个主题", WeeklyCheckin: true, BookEnabled: true,
		Readings: []TaskBinding{{Title: "第一章", URL: "https://example.org/chapter.htm"}},
	}
	if title := WeekTitle(input); title != input.Title {
		t.Fatalf("week title = %q", title)
	}
	drafts := BuildTaskDrafts(input, "")
	if len(drafts) != 1 || drafts[0].TaskType != "weekly_book" || drafts[0].Optional {
		t.Fatalf("retired aggregate input must produce one required book: %+v", drafts)
	}
	raw := []map[string]any{
		{"id": uint64(11), "task_type": "weekly_checkin", "title": input.Title},
		{"id": uint64(12), "task_type": "weekly_book", "title": "第一章"},
	}
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion":  map[string]any{"enabled": false},
		"scripture": map[string]any{"enabled": false},
	}}}
	tasks := buildTodayTasks("2026-09-20", map[string]any{"id": weekID, "book_enabled": true},
		raw, settings, []TodayRecord{
			{ID: 1, TaskType: "weekly_checkin", WeekID: &weekID, LogicalDate: "2026-09-18"},
		})
	if len(tasks) != 1 || tasks[0].Type != "weekly_book" || tasks[0].Completed {
		t.Fatalf("historical aggregate must be hidden without completing the book: %+v", tasks)
	}
	active := activeWeekTaskMaps([]Task{
		{ID: 11, TaskType: "weekly_checkin", Title: input.Title, Enabled: true},
		{ID: 12, TaskType: "weekly_book", Title: "第一章", Enabled: true, Required: false},
	})
	if len(active) != 1 || active[0]["task_type"] != "weekly_book" {
		t.Fatalf("active task maps must expose only the book: %+v", active)
	}
	payload, err := json.Marshal(WeekVO{WeeklyCheckin: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "weekly_checkin") {
		t.Fatalf("retired aggregate mode leaked through the week API: %s", payload)
	}
}

func TestLegacyContentBindings(t *testing.T) {
	if got := InferTaskBindingType("weekly_book", "https://example.org/chapter.htm?v=1#part", ""); got != "iframe" {
		t.Fatalf("external HTML binding = %q", got)
	}
	if drafts := BuildTaskDrafts(WeekInput{VideoEnabled: true, VerseEnabled: true}, ""); len(drafts) != 0 {
		t.Fatalf("empty content created required tasks: %+v", drafts)
	}
}

func TestCustomDailyDevotionPlansRespectLogicalDate(t *testing.T) {
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"checkin_mode": "separate",
		"devotion": map[string]any{
			"enabled":   true,
			"plan_mode": "custom",
			"plans": []any{
				map[string]any{"date": "2026-09-22", "title": "当日灵修"},
			},
		},
		"scripture": map[string]any{"enabled": false},
	}}}

	tasks := buildTodayTasks("2026-09-22", nil, nil, settings, nil)
	if len(tasks) != 1 || tasks[0].Type != "daily_devotion" || tasks[0].Title != "当日灵修" {
		t.Fatalf("custom daily tasks = %+v, want matching devotion plan", tasks)
	}
	if DailyTaskTypeEnabledOnDate(settings, "daily_devotion", "2026-09-23") {
		t.Fatal("custom devotion check-in should be disabled without a matching plan")
	}
	if tasks := buildTodayTasks("2026-09-23", nil, nil, settings, nil); len(tasks) != 0 {
		t.Fatalf("missing custom date returned tasks: %+v", tasks)
	}
}

func TestAutomaticAndCombinedDailyModesRemainCompatible(t *testing.T) {
	automatic := buildTodayTasks("2026-09-22", nil, nil, map[string]any{}, nil)
	if len(automatic) != 1 || automatic[0].Type != "daily_devotion" {
		t.Fatalf("automatic daily tasks = %+v, want legacy devotion task", automatic)
	}

	combined := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"checkin_mode": "combined",
		"devotion": map[string]any{
			"enabled":   true,
			"plan_mode": "custom",
			"plans": []any{
				map[string]any{"date": "2026-09-22", "title": "合并模式当天标题"},
			},
		},
		"scripture": map[string]any{"enabled": true},
	}}}
	tasks := buildTodayTasks("2026-09-22", nil, nil, combined, nil)
	if len(tasks) != 1 || tasks[0].Title != "合并模式当天标题" {
		t.Fatalf("combined custom daily tasks = %+v, want custom title", tasks)
	}
	if !DailyTaskTypeEnabledOnDate(combined, "daily_devotion", "2026-09-23") {
		t.Fatal("combined daily check-in should remain enabled when scripture is enabled")
	}
}

func TestDailyContentHiddenBeforeConfiguredStartDate(t *testing.T) {
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"checkin_mode": "separate",
		"devotion":     map[string]any{"enabled": true, "numbered_start_date": "2026-09-24"},
		"scripture":    map[string]any{"enabled": true, "start_date": "2026-09-25"},
	}}}
	if got := buildTodayTasks("2026-09-23", nil, nil, settings, nil); len(got) != 0 {
		t.Fatalf("pre-start daily content = %+v, want none", got)
	}
	if got := buildTodayTasks("2026-09-24", nil, nil, settings, nil); len(got) != 1 || got[0].Type != "daily_devotion" {
		t.Fatalf("start-date devotion tasks = %+v", got)
	}
}
