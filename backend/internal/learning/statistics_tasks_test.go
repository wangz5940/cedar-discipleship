package learning

import (
	"context"
	"reflect"
	"testing"
)

type statisticsTasksRepository struct {
	Repository
	weeks    []Week
	tasks    []Task
	selected []uint64
}

func (r *statisticsTasksRepository) ListWeeks(context.Context, uint64) ([]Week, error) {
	return r.weeks, nil
}

func (r *statisticsTasksRepository) ListTasksForWeeks(_ context.Context, _ uint64, ids []uint64) ([]Task, error) {
	r.selected = ids
	var result []Task
	for _, task := range r.tasks {
		for _, id := range ids {
			if task.WeekID == id {
				result = append(result, task)
			}
		}
	}
	return result, nil
}

func TestStatisticsTaskTypesUsesLearningPlanRulesWithoutCompletions(t *testing.T) {
	for _, verseType := range []string{"weekly_verse", "daily_verse"} {
		t.Run(verseType, func(t *testing.T) {
			repo := &statisticsTasksRepository{
				weeks: []Week{
					{ID: 1, StartDate: "2026-10-01", EndDate: "2026-10-07", VerseEnabled: true, VideoEnabled: true},
					{ID: 2, StartDate: "2026-10-08", EndDate: "2026-10-14", BookEnabled: true},
				},
				tasks: []Task{
					{WeekID: 1, TaskType: verseType, Enabled: true},
					{WeekID: 1, TaskType: "weekly_video", Enabled: true},
					{WeekID: 1, TaskType: "weekly_book", Enabled: true},
					{WeekID: 1, TaskType: "weekly_outline", Enabled: false},
					{WeekID: 2, TaskType: "weekly_book", Enabled: true},
				},
			}
			got, err := NewService(repo).StatisticsTaskTypes(t.Context(), 4, "2026-10-01", "2026-10-03", map[string]any{})
			want := []string{"daily_devotion", verseType, "weekly_video"}
			if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(repo.selected, []uint64{1}) {
				t.Fatalf("types=%v selected=%v err=%v, want %v", got, repo.selected, err, want)
			}
		})
	}
}

func TestStatisticsTaskTypesIncludesSeparateDailyScripture(t *testing.T) {
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"checkin_mode": "separate",
		"devotion":     map[string]any{"enabled": false},
		"scripture":    map[string]any{"enabled": true, "type": "checkin"},
	}}}
	got, err := NewService(&statisticsTasksRepository{}).StatisticsTaskTypes(t.Context(), 4, "2026-10-01", "2026-10-03", settings)
	if err != nil || !reflect.DeepEqual(got, []string{"daily_scripture"}) {
		t.Fatalf("types=%v err=%v", got, err)
	}
}
