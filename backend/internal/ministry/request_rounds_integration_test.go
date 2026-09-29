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
	testdb.Apply(t, db, "005_ministry_catalog_and_pins.sql")
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
		requests, err := repo.ListPendingRequests(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(requests) != 1 || requests[0].SubmissionRound != uint64(round) {
			t.Fatalf("pending request round = %+v, want %d", requests, round)
		}
		assertCount("join_request", round)
		if err := repo.DecideRequest(t.Context(), 1, id, 2, uint64(round), StatusRejected, at); err != nil {
			t.Fatal(err)
		}
		if err := repo.DecideRequest(t.Context(), 1, id, 2, uint64(round), StatusRejected, at); !errors.Is(err, ErrRequestAlreadyReviewed) {
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
		shares, err := repo.ListShares(t.Context(), 1, 1, 1, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(shares) != 1 || shares[0].SubmissionRound != uint64(round) {
			t.Fatalf("pending share round = %+v, want %d", shares, round)
		}
		if err := repo.DecideShare(t.Context(), 1, 1, share, 2, uint64(round), StatusPublished, at); err != nil {
			t.Fatal(err)
		}
		if err := repo.DecideShare(t.Context(), 1, 1, share, 2, uint64(round), StatusPublished, at); !errors.Is(err, ErrShareAlreadyReviewed) {
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

func TestStaleDecisionCannotReviewNewSubmissionRound(t *testing.T) {
	repo, db := requestRoundFixture(t)
	at := time.Now()

	requestID, err := repo.RequestJoin(t.Context(), 1, 1, 1, "round one", false, at)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DecideRequest(t.Context(), 1, requestID, 2, 1, StatusRejected, at); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RequestJoin(t.Context(), 1, 1, 1, "round two", false, at); err != nil {
		t.Fatal(err)
	}
	request, err := repo.Request(t.Context(), 1, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if request.SubmissionRound != 2 {
		t.Fatalf("request round = %d, want 2", request.SubmissionRound)
	}
	if err := repo.DecideRequest(t.Context(), 1, requestID, 2, 1, StatusApproved, at); !errors.Is(err, ErrSubmissionRoundConflict) {
		t.Fatalf("stale request decision error = %v, want %v", err, ErrSubmissionRoundConflict)
	}
	var requestStatus string
	var requestRound, memberships, decisions int
	if err := db.QueryRow(`SELECT status,submission_round FROM ministry_group_requests WHERE id=?`, requestID).
		Scan(&requestStatus, &requestRound); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_group_members WHERE ministry_group_id=1 AND user_id=1 AND status=1`).
		Scan(&memberships); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_notifications WHERE notification_type='join_decision'`).
		Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if requestStatus != "pending" || requestRound != 2 || memberships != 0 || decisions != 1 {
		t.Fatalf(
			"stale request changed state: status=%s round=%d memberships=%d decisions=%d",
			requestStatus,
			requestRound,
			memberships,
			decisions,
		)
	}
	if err := repo.DecideRequest(t.Context(), 1, requestID, 2, 2, StatusApproved, at); err != nil {
		t.Fatalf("current request decision failed: %v", err)
	}

	shareID, err := repo.CreateShare(
		t.Context(),
		1,
		1,
		1,
		ShareInput{Title: "Round one", Body: "Body"},
		StatusPending,
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateShare(
		t.Context(),
		1,
		1,
		shareID,
		1,
		ShareInput{Title: "Round two", Body: "Changed"},
		StatusPending,
		false,
		at,
	); err != nil {
		t.Fatal(err)
	}
	if err := repo.DecideShare(t.Context(), 1, 1, shareID, 2, 1, StatusPublished, at); !errors.Is(err, ErrSubmissionRoundConflict) {
		t.Fatalf("stale share decision error = %v, want %v", err, ErrSubmissionRoundConflict)
	}
	var shareTitle, shareStatus string
	var shareRound, shareDecisions int
	if err := db.QueryRow(`SELECT title,status,submission_round FROM ministry_shares WHERE id=?`, shareID).
		Scan(&shareTitle, &shareStatus, &shareRound); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_notifications WHERE notification_type='share_decision'`).
		Scan(&shareDecisions); err != nil {
		t.Fatal(err)
	}
	if shareTitle != "Round two" || shareStatus != "pending" || shareRound != 2 || shareDecisions != 0 {
		t.Fatalf(
			"stale share changed state: title=%q status=%s round=%d decisions=%d",
			shareTitle,
			shareStatus,
			shareRound,
			shareDecisions,
		)
	}
	if err := repo.DecideShare(t.Context(), 1, 1, shareID, 2, 2, StatusPublished, at); err != nil {
		t.Fatalf("current share decision failed: %v", err)
	}
}

func TestRequestApprovalRequiresActiveStudyGroupMember(t *testing.T) {
	tests := []struct {
		name       string
		deactivate string
	}{
		{
			name:       "inactive group membership",
			deactivate: `UPDATE group_members SET status=0 WHERE group_id=1 AND user_id=1`,
		},
		{
			name:       "inactive user",
			deactivate: `UPDATE users SET status=0 WHERE id=1`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, db := requestRoundFixture(t)
			at := time.Now()
			requestID, err := repo.RequestJoin(t.Context(), 1, 1, 1, "join", false, at)
			if err != nil {
				t.Fatal(err)
			}
			testdb.Exec(t, db, test.deactivate)

			err = repo.DecideRequest(t.Context(), 1, requestID, 2, 1, StatusApproved, at)
			if !errors.Is(err, ErrRequestApplicantNotMember) {
				t.Fatalf("approval error = %v, want %v", err, ErrRequestApplicantNotMember)
			}
			var status string
			var memberships, decisions int
			if err := db.QueryRow(`SELECT status FROM ministry_group_requests WHERE id=?`, requestID).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_group_members WHERE ministry_group_id=1 AND user_id=1 AND status=1`).
				Scan(&memberships); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM ministry_notifications WHERE notification_type='join_decision'`).
				Scan(&decisions); err != nil {
				t.Fatal(err)
			}
			if status != "pending" || memberships != 0 || decisions != 0 {
				t.Fatalf("invalid approval changed state: status=%s memberships=%d decisions=%d", status, memberships, decisions)
			}
			if err := repo.DecideRequest(t.Context(), 1, requestID, 2, 1, StatusRejected, at); err != nil {
				t.Fatalf("rejection should remain allowed: %v", err)
			}
		})
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
