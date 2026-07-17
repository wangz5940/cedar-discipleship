package backup

import (
	"testing"

	"agp/backend/internal/learning"
)

func TestMapBackupTaskIDsHandlesDuplicateTitles(t *testing.T) {
	week := learning.WeekInput{
		Readings: []learning.TaskBinding{
			{TaskID: 11, Title: "同名读物"},
			{TaskID: 12, Title: "同名读物"},
		},
	}
	drafts := []learning.TaskDraft{
		{TaskType: "weekly_book", Title: "同名读物"},
		{TaskType: "weekly_book", Title: "同名读物"},
	}
	taskIDs := map[uint64]uint64{}

	mapBackupTaskIDs(week, drafts, []uint64{101, 102}, taskIDs)

	if taskIDs[11] != 101 || taskIDs[12] != 102 {
		t.Fatalf("task ID map = %#v, want 11->101 and 12->102", taskIDs)
	}
}
