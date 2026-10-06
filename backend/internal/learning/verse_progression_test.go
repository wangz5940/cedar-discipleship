package learning

import "testing"

func progressionSettings(plan map[string]any) map[string]any {
	return map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false},
		"verse": map[string]any{"enabled": true, "plans": []any{plan}},
	}}}
}

func TestDailyVerseProgression(t *testing.T) {
	text := "【创1:1】第一节。\n创1:2 第二节。\n创1:3 第三节。\n创1:4 第四节。\n创2:1 第五节。"
	plan := map[string]any{"date": "2026-10-05", "end_date": "2026-10-11", "completion_mode": "daily", "verses_per_day": float64(2), "verse_ref": "创1:1-4，创2:1", "recite_text": text}
	settings := progressionSettings(plan)
	if err := ValidateDailyVerse(settings); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ date, ref, text string }{
		{"2026-10-05", "创1:1，创1:2", "【创1:1】第一节。\n创1:2 第二节。"},
		{"2026-10-06", "创1:3，创1:4", "创1:3 第三节。\n创1:4 第四节。"},
		{"2026-10-07", "创2:1", "创2:1 第五节。"},
	} {
		// No previous check-ins: the calendar, not completion order, advances the plan.
		tasks := buildTodayTasks(tc.date, nil, nil, settings, nil)
		if len(tasks) != 1 || tasks[0].Title != tc.ref || tasks[0].Content != tc.text || tasks[0].Completed {
			t.Fatalf("date=%s tasks=%+v", tc.date, tasks)
		}
	}
	for _, date := range []string{"2026-10-04", "2026-10-08", "2026-10-12"} {
		if DailyTaskTypeEnabledOnDate(settings, "daily_verse", date) || len(buildTodayTasks(date, nil, nil, settings, nil)) != 0 {
			t.Fatalf("unexpected task at %s", date)
		}
	}
	if plan["recite_text"] != text || plan["verse_ref"] != "创1:1-4，创2:1" {
		t.Fatal("stored source changed")
	}
	delete(plan, "verses_per_day")
	for _, mode := range []string{"daily", "weekly"} {
		plan["completion_mode"] = mode
		resolved, ok := DailyVersePlan(settings, "2026-10-11")
		if !ok || resolved["recite_text"] != text {
			t.Fatalf("legacy %s changed", mode)
		}
	}
}

func TestProgressionRejectsAmbiguousBoundaries(t *testing.T) {
	for _, text := range []string{"创1:1-2 原文", "创1:1 原文\n创1:1 重复", "创1:0 原文", "创1:1", "没有章节的原文"} {
		plan := map[string]any{"date": "2026-10-05", "end_date": "2026-10-11", "completion_mode": "daily", "verses_per_day": float64(2), "verse_ref": "创1:1", "recite_text": text}
		if ValidateDailyVerse(progressionSettings(plan)) != ErrInvalidDailyVerse {
			t.Errorf("accepted %q", text)
		}
	}
	for _, value := range []any{float64(0), float64(101), float64(1.5), "2", nil} {
		plan := map[string]any{"date": "2026-10-05", "completion_mode": "daily", "verses_per_day": value, "verse_ref": "创1:1", "recite_text": "创1:1 原文"}
		if ValidateDailyVerse(progressionSettings(plan)) != ErrInvalidDailyVerse {
			t.Errorf("accepted count=%v", value)
		}
	}
}

func TestProgressionStartIndependentOfWeek(t *testing.T) {
	plan := map[string]any{"date": "2026-10-05", "end_date": "2026-10-11", "completion_mode": "daily", "verses_per_day": float64(2), "progression_start_date": "2026-10-04", "verse_ref": "创1:1-5", "recite_text": "创1:1 一。\n创1:2 二。\n创1:3 三。\n创1:4 四。\n创1:5 五。"}
	settings := progressionSettings(plan)
	if err := ValidateDailyVerse(settings); err != nil {
		t.Fatal(err)
	}
	resolved, ok := DailyVersePlan(settings, "2026-10-05")
	if !ok || resolved["verse_ref"] != "创1:3，创1:4" {
		t.Fatalf("resolved=%v", resolved)
	}
	plan["progression_start_date"] = "2026-10-07"
	if _, ok := DailyVersePlan(settings, "2026-10-06"); ok {
		t.Fatal("task before start")
	}
	plan["progression_start_date"] = "2026-10-12"
	if ValidateDailyVerse(settings) != ErrInvalidDailyVerse {
		t.Fatal("accepted start after end")
	}
}
