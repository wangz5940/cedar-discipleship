package audit

type CreateLogInput struct {
	GroupID    uint64
	ActorID    uint64
	Action     string
	TargetType string
	TargetID   uint64
	Before     any
	After      any
	IP         string
	UserAgent  string
}

type LogVO struct {
	ID          uint64 `json:"id"`
	ActorUserID uint64 `json:"actor_user_id"`
	Action      string `json:"action"`
	TargetType  string `json:"target_type"`
	TargetID    uint64 `json:"target_id"`
	CreatedAt   string `json:"created_at"`
}
