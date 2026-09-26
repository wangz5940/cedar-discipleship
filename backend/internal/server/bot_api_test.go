package server

import "testing"

func TestBotTaskTypeSupportsExistingCheckinTypes(t *testing.T) {
	for input, want := range map[string]string{
		"daily_devotion":  "daily_devotion",
		"每日读经":          "daily_scripture",
		"daily_scripture": "daily_scripture",
		"周任务":            "weekly_checkin",
		"每周学习":           "weekly_checkin",
		"weekly_checkin":  "weekly_checkin",
		"weekly_book":     "weekly_book",
	} {
		if got := botTaskType(input); got != want {
			t.Errorf("botTaskType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBotDevotionAssetOnlyAllowsConfiguredGroupResource(t *testing.T) {
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion": map[string]any{"path": "/api/assets/263/download", "plans": []any{
			map[string]any{"path": "/api/assets/473/download"},
		}},
	}}}
	if !botDevotionAssetAllowed(settings, 263) || !botDevotionAssetAllowed(settings, 473) {
		t.Fatal("configured devotion resources were rejected")
	}
	if botDevotionAssetAllowed(settings, 999) {
		t.Fatal("unrelated group asset was allowed")
	}
}
