package feedback

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

type unreadRepository struct {
	Repository
	own   []UnreadCandidate
	admin []UnreadCandidate
	item  Feedback
}

func (r *unreadRepository) UnreadCandidates(_ context.Context, _ uint64, admin bool, _ time.Time) ([]UnreadCandidate, error) {
	if admin {
		return r.admin, nil
	}
	return r.own, nil
}
func (r *unreadRepository) FindByID(_ context.Context, id uint64) (*Feedback, error) {
	if id != r.item.ID {
		return nil, ErrNotFound
	}
	item := r.item
	return &item, nil
}
func (r *unreadRepository) FindByUser(ctx context.Context, userID, id uint64) (*Feedback, error) {
	if userID != r.item.UserID {
		return nil, ErrNotFound
	}
	return r.FindByID(ctx, id)
}

func TestUnreadFeedbackReadPersistsPerItemAndAccount(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	reads, err := NewReadStateStore(root, now)
	if err != nil {
		t.Fatal(err)
	}
	repo := &unreadRepository{own: []UnreadCandidate{{ID: 41, LastReplyID: 10}, {ID: 42, LastReplyID: 11}}, admin: []UnreadCandidate{{ID: 41}, {ID: 42}}, item: Feedback{ID: 41, UserID: 11, Replies: []Reply{{ID: 10}, {ID: 12}}}}
	service := NewService(repo, nil, reads)
	view, err := service.Unread(t.Context(), 11, false)
	if err != nil || !reflect.DeepEqual(view.OwnIDs, []uint64{41, 42}) || len(view.AdminIDs) != 0 {
		t.Fatalf("initial=%+v err=%v", view, err)
	}
	// The page displayed reply 10; reply 12 arrived before its read request.
	repo.own[0].LastReplyID = 12
	if err := service.MarkRead(t.Context(), 11, 41, false, 10); err != nil {
		t.Fatal(err)
	}
	view, err = service.Unread(t.Context(), 11, false)
	if err != nil || !reflect.DeepEqual(view.OwnIDs, []uint64{41, 42}) {
		t.Fatalf("new reply cleared prematurely: %+v err=%v", view, err)
	}
	if err := service.MarkRead(t.Context(), 11, 41, false, 12); err != nil {
		t.Fatal(err)
	}
	if err := service.MarkRead(t.Context(), 99, 41, true, 0); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewReadStateStore(root, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	service = NewService(repo, nil, reloaded)
	view, err = service.Unread(t.Context(), 11, false)
	if err != nil || !reflect.DeepEqual(view.OwnIDs, []uint64{42}) || !reloaded.since.Equal(now.UTC()) {
		t.Fatalf("reload=%+v err=%v", view, err)
	}
	view, err = service.Unread(t.Context(), 99, true)
	if err != nil || !reflect.DeepEqual(view.AdminIDs, []uint64{42}) {
		t.Fatalf("admin=%+v err=%v", view, err)
	}
	view, err = service.Unread(t.Context(), 100, true)
	if err != nil || !reflect.DeepEqual(view.AdminIDs, []uint64{41, 42}) {
		t.Fatalf("other admin=%+v err=%v", view, err)
	}
	for _, tc := range []struct {
		name        string
		user, reply uint64
	}{{"other account", 12, 12}, {"forged reply", 11, 999}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := service.MarkRead(t.Context(), tc.user, 41, false, tc.reply); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unauthorized read=%v", err)
			}
		})
	}
}

func TestFeedbackReadFailurePreservesUnread(t *testing.T) {
	reads, err := NewReadStateStore(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(reads.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(reads.path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := reads.mark(t.Context(), 11, 41, true, 0); err == nil {
		t.Fatal("expected write failure")
	}
	if got := reads.unread(11, true, []UnreadCandidate{{ID: 41}}); !reflect.DeepEqual(got, []uint64{41}) {
		t.Fatalf("read on failed persistence=%v", got)
	}
}

func TestFeedbackConcurrentReadCursorsNeverMoveBackwards(t *testing.T) {
	reads, err := NewReadStateStore(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for cursor := uint64(1); cursor <= 20; cursor++ {
		group.Go(func() {
			if err := reads.mark(t.Context(), 11, 41, false, cursor); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if got := reads.unread(11, false, []UnreadCandidate{{ID: 41, LastReplyID: 20}, {ID: 42, LastReplyID: 1}}); !reflect.DeepEqual(got, []uint64{42}) {
		t.Fatalf("concurrent read=%v", got)
	}
}
