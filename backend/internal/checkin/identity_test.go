package checkin

import "testing"

func TestActiveRecordKey(t *testing.T) {
	first := ActiveRecordKey("weekly_book", 1)
	second := ActiveRecordKey("weekly_book", 2)
	if first == 0 || second == 0 || first == second {
		t.Fatalf("weekly book keys are not distinct: first=%d second=%d", first, second)
	}
	for _, test := range []struct {
		taskType string
		taskID   uint64
	}{
		{taskType: "weekly_book"},
		{taskType: "weekly_video", taskID: 1},
		{taskType: "daily_devotion", taskID: 1},
		{taskType: "weekly_book", taskID: weeklyBookActiveKeyMask},
	} {
		if got := ActiveRecordKey(test.taskType, test.taskID); got != 0 {
			t.Errorf("ActiveRecordKey(%q, %d) = %d, want 0", test.taskType, test.taskID, got)
		}
	}
}
