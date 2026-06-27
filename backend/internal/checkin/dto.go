package checkin

type CreateRecordRequest struct {
	TaskType    string `json:"task_type"`
	LogicalDate string `json:"logical_date"`
	Part        string `json:"part"`
	Detail      string `json:"detail"`
	Note        string `json:"note"`
	WeekID      uint64 `json:"week_id"`
	TaskID      uint64 `json:"task_id"`
	IsRetro     bool   `json:"is_retro"`
}

type RecordVO struct {
	ID          uint64 `json:"id"`
	TaskID      uint64 `json:"task_id,omitempty"`
	WeekID      uint64 `json:"week_id,omitempty"`
	LogicalDate string `json:"logical_date"`
	TaskType    string `json:"task_type"`
	Part        string `json:"part"`
	Detail      string `json:"detail"`
}
