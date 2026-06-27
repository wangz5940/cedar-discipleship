package learning

type Plan struct {
	ID          uint64
	GroupID     uint64
	Name        string
	Description string
	Status      int
}

type Week struct {
	ID             uint64
	GroupID        uint64
	StartDate      string
	EndDate        string
	Title          string
	VerseRef       string
	ReciteText     string
	BookEnabled    bool
	VideoEnabled   bool
	VerseEnabled   bool
	OutlineEnabled bool
}

type Task struct {
	ID       uint64
	GroupID  uint64
	WeekID   uint64
	TaskType string
	Title    string
	Content  string
	Required bool
	Enabled  bool
	Assets   []TaskAsset
}

type TaskAsset struct {
	ID           uint64
	Category     string
	Title        string
	OriginalName string
	UsageType    string
}

type TaskDraft struct {
	TaskType  string
	Title     string
	Content   string
	SortOrder int
	AssetID   uint64
	UsageType string
}
