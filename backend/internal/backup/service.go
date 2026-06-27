package backup

import (
	"context"
	"time"

	"agp/backend/internal/learning"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error) {
	return s.repo.CheckinDetails(ctx, groupID, loc)
}

func (s *Service) DailySummaries(ctx context.Context, groupID uint64) (int, []DailySummary, error) {
	return s.repo.DailySummaries(ctx, groupID)
}

func (s *Service) FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error) {
	return s.repo.FeedbackExports(ctx, groupID, loc)
}

func (s *Service) LocalBackup(ctx context.Context, groupID uint64, settings map[string]any, weeks []learning.WeekInput, exportedAt string) (Payload, error) {
	group, err := s.repo.GroupInfo(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	members, err := s.repo.BackupMembers(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	checkins, err := s.repo.BackupCheckins(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	feedbacks, err := s.repo.BackupFeedbacks(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	assets, err := s.repo.BackupAssets(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	return Payload{
		Version:    1,
		ExportedAt: exportedAt,
		Group: map[string]any{
			"id":          group.ID,
			"code":        group.Code,
			"name":        group.Name,
			"description": group.Description,
		},
		Settings:  settings,
		Members:   members,
		Weeks:     weeks,
		Checkins:  checkins,
		Feedbacks: feedbacks,
		Assets:    assets,
	}, nil
}

func (s *Service) ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error {
	return s.repo.ReplaceStudyWeeks(ctx, groupID, weeks, now)
}

func (s *Service) ImportLocalBackup(ctx context.Context, groupID, actorID uint64, payload Payload, now time.Time) error {
	return s.repo.ImportLocalBackup(ctx, groupID, actorID, payload, now)
}
