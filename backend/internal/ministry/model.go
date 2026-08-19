package ministry

import "time"

type Visibility string

const (
	VisibilityAll  Visibility = "all"
	VisibilitySelf Visibility = "self"
)

type MemberRole string

const (
	MemberRoleMember MemberRole = "member"
	MemberRoleAdmin  MemberRole = "admin"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusApproved  Status = "approved"
	StatusRejected  Status = "rejected"
	StatusPublished Status = "published"
)

type Actor struct {
	UserID       uint64
	IsSuperAdmin bool
	IsStudyAdmin bool
}

type Access struct {
	GroupID          uint64
	LeaderUserID     uint64
	MemberVisibility Visibility
	ShareAutoApprove bool
	IsMember         bool
	IsLeader         bool
	IsAdmin          bool
	IdentityPublic   bool
}

type Group struct {
	ID               uint64
	StudyGroupID     uint64
	Code             string
	Name             string
	Description      string
	LeaderUserID     uint64
	MemberVisibility Visibility
	ShareAutoApprove bool
}

type Member struct {
	UserID         uint64
	Username       string
	DisplayName    string
	Role           MemberRole
	IdentityPublic bool
	IsLeader       bool
}

type AttendanceMember struct {
	UserID      uint64
	Username    string
	DisplayName string
}

type AttendanceRecord struct {
	UserID uint64
	Date   string
}

type Request struct {
	ID              uint64
	GroupID         uint64
	UserID          uint64
	UserDisplayName string
	Message         string
	Status          Status
	CreatedAt       time.Time
}

type Notification struct {
	ID        uint64
	GroupID   uint64
	GroupName string
	Type      string
	Title     string
	Body      string
	IsRead    bool
	CreatedAt time.Time
}

type Share struct {
	ID          uint64
	GroupID     uint64
	AuthorID    uint64
	AuthorName  string
	Title       string
	Body        string
	Status      Status
	ReviewedBy  uint64
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Attachment struct {
	ID           uint64
	Title        string
	OriginalName string
	MimeType     string
	URL          string
}

type Progress struct {
	ID          uint64
	GroupID     uint64
	AuthorID    uint64
	AuthorName  string
	OccurredAt  time.Time
	Content     string
	Attachments []Attachment
	CreatedAt   time.Time
}
