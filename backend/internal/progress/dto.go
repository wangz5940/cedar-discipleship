package progress

type UpdateProgressRequest struct {
	ContentID uint64 `json:"content_id"`
	TaskID    uint64 `json:"task_id"`
	Kind      Kind   `json:"kind"`
	Value     int64  `json:"value"`
	Completed bool   `json:"completed"`
}

type ProgressVO struct {
	ContentID uint64 `json:"content_id"`
	TaskID    uint64 `json:"task_id"`
	Kind      Kind   `json:"kind"`
	Value     int64  `json:"value"`
	Completed bool   `json:"completed"`
}
