package learning

import "testing"

func TestPreserveDailyScheduleHistoryArchivesForwardChanges(t *testing.T) {
	t.Parallel()

	existing := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion": map[string]any{
					"numbered_start_date": "2026-05-27",
					"numbered_start":      float64(43),
					"path":                "/api/assets/1/download",
				},
				"scripture": map[string]any{
					"start_date":    "2026-05-27",
					"book":          "路加福音",
					"book_id":       "42",
					"start_chapter": float64(1),
				},
			},
		},
	}
	next := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion": map[string]any{
					"numbered_start_date": "2026-09-07",
					"numbered_start":      float64(148),
					"path":                "/api/assets/1/download",
				},
				"scripture": map[string]any{
					"start_date":    "2026-05-27",
					"book":          "路加福音",
					"book_id":       "42",
					"start_chapter": float64(1),
				},
			},
		},
	}

	if err := preserveDailyScheduleHistory(existing, next); err != nil {
		t.Fatalf("preserveDailyScheduleHistory() error = %v", err)
	}
	devotion, _ := nestedMap(next, "task_sections", "daily", "devotion")
	history, ok := devotion["schedule_history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("devotion schedule_history = %#v, want one version", devotion["schedule_history"])
	}
	previous, ok := history[0].(map[string]any)
	if !ok {
		t.Fatalf("history item type = %T", history[0])
	}
	if previous["numbered_start_date"] != "2026-05-27" || previous["numbered_start"] != float64(43) {
		t.Fatalf("previous devotion = %#v", previous)
	}
	scripture, _ := nestedMap(next, "task_sections", "daily", "scripture")
	if _, exists := scripture["schedule_history"]; exists {
		t.Fatalf("unchanged scripture unexpectedly gained history: %#v", scripture["schedule_history"])
	}
}

func TestPreserveDailyScheduleHistoryDoesNotDuplicateSameVersion(t *testing.T) {
	t.Parallel()

	previous := map[string]any{
		"numbered_start_date": "2026-05-27",
		"numbered_start":      float64(43),
	}
	existing := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion": map[string]any{
					"numbered_start_date": "2026-09-07",
					"numbered_start":      float64(148),
					"schedule_history":    []any{previous},
				},
			},
		},
	}
	next := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion": map[string]any{
					"numbered_start_date": "2026-09-07",
					"numbered_start":      float64(148),
				},
			},
		},
	}

	if err := preserveDailyScheduleHistory(existing, next); err != nil {
		t.Fatalf("preserveDailyScheduleHistory() error = %v", err)
	}
	devotion, _ := nestedMap(next, "task_sections", "daily", "devotion")
	history, ok := devotion["schedule_history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("devotion schedule_history = %#v, want one preserved version", devotion["schedule_history"])
	}
}
