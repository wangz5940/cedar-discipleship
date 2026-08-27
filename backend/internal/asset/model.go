package asset

import "time"

const (
	AssetKindOwned    = "owned"
	AssetKindImported = "imported"
)

type ShareScope string

const (
	ShareScopePrivate        ShareScope = "private"
	ShareScopeAllGroups      ShareScope = "all_groups"
	ShareScopeSelectedGroups ShareScope = "selected_groups"
)

type Asset struct {
	ID             uint64
	GroupID        uint64
	ResourceKey    string
	AssetKind      string
	SourceAssetID  uint64
	ImportedAt     *time.Time
	Category       string
	Title          string
	OriginalName   string
	StoragePath    string
	MimeType       string
	FileSize       uint64
	ChecksumSHA256 string
	Visibility     string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type StoredObject struct {
	StoragePath    string
	FileSize       uint64
	ChecksumSHA256 string
}

type ResolvedObject struct {
	AbsolutePath string
	OriginalName string
}

type DownloadFile struct {
	AbsolutePath string
	OriginalName string
	MimeType     string
}

type Binding struct {
	AssetID       uint64
	GroupID       uint64
	ResourceKey   string
	AssetKind     string
	SourceAssetID uint64
	ImportedAt    *time.Time
	DeletedAt     *time.Time
}

type GroupInfo struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type SharingSettings struct {
	AssetID          uint64     `json:"asset_id"`
	OwnerGroupID     uint64     `json:"owner_group_id"`
	Scope            ShareScope `json:"scope"`
	ConsumerGroupIDs []uint64   `json:"consumer_group_ids"`
}

type SharedFilter struct {
	OwnerGroupID uint64
	Category     string
	Status       string
}

type SharedResource struct {
	AssetID         uint64     `json:"asset_id"`
	OwnerGroup      GroupInfo  `json:"owner_group"`
	Category        string     `json:"category"`
	Title           string     `json:"title"`
	OriginalName    string     `json:"original_name"`
	StoragePath     string     `json:"-"`
	MimeType        string     `json:"mime_type"`
	FileSize        uint64     `json:"file_size"`
	ChecksumSHA256  string     `json:"checksum_sha256"`
	UpdatedAt       time.Time  `json:"updated_at"`
	Imported        bool       `json:"imported"`
	ImportedAssetID uint64     `json:"imported_asset_id,omitempty"`
	ImportedAt      *time.Time `json:"imported_at,omitempty"`
}

type ImportPreview struct {
	Allowed     bool           `json:"allowed"`
	SourceGroup GroupInfo      `json:"source_group"`
	Resource    SharedResource `json:"resource"`
	Conflicts   []string       `json:"conflicts"`
	Permissions []string       `json:"permissions"`
	Imported    bool           `json:"imported"`
	ImportedAt  *time.Time     `json:"imported_at,omitempty"`
}

type ImportEvent struct {
	ID              uint64    `json:"id"`
	TargetGroupID   uint64    `json:"target_group_id"`
	ImportedAssetID uint64    `json:"imported_asset_id"`
	SourceAssetID   uint64    `json:"source_asset_id"`
	EventType       string    `json:"event_type"`
	ActorUserID     uint64    `json:"actor_user_id"`
	Detail          string    `json:"detail,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

type DependencyNode struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Type    string `json:"type,omitempty"`
	GroupID uint64 `json:"group_id,omitempty"`
	AssetID uint64 `json:"asset_id,omitempty"`
}

type DependencyEdge struct {
	ID            string `json:"id"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	SourceGroupID uint64 `json:"source_group_id"`
	TargetGroupID uint64 `json:"target_group_id"`
	Strength      int    `json:"strength"`
	Status        string `json:"status"`
}

type DependencyMetrics struct {
	Imports          int `json:"imports"`
	Dependents       int `json:"dependents"`
	MaxDepth         int `json:"max_depth"`
	BrokenReferences int `json:"broken_references"`
}

type DependencyGraph struct {
	Nodes   []DependencyNode  `json:"nodes"`
	Edges   []DependencyEdge  `json:"edges"`
	Metrics DependencyMetrics `json:"metrics"`
}

type DependencyFilter struct {
	GroupID  uint64
	Category string
	Strength string
	Depth    int
}

type BatchShareInput struct {
	AssetIDs         []uint64   `json:"asset_ids"`
	Scope            ShareScope `json:"scope"`
	ConsumerGroupIDs []uint64   `json:"consumer_group_ids"`
}

type BatchDeleteInput struct {
	AssetIDs []uint64 `json:"asset_ids"`
}

type BatchImportInput struct {
	SourceAssetIDs []uint64 `json:"source_asset_ids"`
}

type BatchShareResult struct {
	AssetIDs []uint64   `json:"asset_ids"`
	Scope    ShareScope `json:"scope"`
	Count    int        `json:"count"`
}

type BatchDeleteResult struct {
	AssetIDs      []uint64 `json:"asset_ids"`
	DeletedIDs    []uint64 `json:"deleted_ids"`
	OwnedCount    int      `json:"owned_count"`
	ImportedCount int      `json:"imported_count"`
	Count         int      `json:"count"`
}

type BatchImportResult struct {
	SourceAssetIDs   []uint64 `json:"source_asset_ids"`
	ImportedAssetIDs []uint64 `json:"imported_asset_ids"`
	Count            int      `json:"count"`
}
