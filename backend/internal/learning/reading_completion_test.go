package learning

import "testing"

func TestReadingCompletionSurvivesReducedRange(t *testing.T) {
	weekID, oldTaskID := uint64(185), uint64(1436)
	task := TodayTaskVO{Type: "weekly_book", TaskID: 1483, WeekID: weekID, Part: "灵命四季 13-15页", Assets: []map[string]any{{"id": uint64(474)}}}
	record := TodayRecord{ID: 10316, TaskType: "weekly_book", TaskID: &oldTaskID, WeekID: &weekID, AssetID: 474, Part: "灵命四季 13-27页", LogicalDate: "2026-10-01"}
	if got := matchingTodayRecord(task, []TodayRecord{record}, "2026-10-04"); got == nil {
		t.Fatal("completed larger reading disappeared after narrowing the same week's range")
	}
	for _, mutate := range []func(*TodayRecord){
		func(r *TodayRecord) { r.AssetID = 475 },
		func(r *TodayRecord) { otherWeek := uint64(184); r.WeekID = &otherWeek },
		func(r *TodayRecord) { r.Part = "灵命四季 13-14页" },
		func(r *TodayRecord) { r.Part = "灵命四季" },
	} {
		boundary := record
		mutate(&boundary)
		if got := matchingTodayRecord(task, []TodayRecord{boundary}, "2026-10-04"); got != nil {
			t.Fatalf("unproven reading counted as completed: %+v", boundary)
		}
	}
}

func TestReadingMetadataSurvivesTodayTaskConstruction(t *testing.T) {
	weekID, oldTaskID := uint64(185), uint64(1436)
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{
		"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false},
	}}}
	tasks := buildTodayTasks("2026-10-04", map[string]any{"id": weekID}, []map[string]any{{
		"id": uint64(1483), "task_type": "weekly_book", "title": "新名称",
		"content": `{"url":"/api/assets/474/download","page_start":13,"page_end":15}`,
		"assets":  []map[string]any{{"id": uint64(474), "title": "资料"}},
	}}, settings, []TodayRecord{{ID: 1, TaskType: "weekly_book", WeekID: &weekID, TaskID: &oldTaskID,
		AssetID: 474, Part: "旧名称", ReadingContent: `{"page_start":13,"page_end":27}`, LogicalDate: "2026-10-01"}})
	if len(tasks) != 1 || !tasks[0].Completed {
		t.Fatalf("page metadata lost between task construction and completion: %+v", tasks)
	}
}
