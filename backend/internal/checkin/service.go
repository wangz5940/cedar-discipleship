package checkin

import (
	"context"
	"database/sql"
	"errors"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, record *Record, actorID uint64) (uint64, bool, error) {
	switch record.TaskType {
	case "weekly_book":
		existingID, err := s.repo.FindExistingWeeklyBook(ctx, record.GroupID, record.UserID, record.TaskID, record.WeekID, record.Part, record.Detail)
		if err == nil {
			return existingID, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
	case "weekly_video", "weekly_verse":
		existingID, err := s.repo.FindExistingWeeklyTask(ctx, record.GroupID, record.UserID, record.TaskID, record.WeekID, record.TaskType)
		if err == nil {
			return existingID, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, err
		}
	}
	id, err := s.repo.Create(ctx, record, actorID)
	return id, false, err
}

func (s *Service) DeleteOwn(ctx context.Context, groupID, userID, recordID uint64) error {
	return s.repo.DeleteOwn(ctx, groupID, userID, recordID)
}

func (s *Service) DeleteAny(ctx context.Context, groupID, recordID uint64) error {
	return s.repo.DeleteAny(ctx, groupID, recordID)
}

func (s *Service) List(ctx context.Context, groupID uint64, from, to string, userID uint64, limit int) ([]Record, error) {
	return s.repo.List(ctx, groupID, from, to, userID, limit)
}
