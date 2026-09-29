package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const redactedValue = "[REDACTED]"

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, input CreateLogInput, now time.Time) error {
	beforeJSON, err := optionalJSON(input.Before)
	if err != nil {
		return fmt.Errorf("encode audit before state: %w", err)
	}
	afterJSON, err := optionalJSON(input.After)
	if err != nil {
		return fmt.Errorf("encode audit after state: %w", err)
	}
	return s.repo.Create(ctx, Log{
		GroupID:    input.GroupID,
		ActorID:    input.ActorID,
		Action:     input.Action,
		TargetType: input.TargetType,
		TargetID:   input.TargetID,
		BeforeJSON: beforeJSON,
		AfterJSON:  afterJSON,
		IP:         truncateRunes(input.IP, 64),
		UserAgent:  truncateRunes(input.UserAgent, 512),
		CreatedAt:  now.UTC().Format("2006-01-02 15:04:05.000"),
	})
}

func (s *Service) ListByGroup(ctx context.Context, groupID uint64, limit int) ([]LogVO, error) {
	items, err := s.repo.ListByGroup(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	return toLogVOs(items), nil
}

func (s *Service) ListAll(ctx context.Context, limit int) ([]LogVO, error) {
	items, err := s.repo.ListAll(ctx, limit)
	if err != nil {
		return nil, err
	}
	return toLogVOs(items), nil
}

func toLogVOs(items []Log) []LogVO {
	out := make([]LogVO, 0, len(items))
	for _, item := range items {
		out = append(out, LogVO{
			ID:               item.ID,
			GroupID:          item.GroupID,
			ActorUserID:      item.ActorID,
			ActorUsername:    item.ActorUsername,
			ActorDisplayName: item.ActorDisplayName,
			Action:           item.Action,
			TargetType:       item.TargetType,
			TargetID:         item.TargetID,
			Before:           optionalRawJSON(item.BeforeJSON),
			After:            optionalRawJSON(item.AfterJSON),
			CreatedAt:        item.CreatedAt,
		})
	}
	return out
}

func optionalJSON(v any) (string, error) {
	normalized, err := Sanitize(v)
	if err != nil {
		return "", err
	}
	if normalized == nil {
		return "", nil
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func Sanitize(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return redactValue("", normalized), nil
}

func optionalRawJSON(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	return json.RawMessage(value)
}

func redactValue(key string, value any) any {
	if sensitiveKey(key) {
		return redactedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			redacted[childKey] = redactValue(childKey, childValue)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, item := range typed {
			redacted[index] = redactValue(key, item)
		}
		return redacted
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if strings.HasSuffix(normalized, "_changed") {
		return false
	}
	if strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "secret") {
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "csrf")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
