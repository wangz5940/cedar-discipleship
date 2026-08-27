package asset

import (
	"io"
	"time"
)

type UploadRequest struct {
	GroupID  uint64
	ActorID  uint64
	Category string
	FileName string
	Reader   io.Reader
}

type AssetVO struct {
	ID            uint64     `json:"id"`
	Category      string     `json:"category"`
	Title         string     `json:"title"`
	OriginalName  string     `json:"original_name"`
	MimeType      string     `json:"mime_type"`
	FileSize      uint64     `json:"file_size"`
	URL           string     `json:"url"`
	AssetKind     string     `json:"asset_kind,omitempty"`
	SourceAssetID uint64     `json:"source_asset_id,omitempty"`
	ImportedAt    *time.Time `json:"imported_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
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

type ShareInput struct {
	Scope            ShareScope `json:"scope"`
	ConsumerGroupIDs []uint64   `json:"consumer_group_ids"`
}

type ImportInput struct {
	SourceAssetID uint64 `json:"source_asset_id"`
}
