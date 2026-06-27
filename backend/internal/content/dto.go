package content

type CreateContentRequest struct {
	Title          string `json:"title"`
	Type           Type   `json:"type"`
	Description    string `json:"description"`
	CoverAssetID   uint64 `json:"cover_asset_id"`
	PrimaryAssetID uint64 `json:"primary_asset_id"`
}

type ContentVO struct {
	ID          uint64 `json:"id"`
	Title       string `json:"title"`
	Type        Type   `json:"type"`
	Description string `json:"description"`
	CoverURL    string `json:"cover_url"`
}
