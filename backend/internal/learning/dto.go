package learning

type TaskBinding struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Type    string `json:"type"`
	AssetID uint64 `json:"asset_id"`
}

type WeekInput struct {
	StartDate      string        `json:"start_date"`
	EndDate        string        `json:"end_date"`
	Title          string        `json:"title"`
	VerseRef       string        `json:"verse_ref"`
	ReciteText     string        `json:"recite_text"`
	BookEnabled    bool          `json:"book_enabled"`
	VideoEnabled   bool          `json:"video_enabled"`
	VerseEnabled   bool          `json:"verse_enabled"`
	OutlineEnabled bool          `json:"outline_enabled"`
	Readings       []TaskBinding `json:"readings"`
	Videos         []TaskBinding `json:"videos"`
	Outline        TaskBinding   `json:"outline"`
}

type WeekVO struct {
	ID             uint64        `json:"id"`
	Start          string        `json:"start"`
	End            string        `json:"end"`
	Title          string        `json:"title"`
	VerseRef       string        `json:"verse_ref"`
	ReciteText     string        `json:"recite_text"`
	BookEnabled    bool          `json:"book_enabled"`
	VideoEnabled   bool          `json:"video_enabled"`
	VerseEnabled   bool          `json:"verse_enabled"`
	OutlineEnabled bool          `json:"outline_enabled"`
	Readings       []TaskBinding `json:"readings,omitempty"`
	Videos         []TaskBinding `json:"videos,omitempty"`
	Outline        TaskBinding   `json:"outline,omitempty"`
}

type TodayTaskVO struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	Kind      string           `json:"kind"`
	Title     string           `json:"title"`
	Summary   string           `json:"summary"`
	TaskID    uint64           `json:"task_id,omitempty"`
	WeekID    uint64           `json:"week_id,omitempty"`
	Part      string           `json:"part,omitempty"`
	Detail    string           `json:"detail,omitempty"`
	Content   string           `json:"content,omitempty"`
	Required  bool             `json:"required"`
	Status    string           `json:"status"`
	Completed bool             `json:"completed"`
	Record    *TodayRecord     `json:"record,omitempty"`
	Assets    []map[string]any `json:"assets,omitempty"`
}

type TodayProgress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Percent   int `json:"percent"`
}

type TodayRecord struct {
	ID          uint64  `json:"id"`
	UserID      uint64  `json:"user_id"`
	TaskID      *uint64 `json:"task_id,omitempty"`
	WeekID      *uint64 `json:"week_id,omitempty"`
	LogicalDate string  `json:"logical_date"`
	CheckinTime string  `json:"checkin_time"`
	TaskType    string  `json:"task_type"`
	Part        string  `json:"part"`
	Detail      string  `json:"detail"`
	Note        string  `json:"note"`
}

type TodayVO struct {
	Date        string         `json:"date"`
	Title       string         `json:"title"`
	Subtitle    string         `json:"subtitle"`
	CurrentWeek map[string]any `json:"current_week,omitempty"`
	Progress    TodayProgress  `json:"progress"`
	Tasks       []TodayTaskVO  `json:"tasks"`
	Records     []TodayRecord  `json:"records"`
}
