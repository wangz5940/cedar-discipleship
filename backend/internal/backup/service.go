package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"agp/backend/internal/learning"
)

var (
	ErrBackupConfirmationRequired = errors.New("backup_confirmation_required")
	ErrBackupContentRequired      = errors.New("backup_content_required")
	ErrBackupGroupMismatch        = errors.New("backup_group_mismatch")
	ErrBackupRoleChangeForbidden  = errors.New("backup_role_change_forbidden")
	ErrBackupVersionUnsupported   = errors.New("backup_version_unsupported")
)

const backupConfirmationTTL = 5 * time.Minute

type Service struct {
	repo           Repository
	confirmationMu sync.Mutex
	confirmations  map[backupConfirmationKey]backupConfirmation
}

type backupConfirmationKey struct {
	groupID uint64
	actorID uint64
}

type backupConfirmation struct {
	value     string
	digest    [sha256.Size]byte
	expiresAt time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:          repo,
		confirmations: make(map[backupConfirmationKey]backupConfirmation),
	}
}

func (s *Service) CheckinDetails(ctx context.Context, groupID uint64, loc *time.Location) ([]CheckinDetail, error) {
	return s.repo.CheckinDetails(ctx, groupID, loc)
}

func (s *Service) FeedbackExports(ctx context.Context, groupID uint64, loc *time.Location) ([]FeedbackExport, error) {
	return s.repo.FeedbackExports(ctx, groupID, loc)
}

func (s *Service) LocalBackup(ctx context.Context, groupID uint64, exportedAt string) (Payload, error) {
	snapshot, err := s.repo.LocalBackupSnapshot(ctx, groupID)
	if err != nil {
		return Payload{}, err
	}
	return Payload{
		Version:    CurrentVersion,
		ExportedAt: exportedAt,
		Group: map[string]any{
			"id":          snapshot.Group.ID,
			"code":        snapshot.Group.Code,
			"name":        snapshot.Group.Name,
			"description": snapshot.Group.Description,
		},
		Settings:  snapshot.Settings,
		Members:   snapshot.Members,
		Weeks:     snapshot.Weeks,
		Checkins:  snapshot.Checkins,
		Feedbacks: snapshot.Feedbacks,
		Assets:    snapshot.Assets,
	}, nil
}

func (s *Service) ReplaceStudyWeeks(ctx context.Context, groupID uint64, weeks []learning.WeekInput, now time.Time) error {
	return s.repo.ReplaceStudyWeeks(ctx, groupID, weeks, now)
}

func (s *Service) PrepareLocalBackupImport(
	ctx context.Context,
	groupID, actorID uint64,
	payload Payload,
	now time.Time,
) (string, error) {
	if err := s.validateLocalBackup(ctx, groupID, payload); err != nil {
		return "", err
	}
	digest, err := backupPayloadDigest(payload)
	if err != nil {
		return "", err
	}
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	confirmation := strings.ToUpper(hex.EncodeToString(random[:]))
	key := backupConfirmationKey{groupID: groupID, actorID: actorID}
	s.confirmationMu.Lock()
	defer s.confirmationMu.Unlock()
	for existingKey, existing := range s.confirmations {
		if !now.Before(existing.expiresAt) {
			delete(s.confirmations, existingKey)
		}
	}
	s.confirmations[key] = backupConfirmation{
		value:     confirmation,
		digest:    digest,
		expiresAt: now.Add(backupConfirmationTTL),
	}
	return confirmation, nil
}

func (s *Service) ImportLocalBackup(
	ctx context.Context,
	groupID, actorID uint64,
	payload Payload,
	confirmation string,
	now time.Time,
) error {
	if err := s.validateLocalBackup(ctx, groupID, payload); err != nil {
		return err
	}
	digest, err := backupPayloadDigest(payload)
	if err != nil {
		return err
	}
	if !s.consumeBackupConfirmation(groupID, actorID, strings.TrimSpace(confirmation), digest, now) {
		return ErrBackupConfirmationRequired
	}
	return s.repo.ImportLocalBackup(ctx, groupID, actorID, payload, now)
}

func (s *Service) validateLocalBackup(ctx context.Context, groupID uint64, payload Payload) error {
	if payload.Version != CurrentVersion {
		return ErrBackupVersionUnsupported
	}
	if strings.TrimSpace(payload.ExportedAt) == "" {
		return ErrBackupContentRequired
	}
	if len(payload.Settings) == 0 &&
		len(payload.Members) == 0 &&
		len(payload.Weeks) == 0 &&
		len(payload.Checkins) == 0 &&
		len(payload.Feedbacks) == 0 &&
		len(payload.Assets) == 0 {
		return ErrBackupContentRequired
	}
	group, err := s.repo.GroupInfo(ctx, groupID)
	if err != nil {
		return err
	}
	payloadGroupID, ok := backupGroupID(payload.Group["id"])
	if !ok || payloadGroupID != group.ID {
		return ErrBackupGroupMismatch
	}
	payloadGroupCode, ok := payload.Group["code"].(string)
	if !ok || strings.TrimSpace(payloadGroupCode) != strings.TrimSpace(group.Code) {
		return ErrBackupGroupMismatch
	}
	return nil
}

func (s *Service) consumeBackupConfirmation(
	groupID, actorID uint64,
	confirmation string,
	digest [sha256.Size]byte,
	now time.Time,
) bool {
	key := backupConfirmationKey{groupID: groupID, actorID: actorID}
	s.confirmationMu.Lock()
	defer s.confirmationMu.Unlock()
	expected, ok := s.confirmations[key]
	if !ok || !now.Before(expected.expiresAt) {
		delete(s.confirmations, key)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(confirmation), []byte(expected.value)) != 1 ||
		subtle.ConstantTimeCompare(digest[:], expected.digest[:]) != 1 {
		return false
	}
	delete(s.confirmations, key)
	return true
}

func backupPayloadDigest(payload Payload) ([sha256.Size]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(data), nil
}

func backupGroupID(value any) (uint64, bool) {
	switch id := value.(type) {
	case uint64:
		return id, id > 0
	case uint:
		return uint64(id), id > 0
	case int:
		return uint64(id), id > 0
	case int64:
		return uint64(id), id > 0
	case float64:
		if id <= 0 || id > math.MaxUint64 || math.Trunc(id) != id {
			return 0, false
		}
		return uint64(id), true
	default:
		return 0, false
	}
}
