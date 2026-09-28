//go:build integration

package ministry

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func requestRoundFixture(t *testing.T) (*MySQLRepository, *sql.DB) {
	t.Helper()
	db := testdb.Open(t)
	testdb.Apply(t, db, "003_ministry_groups.sql")
	testdb.Apply(t, db, "006_ministry_content_deletions.sql")
	testdb.Apply(t, db, "014_ministry_submission_rounds.sql")
	testdb.Apply(t, db, "014_ministry_submission_rounds.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'member','Member','member',NOW(),NOW()),(2,'reviewer','Reviewer','reviewer',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'Member',NOW(),NOW(),NOW()),(1,2,'Reviewer',NOW(),NOW(),NOW());
		INSERT INTO ministry_groups(id,study_group_id,code,name,leader_user_id,created_at,updated_at)
		VALUES (1,1,'test','Test',2,NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at)
		VALUES (1,2,'group_admin',NOW())`)
	return NewMySQLRepository(db), db
}

func TestJoinAndShareNotificationsFollowSubmissionRounds(t *testing.T) {
	repo, db := requestRoundFixture(t)
	at := time.Now()
	assertCount := func(kind string, want int) {
		t.Helper()
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_notifications WHERE notification_type=?`, kind).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Errorf("%s notifications=%d want=%d", kind, count, want)
		}
	}
	var id uint64
	for round := 1; round <= 2; round++ {
		got, err := repo.RequestJoin(t.Context(), 1, 1, 1, "join", false, at)
		if err != nil {
			t.Fatal(err)
		}
		if round > 1 && got != id {
			t.Fatal("reapplication changed request identity")
		}
		id = got
		if again, err := repo.RequestJoin(t.Context(), 1, 1, 1, "retry", false, at); err != nil || again != id {
			t.Fatalf("pending retry=%d err=%v", again, err)
		}
		assertCount("join_request", round)
		if err := repo.DecideRequest(t.Context(), 1, id, 2, StatusRejected, at); err != nil {
			t.Fatal(err)
		}
		if err := repo.DecideRequest(t.Context(), 1, id, 2, StatusRejected, at); !errors.Is(err, ErrRequestAlreadyReviewed) {
			t.Fatalf("decision retry=%v", err)
		}
		assertCount("join_decision", round)
	}
	for round := 1; round <= 2; round++ {
		if _, err := repo.RequestJoin(t.Context(), 1, 1, 1, "auto", true, at); err != nil {
			t.Fatal(err)
		}
		assertCount("join_decision", 2+round)
		if err := repo.Leave(t.Context(), 1, 1, 1, at); err != nil {
			t.Fatal(err)
		}
	}
	share, err := repo.CreateShare(t.Context(), 1, 1, 1, ShareInput{Title: "Title", Body: "Body"}, StatusPending, at)
	if err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		assertCount("share_review", round)
		if err := repo.DecideShare(t.Context(), 1, 1, share, 2, StatusPublished, at); err != nil {
			t.Fatal(err)
		}
		if err := repo.DecideShare(t.Context(), 1, 1, share, 2, StatusPublished, at); !errors.Is(err, ErrShareAlreadyReviewed) {
			t.Fatalf("share decision retry=%v", err)
		}
		assertCount("share_decision", round)
		if round == 1 {
			if err := repo.UpdateShare(t.Context(), 1, 1, share, 1, ShareInput{Title: "Edited", Body: "Body"}, StatusPending, false, at); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDelayedJoinCannotReopenApprovedRequest(t *testing.T) {
	repo, db := requestRoundFixture(t)
	at := time.Now()
	id, err := repo.RequestJoin(t.Context(), 1, 1, 1, "join", false, at)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var lockedID uint64
	for _, query := range []string{
		`SELECT id FROM ministry_groups WHERE id=1 FOR UPDATE`,
		`SELECT id FROM ministry_group_requests WHERE id=? FOR UPDATE`,
	} {
		args := []any{}
		if query != `SELECT id FROM ministry_groups WHERE id=1 FOR UPDATE` {
			args = append(args, id)
		}
		if err := tx.QueryRowContext(t.Context(), query, args...).Scan(&lockedID); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, err := repo.RequestJoin(ctx, 1, 1, 1, "delayed", false, at)
		done <- err
	}()
	// Wait for an actual database lock wait, not a scheduling delay.
	testdb.WaitForLockWait(t, db)
	if _, err := tx.ExecContext(t.Context(), `UPDATE ministry_group_requests
		SET status='approved',reviewed_by=2,reviewed_at=? WHERE id=?`, at, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(t.Context(), `INSERT INTO ministry_group_members
		(study_group_id,ministry_group_id,user_id,status,joined_at,created_at,updated_at)
		VALUES (1,1,1,1,?,?,?)`, at, at, at); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrAlreadyMember) {
		t.Errorf("delayed join=%v want already member", err)
	}
	var status string
	var reviewer sql.NullInt64
	if err := db.QueryRow(`SELECT status,reviewed_by FROM ministry_group_requests WHERE id=?`, id).Scan(&status, &reviewer); err != nil {
		t.Fatal(err)
	}
	if status != "approved" || reviewer.Int64 != 2 {
		t.Fatalf("approval overwritten: %s %+v", status, reviewer)
	}
}
