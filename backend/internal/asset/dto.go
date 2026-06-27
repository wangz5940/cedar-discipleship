package asset

import "io"

type UploadRequest struct {
	GroupID  uint64
	ActorID  uint64
	Category string
	FileName string
	Reader   io.Reader
}

type CreateRequest struct {
	GroupID     uint64
	ActorID     uint64
	Category    string
	Title       string
	StoragePath string
	MimeType    string
	FileSize    uint64
}

type AssetVO struct {
	ID           uint64 `json:"id"`
	Category     string `json:"category"`
	Title        string `json:"title"`
	OriginalName string `json:"original_name"`
	MimeType     string `json:"mime_type"`
	FileSize     uint64 `json:"file_size"`
	URL          string `json:"url"`
}

type LibrarySection struct {
	Key   string        `json:"key"`
	Label string        `json:"label"`
	Items []LibraryItem `json:"items"`
	Count int           `json:"count"`
}

type LibraryItem struct {
	ID           uint64 `json:"id,omitempty"`
	Title        string `json:"title"`
	OriginalName string `json:"original_name"`
	URL          string `json:"url"`
	Category     string `json:"category"`
	Source       string `json:"source"`
	Type         string `json:"type"`
}
