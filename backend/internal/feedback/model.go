package feedback

import (
	"context"
	"io"
	"time"
)

type Status string
type Source string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusNeedsInfo  Status = "needs_info"
	StatusResolved   Status = "resolved"
	StatusClosed     Status = "closed"

	SourceManual    Source = "manual"
	SourceAutomatic Source = "automatic"

	MaxImages       = 4
	MaxImageBytes   = int64(5 << 20)
	MaxRequestBytes = int64(25 << 20)
)

type Feedback struct {
	ID              uint64
	GroupID         uint64
	UserID          uint64
	Username        string
	DisplayName     string
	MemberName      string
	GroupName       string
	LegacyName      string
	LegacyContact   string
	Message         string
	Source          Source
	Status          Status
	LogID           string
	DiagnosticsJSON string
	AttachmentCount int
	ReplyCount      int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Attachments     []Attachment
	Replies         []Reply
}

type Attachment struct {
	ID           uint64
	FeedbackID   uint64
	OriginalName string
	StoragePath  string
	MimeType     string
	FileSize     uint64
	Width        int
	Height       int
	CreatedAt    time.Time
}

type Reply struct {
	ID               uint64
	FeedbackID       uint64
	AdminUserID      uint64
	AdminDisplayName string
	Message          string
	CreatedAt        time.Time
}

type ImageUpload struct {
	FileName string
	Reader   io.Reader
}

type CreateInput struct {
	UserID      uint64
	GroupID     uint64
	Message     string
	Source      Source
	LogID       string
	Diagnostics map[string]string
	Images      []ImageUpload
}

type AutomaticSettings struct {
	Enabled         bool     `json:"enabled"`
	MutedErrorTypes []string `json:"muted_error_types"`
}

type AutomaticCreateResult struct {
	Feedback *UserView
	Reason   string
}

type AttachmentView struct {
	ID           uint64 `json:"id"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	FileSize     uint64 `json:"file_size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
}

type ReplyView struct {
	ID               uint64 `json:"id"`
	AdminDisplayName string `json:"admin_display_name"`
	Message          string `json:"message"`
	CreatedAt        string `json:"created_at"`
}

type UserView struct {
	ID              uint64           `json:"id"`
	Message         string           `json:"message"`
	Source          Source           `json:"source"`
	Status          Status           `json:"status"`
	AttachmentCount int              `json:"attachment_count"`
	ReplyCount      int              `json:"reply_count"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
	Attachments     []AttachmentView `json:"attachments,omitempty"`
	Replies         []ReplyView      `json:"replies,omitempty"`
}

type AdminView struct {
	UserView
	GroupID       uint64            `json:"group_id,omitempty"`
	UserID        uint64            `json:"user_id,omitempty"`
	Username      string            `json:"username,omitempty"`
	DisplayName   string            `json:"display_name,omitempty"`
	MemberName    string            `json:"member_name,omitempty"`
	GroupName     string            `json:"group_name,omitempty"`
	LegacyName    string            `json:"legacy_name,omitempty"`
	LegacyContact string            `json:"legacy_contact,omitempty"`
	LogID         string            `json:"log_id,omitempty"`
	Diagnostics   map[string]string `json:"diagnostics,omitempty"`
}

type StoredObject struct {
	StoragePath string
	FileSize    uint64
}

type ResolvedObject struct {
	AbsolutePath string
}

type Storage interface {
	Save(ctx context.Context, extension string, content []byte) (*StoredObject, error)
	Resolve(ctx context.Context, storagePath string) (*ResolvedObject, error)
	Delete(ctx context.Context, storagePath string) error
}
