package auth

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SwitchGroupRequest struct {
	GroupID uint64 `json:"group_id"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}
