package audit

type Log struct {
	ID         uint64
	GroupID    uint64
	ActorID    uint64
	Action     string
	TargetType string
	TargetID   uint64
	BeforeJSON string
	AfterJSON  string
	IP         string
	UserAgent  string
	CreatedAt  string
}
