package user

type UserVO struct {
	ID                 uint64   `json:"id"`
	Username           string   `json:"username"`
	DisplayName        string   `json:"display_name"`
	IsSuperAdmin       bool     `json:"is_super_admin"`
	DefaultGroupID     uint64   `json:"default_group_id"`
	MustChangePassword bool     `json:"must_change_password"`
	CurrentGroupID     uint64   `json:"current_group_id"`
	Groups             []Group  `json:"study_groups"`
	Roles              []string `json:"roles"`
}

type MemberVO struct {
	MemberID     uint64   `json:"member_id"`
	UserID       uint64   `json:"user_id"`
	Username     string   `json:"username"`
	DisplayName  string   `json:"display_name"`
	MemberName   string   `json:"member_name"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Roles        []string `json:"roles"`
}

type UserListItemVO struct {
	ID           uint64 `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	Status       int    `json:"status"`
}

type CreateUserRequest struct {
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	NamePinyin   string `json:"name_pinyin"`
	Password     string `json:"password"`
	GroupID      uint64 `json:"group_id"`
	Role         string `json:"role"`
	IsSuperAdmin bool   `json:"is_super_admin"`
}

type CreateMemberInput struct {
	CreateUser  bool
	UserID      uint64
	DisplayName string
	Username    string
	NamePinyin  string
}
