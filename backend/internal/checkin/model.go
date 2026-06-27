package checkin

type Record struct {
	ID          uint64
	GroupID     uint64
	UserID      uint64
	TaskID      uint64
	WeekID      uint64
	LogicalDate string
	CheckinTime string
	TaskType    string
	Part        string
	Detail      string
	Note        string
	IsRetro     bool
}
