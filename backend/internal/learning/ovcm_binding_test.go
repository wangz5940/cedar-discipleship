package learning

import "testing"

func TestOVCMWeekBindingSurvivesTaskRoundTrip(t *testing.T) {
	t.Parallel()
	const url = "https://ovcm.net/tx2026/#/course/ds10tg/01?t=1044"
	drafts := BuildTaskDrafts(WeekInput{
		VideoEnabled: true,
		Videos:       []TaskBinding{{Title: "第一讲", URL: url}},
	}, "")
	if len(drafts) != 1 {
		t.Fatalf("got %d tasks, want one linked lesson", len(drafts))
	}
	draft := drafts[0]
	_, videos, _ := SplitWeekTaskBindings(TaskMaps([]Task{{
		TaskType: draft.TaskType, Title: draft.Title, Content: draft.Content, Enabled: true,
	}}))
	if len(videos) != 1 || videos[0].URL != url || videos[0].Title != "第一讲" || videos[0].AssetID != 0 {
		t.Fatalf("reloaded binding = %+v, want external URL and title preserved", videos)
	}
}
