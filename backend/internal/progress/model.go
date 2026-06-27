package progress

type Kind string

const (
	KindPage    Kind = "page"
	KindPercent Kind = "percent"
	KindCount   Kind = "count"
	KindDone    Kind = "done"
)

type Progress struct {
	ID        uint64
	GroupID   uint64
	UserID    uint64
	ContentID uint64
	TaskID    uint64
	Kind      Kind
	Value     int64
	Completed bool
	UpdatedAt string
}
