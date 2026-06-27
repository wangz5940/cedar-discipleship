package statistics

type Summary struct {
	GroupID        uint64
	From           string
	To             string
	TotalTasks     int
	CompletedTasks int
	ActiveUsers    int
}

type LearningTotals struct {
	ReadPages      int
	WatchedMinutes int
	CompletedDays  int
	StreakDays     int
}

type Member struct {
	MemberID    uint64
	UserID      uint64
	Username    string
	DisplayName string
	MemberName  string
}

type TaskCount struct {
	UserID   uint64
	TaskType string
	Count    int
}

type CalendarItem struct {
	Date     string
	TaskType string
	Part     string
}
