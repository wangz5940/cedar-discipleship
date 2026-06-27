package content

type Type string

const (
	TypeBook     Type = "book"
	TypeVideo    Type = "video"
	TypeAudio    Type = "audio"
	TypeMarkdown Type = "markdown"
	TypePDF      Type = "pdf"
)

type Content struct {
	ID             uint64
	GroupID        uint64
	Title          string
	Type           Type
	Description    string
	CoverAssetID   uint64
	PrimaryAssetID uint64
	Status         int
}
