//go:build integration

package user

import (
	"database/sql"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestRemoveMemberClosesOnlyTheirPendingMinistryRequests(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "003_ministry_groups.sql")
	testdb.Apply(t, db, "014_ministry_submission_rounds.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'member','Member','member',NOW(),NOW()),
		       (2,'reviewer','Reviewer','reviewer',NOW(),NOW()),
		       (3,'other','Other','other',NOW(),NOW());
		INSERT INTO group_members(id,group_id,user_id,member_name,status,joined_at,created_at,updated_at)
		VALUES (1,1,1,'Member',1,NOW(),NOW(),NOW()),
		       (2,1,2,'Reviewer',1,NOW(),NOW(),NOW()),
		       (3,1,3,'Other',1,NOW(),NOW(),NOW()),
		       (4,2,1,'Member',1,NOW(),NOW(),NOW());
		INSERT INTO ministry_groups(id,study_group_id,code,name,created_at,updated_at)
		VALUES (1,1,'one','One',NOW(),NOW()),
		       (2,1,'two','Two',NOW(),NOW()),
		       (3,2,'three','Three',NOW(),NOW());
		INSERT INTO ministry_group_requests
			(id,study_group_id,ministry_group_id,user_id,request_type,status,created_at,updated_at)
		VALUES (1,1,1,1,'join','pending',NOW(),NOW()),
		       (2,1,2,1,'join','pending',NOW(),NOW()),
		       (3,1,1,3,'join','pending',NOW(),NOW()),
		       (4,1,2,1,'other','pending',NOW(),NOW()),
		       (5,2,3,1,'join','pending',NOW(),NOW())`)

	at := time.Now()
	if err := NewMySQLRepository(db).RemoveMember(t.Context(), 1, 1, 1, 2, at); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		id             uint64
		wantStatus     string
		wantReviewedBy sql.NullInt64
	}{
		{id: 1, wantStatus: "rejected", wantReviewedBy: sql.NullInt64{Int64: 2, Valid: true}},
		{id: 2, wantStatus: "rejected", wantReviewedBy: sql.NullInt64{Int64: 2, Valid: true}},
		{id: 3, wantStatus: "pending"},
		{id: 4, wantStatus: "pending"},
		{id: 5, wantStatus: "pending"},
	} {
		var status string
		var reviewedBy sql.NullInt64
		if err := db.QueryRow(
			`SELECT status,reviewed_by FROM ministry_group_requests WHERE id=?`,
			test.id,
		).Scan(&status, &reviewedBy); err != nil {
			t.Fatal(err)
		}
		if status != test.wantStatus || reviewedBy != test.wantReviewedBy {
			t.Errorf(
				"request %d = status %q reviewer %+v, want %q %+v",
				test.id,
				status,
				reviewedBy,
				test.wantStatus,
				test.wantReviewedBy,
			)
		}
	}
	var memberStatus int
	if err := db.QueryRow(`SELECT status FROM group_members WHERE id=1`).Scan(&memberStatus); err != nil {
		t.Fatal(err)
	}
	if memberStatus != 0 {
		t.Fatalf("member status = %d, want inactive", memberStatus)
	}
}
