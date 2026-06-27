package backup

import (
	"context"
	"time"

	"agp/backend/internal/learning"
)

type Repository interface {
	CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error)
	DailySummaries(ctx context.Context, groupID uint64) (int, []DailySummary, error)
	FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error)
	GroupInfo(ctx context.Context, groupID uint64) (*GroupInfo, error)
	BackupMembers(ctx context.Context, groupID uint64) ([]Member, error)
	BackupCheckins(ctx context.Context, groupID uint64) ([]Checkin, error)
	BackupFeedbacks(ctx context.Context, groupID uint64) ([]Feedback, error)
	BackupAssets(ctx context.Context, groupID uint64) ([]Asset, error)
	ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error
	ImportLocalBackup(ctx context.Context, groupID, actorID uint64, payload Payload, now time.Time) error
}
