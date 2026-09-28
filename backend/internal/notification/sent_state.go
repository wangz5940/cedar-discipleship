package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type sentState struct {
	GroupID         uint64    `json:"group_id"`
	Target          Target    `json:"target"`
	Topic           string    `json:"topic"`
	Version         string    `json:"version"`
	PeriodID        string    `json:"period_id,omitempty"`
	Hash            string    `json:"hash"`
	Content         string    `json:"content"`
	SentAt          time.Time `json:"sent_at"`
	CoveredRecordID uint64    `json:"covered_record_id,omitempty"`
}

type sentStateStore struct {
	dir string
	mu  sync.Mutex
}

func newSentStateStore(queueDir string) (*sentStateStore, error) {
	store := &sentStateStore{dir: filepath.Join(queueDir, "last-sent")}
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create sent notification state directory: %w", err)
	}
	if err := store.bootstrap(filepath.Join(queueDir, "completed")); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *sentStateStore) NeedsSend(candidate sentState, recordID uint64) (bool, error) {
	if candidate.GroupID == 0 || !validTopic(candidate.Topic) || candidate.Version == "" || candidate.Hash == "" {
		return false, errors.New("invalid notification content state")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.readLocked(candidate.GroupID, candidate.Target, candidate.Topic)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if state.Version == candidate.Version || sameNotificationPeriod(state, candidate) {
		if recordID > 0 && recordID <= state.CoveredRecordID {
			return false, nil
		}
		if state.Hash == candidate.Hash {
			// Identical content is already delivered; retain its wider coverage.
			if candidate.CoveredRecordID > state.CoveredRecordID {
				state.CoveredRecordID = candidate.CoveredRecordID
				return false, s.writeLocked(state)
			}
			return false, nil
		}
		return true, nil
	}
	currentPeriod, currentOK := notificationVersionDate(candidate.Topic, candidate.Version)
	previousPeriod, previousOK := notificationVersionDate(candidate.Topic, state.Version)
	if currentOK && previousOK && currentPeriod.Before(previousPeriod) {
		return false, nil
	}
	return true, nil
}

func (s *sentStateStore) Record(state sentState) error {
	if state.GroupID == 0 || !validTopic(state.Topic) || state.Version == "" || state.Hash == "" {
		return errors.New("invalid sent notification state")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.readLocked(state.GroupID, state.Target, state.Topic)
	if err == nil && (previous.Version == state.Version || sameNotificationPeriod(previous, state)) {
		state.CoveredRecordID = max(state.CoveredRecordID, previous.CoveredRecordID)
	}
	return s.writeLocked(state)
}

func (s *sentStateStore) bootstrap(completedDir string) error {
	files, err := os.ReadDir(completedDir)
	if err != nil {
		return fmt.Errorf("read completed notifications: %w", err)
	}
	latest := make(map[string]sentState)
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(completedDir, file.Name()))
		if err != nil {
			return fmt.Errorf("read completed notification: %w", err)
		}
		var item job
		if json.Unmarshal(data, &item) != nil || item.Status != "sent" {
			continue
		}
		state := stateFromJob(item)
		if state.GroupID == 0 || !validTopic(state.Topic) || state.Hash == "" {
			continue
		}
		key := s.path(state.GroupID, state.Target, state.Topic)
		if previous, exists := latest[key]; !exists || previous.SentAt.Before(state.SentAt) {
			latest[key] = state
		}
	}
	for _, state := range latest {
		path := s.path(state.GroupID, state.Target, state.Topic)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("check sent notification state: %w", err)
		}
		if err := s.writeLocked(state); err != nil {
			return fmt.Errorf("migrate sent notification state: %w", err)
		}
	}
	return nil
}

func (s *sentStateStore) readLocked(groupID uint64, target Target, topic string) (sentState, error) {
	data, err := os.ReadFile(s.path(groupID, target, topic))
	if err != nil {
		return sentState{}, err
	}
	var state sentState
	if err := json.Unmarshal(data, &state); err != nil {
		return sentState{}, fmt.Errorf("decode sent notification state: %w", err)
	}
	if state.GroupID != groupID || state.Target != target || state.Topic != topic || state.Version == "" || state.Hash == "" {
		return sentState{}, errors.New("invalid sent notification state")
	}
	return state, nil
}

func (s *sentStateStore) writeLocked(state sentState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode sent notification state: %w", err)
	}
	if err := writeAtomic(s.path(state.GroupID, state.Target, state.Topic), data); err != nil {
		return fmt.Errorf("write sent notification state: %w", err)
	}
	return nil
}

func (s *sentStateStore) path(groupID uint64, target Target, topic string) string {
	return filepath.Join(s.dir, fmt.Sprintf("%020d-%020d-%d-%s.json", groupID, target.ChatID, target.ChatType, topic))
}

func stateFromJob(item job) sentState {
	topic := item.Topic
	if !validTopic(topic) {
		topic = inferTopic(strings.Join(item.Messages, "\n"))
	}
	content := item.CanonicalContent
	if content == "" {
		content = canonicalNotificationContent(strings.Join(item.Messages, "\n"))
	}
	hash := item.ContentHash
	if hash == "" {
		hash = contentHash(content)
	}
	version := item.ContentVersion
	if version == "" {
		version = legacyContentVersion(item, topic)
	}
	sentAt := item.SentAt
	if sentAt.IsZero() {
		sentAt = item.Event.OccurredAt
	}
	return sentState{
		GroupID:         item.Event.GroupID,
		Target:          item.Target,
		Topic:           topic,
		Version:         version,
		PeriodID:        item.PeriodID,
		Hash:            hash,
		Content:         content,
		SentAt:          sentAt,
		CoveredRecordID: max(item.CoveredRecordID, item.Event.RecordID),
	}
}

func sameNotificationPeriod(left, right sentState) bool {
	return left.PeriodID != "" && left.PeriodID == right.PeriodID
}

func legacyContentVersion(item job, topic string) string {
	if !item.ExpiresAt.IsZero() {
		return topic + ":" + item.ExpiresAt.AddDate(0, 0, -1).Format("2006-01-02")
	}
	if item.Event.LogicalDate != "" {
		return topic + ":" + item.Event.LogicalDate
	}
	if !item.Event.OccurredAt.IsZero() {
		return topic + ":" + item.Event.OccurredAt.Format("2006-01-02")
	}
	return ""
}

func canonicalNotificationContent(text string) string {
	text = strings.ReplaceAll(text, "【新】", "")
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = strings.Join(strings.Fields(lines[index]), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func contentHash(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func inferTopic(text string) string {
	switch {
	case strings.HasPrefix(strings.TrimSpace(text), "每日灵修"):
		return "daily"
	case strings.HasPrefix(strings.TrimSpace(text), "本周任务"):
		return "weekly"
	default:
		return ""
	}
}

func validTopic(topic string) bool {
	return topic == "daily" || topic == "weekly"
}

func notificationVersionDate(topic, version string) (time.Time, bool) {
	prefix := topic + ":"
	if !strings.HasPrefix(version, prefix) {
		return time.Time{}, false
	}
	value := strings.TrimPrefix(version, prefix)
	value = strings.TrimPrefix(value, "none:")
	period, err := time.Parse("2006-01-02", value)
	return period, err == nil
}
