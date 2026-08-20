package ministry

import "time"

type GroupSummary struct {
	ID               uint64     `json:"id"`
	Code             string     `json:"code"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	MemberCount      int        `json:"member_count"`
	Joined           bool       `json:"joined"`
	Role             MemberRole `json:"role"`
	IsLeader         bool       `json:"is_leader"`
	IdentityPublic   bool       `json:"identity_public"`
	RequestStatus    Status     `json:"request_status,omitempty"`
	MemberVisibility Visibility `json:"member_visibility"`
	ShareAutoApprove bool       `json:"share_auto_approve"`
	CanManage        bool       `json:"can_manage"`
	CanReviewShares  bool       `json:"can_review_shares"`
}

type MemberVO struct {
	UserID         uint64     `json:"user_id"`
	DisplayName    string     `json:"display_name"`
	Role           MemberRole `json:"role"`
	IsLeader       bool       `json:"is_leader"`
	IsSelf         bool       `json:"is_self"`
	IdentityPublic bool       `json:"identity_public"`
	IsVisible      bool       `json:"is_visible"`
}

type RequestVO struct {
	ID              uint64    `json:"id"`
	GroupID         uint64    `json:"group_id"`
	UserID          uint64    `json:"user_id"`
	UserDisplayName string    `json:"user_display_name"`
	Message         string    `json:"message"`
	Status          Status    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type NotificationVO struct {
	ID        uint64    `json:"id"`
	GroupID   uint64    `json:"group_id"`
	GroupName string    `json:"group_name"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

type ShareVO struct {
	ID          uint64     `json:"id"`
	GroupID     uint64     `json:"group_id"`
	AuthorID    uint64     `json:"author_id"`
	AuthorName  string     `json:"author_name"`
	Title       string     `json:"title"`
	Body        string     `json:"body_markdown"`
	Status      Status     `json:"status"`
	IsPinned    bool       `json:"is_pinned"`
	CanEdit     bool       `json:"can_edit"`
	CanReview   bool       `json:"can_review"`
	CanPin      bool       `json:"can_pin"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type AttachmentVO struct {
	ID           uint64 `json:"id"`
	Title        string `json:"title"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	URL          string `json:"url"`
}

type ProgressVO struct {
	ID          uint64         `json:"id"`
	GroupID     uint64         `json:"group_id"`
	AuthorID    uint64         `json:"author_id"`
	AuthorName  string         `json:"author_name"`
	OccurredAt  time.Time      `json:"occurred_at"`
	Content     string         `json:"content_markdown"`
	Attachments []AttachmentVO `json:"attachments"`
	CreatedAt   time.Time      `json:"created_at"`
}

type GroupDetail struct {
	Group    GroupSummary `json:"group"`
	Members  []MemberVO   `json:"members"`
	Shares   []ShareVO    `json:"shares"`
	Progress []ProgressVO `json:"progress"`
}

type AttendanceSettingsInput struct {
	Weekdays   []int    `json:"weekdays"`
	ExtraDates []string `json:"extra_dates"`
}

type AttendanceSettingsVO struct {
	Weekdays   []int    `json:"weekdays"`
	ExtraDates []string `json:"extra_dates"`
}

type AttendanceMemberVO struct {
	UserID       uint64          `json:"user_id"`
	Username     string          `json:"username"`
	DisplayName  string          `json:"display_name"`
	Present      map[string]bool `json:"present"`
	PresentCount int             `json:"present_count"`
}

type AttendanceSheetVO struct {
	Month     string               `json:"month"`
	Dates     []string             `json:"dates"`
	Members   []AttendanceMemberVO `json:"members"`
	Settings  AttendanceSettingsVO `json:"settings"`
	CanMark   bool                 `json:"can_mark"`
	CanManage bool                 `json:"can_manage"`
}

type GroupSettingsInput struct {
	MemberVisibility *Visibility `json:"member_visibility,omitempty"`
	ShareAutoApprove *bool       `json:"share_auto_approve,omitempty"`
	LeaderUserID     *uint64     `json:"leader_user_id,omitempty"`
}

type GroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ShareInput struct {
	Title string `json:"title"`
	Body  string `json:"body_markdown"`
}

type ProgressInput struct {
	OccurredAt time.Time `json:"occurred_at"`
	Content    string    `json:"content_markdown"`
	AssetIDs   []uint64  `json:"asset_ids"`
}
