package user

type User struct {
	ID                 uint64 `json:"id"`
	Username           string `json:"username"`
	DisplayName        string `json:"display_name"`
	NamePinyin         string `json:"name_pinyin"`
	PasswordHash       string `json:"-"`
	IsSuperAdmin       bool   `json:"is_super_admin"`
	DefaultGroupID     uint64 `json:"default_group_id"`
	MustChangePassword bool   `json:"must_change_password"`
	Status             int    `json:"status"`
}

type Group struct {
	ID          uint64 `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Role string

const RoleMember = "member"
const RoleGroupAdmin = "group_admin"
const RoleGroupLeader = "group_leader"

type Member struct {
	MemberID     uint64
	UserID       uint64
	Username     string
	DisplayName  string
	MemberName   string
	IsSuperAdmin bool
	Roles        []string
}

type AdminMember struct {
	UserID       uint64
	IsSuperAdmin bool
	LeaderCount  int
}

type UserListItem struct {
	ID           uint64
	Username     string
	DisplayName  string
	IsSuperAdmin bool
	Status       int
}
