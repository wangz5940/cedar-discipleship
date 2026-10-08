package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type accountReadState struct {
	Own   map[uint64]uint64 `json:"own"`
	Admin map[uint64]bool   `json:"admin"`
}

type readStateDocument struct {
	Since    time.Time                   `json:"since"`
	Accounts map[uint64]accountReadState `json:"accounts"`
}

// ReadStateStore has one backend process as its writer, like notification state.
type ReadStateStore struct {
	mu       sync.Mutex
	path     string
	since    time.Time
	document readStateDocument
}

func NewReadStateStore(root string, now time.Time) (*ReadStateStore, error) {
	path := filepath.Join(root, "feedback", "read-state.json")
	if err := rejectStorageSymlinks(root, path); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	s := &ReadStateStore{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s.document = readStateDocument{Since: now.UTC(), Accounts: make(map[uint64]accountReadState)}
		if err := s.write(s.document); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if err := json.Unmarshal(data, &s.document); err != nil {
		return nil, err
	}
	if s.document.Since.IsZero() || s.document.Accounts == nil {
		return nil, errors.New("invalid_feedback_read_state")
	}
	s.since = s.document.Since
	return s, nil
}

func (s *ReadStateStore) unread(userID uint64, admin bool, candidates []UnreadCandidate) []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.document.Accounts[userID]
	ids := make([]uint64, 0)
	for _, item := range candidates {
		if admin && !account.Admin[item.ID] || !admin && item.LastReplyID > account.Own[item.ID] {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func (s *ReadStateStore) mark(ctx context.Context, userID, feedbackID uint64, admin bool, replyID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	document := readStateDocument{Since: s.since, Accounts: make(map[uint64]accountReadState, len(s.document.Accounts)+1)}
	for id, account := range s.document.Accounts {
		document.Accounts[id] = account
	}
	previous := s.document.Accounts[userID]
	account := accountReadState{Own: make(map[uint64]uint64, len(previous.Own)+1), Admin: make(map[uint64]bool, len(previous.Admin)+1)}
	for id, cursor := range previous.Own {
		account.Own[id] = cursor
	}
	for id, seen := range previous.Admin {
		account.Admin[id] = seen
	}
	if admin {
		account.Admin[feedbackID] = true
	} else if replyID > account.Own[feedbackID] {
		account.Own[feedbackID] = replyID
	}
	document.Accounts[userID] = account
	if err := s.write(document); err != nil {
		return err
	}
	s.document = document
	return nil
}

func (s *ReadStateStore) write(document readStateDocument) error {
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".read-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), s.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Service) Unread(ctx context.Context, userID uint64, admin bool) (*UnreadView, error) {
	if s.reads == nil {
		return nil, errors.New("feedback_read_state_unavailable")
	}
	own, err := s.repo.UnreadCandidates(ctx, userID, false, s.reads.since)
	if err != nil {
		return nil, err
	}
	view := &UnreadView{OwnIDs: s.reads.unread(userID, false, own), AdminIDs: []uint64{}}
	if admin {
		candidates, err := s.repo.UnreadCandidates(ctx, userID, true, s.reads.since)
		if err != nil {
			return nil, err
		}
		view.AdminIDs = s.reads.unread(userID, true, candidates)
	}
	return view, nil
}

func (s *Service) MarkRead(ctx context.Context, userID, feedbackID uint64, admin bool, replyID uint64) error {
	if s.reads == nil {
		return errors.New("feedback_read_state_unavailable")
	}
	var item *Feedback
	var err error
	if admin {
		item, err = s.repo.FindByID(ctx, feedbackID)
	} else {
		item, err = s.repo.FindByUser(ctx, userID, feedbackID)
	}
	if err != nil {
		return err
	}
	if !admin && replyID > 0 {
		found := false
		for _, reply := range item.Replies {
			if reply.ID == replyID {
				found = true
				break
			}
		}
		if !found {
			return ErrNotFound
		}
	}
	return s.reads.mark(ctx, userID, feedbackID, admin, replyID)
}
