package statistics

type SummaryVO struct {
	From    string         `json:"from"`
	To      string         `json:"to"`
	Summary map[string]int `json:"summary"`
}

type MonthlyRankingItemVO struct {
	MemberID    uint64         `json:"member_id"`
	UserID      uint64         `json:"user_id"`
	Username    string         `json:"username"`
	DisplayName string         `json:"display_name"`
	MemberName  string         `json:"member_name"`
	Counts      map[string]int `json:"counts"`
	Total       int            `json:"total"`
}

type MonthlyRankingVO struct {
	Month string                 `json:"month"`
	From  string                 `json:"from"`
	To    string                 `json:"to"`
	Items []MonthlyRankingItemVO `json:"items"`
}

type CalendarItemVO struct {
	Date     string `json:"date"`
	TaskType string `json:"task_type"`
	Part     string `json:"part"`
}

type MemberCalendarVO struct {
	Items []CalendarItemVO `json:"items"`
}
