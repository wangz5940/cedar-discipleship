package backup

import "agp/backend/internal/learning"

type Member struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	NamePinyin  string   `json:"name_pinyin"`
	Roles       []string `json:"roles"`
}

type Checkin struct {
	Username    string `json:"username"`
	LogicalDate string `json:"logical_date"`
	CheckinTime string `json:"checkin_time"`
	TaskType    string `json:"task_type"`
	Part        string `json:"part"`
	Detail      string `json:"detail"`
	Note        string `json:"note"`
	IsRetro     bool   `json:"is_retro"`
}

type Feedback struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	Contact   string `json:"contact"`
	Message   string `json:"message"`
	Page      string `json:"page"`
	UserAgent string `json:"user_agent"`
	CreatedAt string `json:"created_at"`
}

type Asset struct {
	Category     string `json:"category"`
	Title        string `json:"title"`
	OriginalName string `json:"original_name"`
	StoragePath  string `json:"storage_path"`
	MimeType     string `json:"mime_type"`
	FileSize     uint64 `json:"file_size"`
}

type Payload struct {
	Version    int                  `json:"version"`
	ExportedAt string               `json:"exported_at"`
	Group      map[string]any       `json:"group"`
	Settings   map[string]any       `json:"settings"`
	Members    []Member             `json:"members"`
	Weeks      []learning.WeekInput `json:"weeks"`
	Checkins   []Checkin            `json:"checkins"`
	Feedbacks  []Feedback           `json:"feedbacks"`
	Assets     []Asset              `json:"assets"`
}

type GroupInfo struct {
	ID          uint64
	Code        string
	Name        string
	Description string
}

type CheckinDetail struct {
	ID          uint64
	LogicalDate string
	CheckinTime string
	TaskType    string
	Part        string
	Detail      string
	Note        string
	Username    string
	MemberName  string
	IsRetro     bool
}

type DailySummary struct {
	LogicalDate    string
	TotalCheckins  int
	CheckedMembers int
	DevotionCount  int
	BookCount      int
	VideoCount     int
	VerseCount     int
}

type FeedbackExport struct {
	CreatedAt string
	Username  string
	Name      string
	Contact   string
	Message   string
	Page      string
	UserAgent string
}
