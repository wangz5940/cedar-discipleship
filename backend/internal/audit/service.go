package audit

import (
	"context"
	"encoding/json"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateLogInput, now time.Time) error {
	beforeJSON, err := optionalJSON(input.Before)
	if err != nil {
		return err
	}
	afterJSON, err := optionalJSON(input.After)
	if err != nil {
		return err
	}
	return s.repo.Create(ctx, Log{
		GroupID:    input.GroupID,
		ActorID:    input.ActorID,
		Action:     input.Action,
		TargetType: input.TargetType,
		TargetID:   input.TargetID,
		BeforeJSON: beforeJSON,
		AfterJSON:  afterJSON,
		IP:         input.IP,
		UserAgent:  input.UserAgent,
		CreatedAt:  now.UTC().Format("2006-01-02 15:04:05.000"),
	})
}

func (s *Service) ListByGroup(ctx context.Context, groupID uint64, limit int) ([]LogVO, error) {
	items, err := s.repo.ListByGroup(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]LogVO, 0, len(items))
	for _, item := range items {
		out = append(out, LogVO{
			ID:          item.ID,
			ActorUserID: item.ActorID,
			Action:      item.Action,
			TargetType:  item.TargetType,
			TargetID:    item.TargetID,
			CreatedAt:   item.CreatedAt,
		})
	}
	return out, nil
}

func optionalJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if string(payload) == "null" {
		return "", nil
	}
	return string(payload), nil
}
