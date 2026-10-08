package feedback

import (
	"context"
	"time"
)

type Repository interface {
	UnreadCandidates(ctx context.Context, userID uint64, admin bool, since time.Time) ([]UnreadCandidate, error)
	Create(ctx context.Context, item Feedback, attachments []Attachment) (uint64, error)
	AutomaticSettings(ctx context.Context) (AutomaticSettings, error)
	SaveAutomaticSettings(
		ctx context.Context,
		settings AutomaticSettings,
		adminUserID uint64,
		updatedAt time.Time,
	) error
	ListByUser(ctx context.Context, userID uint64, source Source, limit int) ([]Feedback, error)
	FindByUser(ctx context.Context, userID, feedbackID uint64) (*Feedback, error)
	ListAll(ctx context.Context, status Status, source Source, limit int) ([]Feedback, error)
	FindByID(ctx context.Context, feedbackID uint64) (*Feedback, error)
	UpdateStatus(ctx context.Context, feedbackID uint64, status Status, updatedAt time.Time) (bool, error)
	CreateReply(ctx context.Context, reply Reply) (uint64, error)
	FindAttachment(ctx context.Context, feedbackID, attachmentID uint64) (*Attachment, error)
}
