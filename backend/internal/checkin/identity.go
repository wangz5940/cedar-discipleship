package checkin

const weeklyBookActiveKeyMask uint64 = 1 << 63

// ActiveRecordKey separates task-backed weekly books without changing their display part.
func ActiveRecordKey(taskType string, taskID uint64) uint64 {
	if taskType == "weekly_book" && taskID > 0 && taskID < weeklyBookActiveKeyMask {
		return weeklyBookActiveKeyMask | taskID
	}
	return 0
}
