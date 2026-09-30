package audit

import "encoding/json"

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
	LogID      string
}

type LogVO struct {
	ID               uint64          `json:"id"`
	GroupID          uint64          `json:"group_id"`
	ActorUserID      uint64          `json:"actor_user_id"`
	ActorUsername    string          `json:"actor_username"`
	ActorDisplayName string          `json:"actor_display_name"`
	Action           string          `json:"action"`
	TargetType       string          `json:"target_type"`
	TargetID         uint64          `json:"target_id"`
	Before           json.RawMessage `json:"before,omitempty"`
	After            json.RawMessage `json:"after,omitempty"`
	LogID            string          `json:"log_id,omitempty"`
	CreatedAt        string          `json:"created_at"`
}
