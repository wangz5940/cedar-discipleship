package backup

import (
	"context"
	"time"

	"agp/backend/internal/learning"
)

type Repository interface {
	CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error)
	FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error)
	GroupInfo(ctx context.Context, groupID uint64) (*GroupInfo, error)
	LocalBackupSnapshot(ctx context.Context, groupID uint64) (Snapshot, error)
	ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error
	ImportLocalBackup(ctx context.Context, groupID, actorID uint64, payload Payload, now time.Time) error
}
