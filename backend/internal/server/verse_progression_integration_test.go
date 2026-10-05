//go:build integration

package server

import (
	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"database/sql"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestProgressingVerseCheckinAndReciteTargetAgree(t *testing.T) {
	a, db := passwordSessionFixture(t)
	a.learning = learning.NewService(learning.NewMySQLRepository(db))
	a.checkins = checkin.NewService(checkin.NewMySQLRepository(db))
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES(1,?,NOW(),NOW())`,
		`{"task_sections":{"daily":{"verse":{"enabled":true,"plans":[{"date":"2026-10-05","end_date":"2026-10-11","completion_mode":"daily","verses_per_day":2,"verse_ref":"创1:1-3","recite_text":"创1:1 第一节。\n创1:2 第二节。\n创1:3 第三节。"}]}}}}`)
	r := httptest.NewRequest("GET", "/", nil)
	target, err := a.dailyReciteTarget(r, 1, "2026-10-06")
	if err != nil || target.VerseRef != "创1:3" {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	record := &checkin.Record{GroupID: 1, UserID: 1, LogicalDate: "2026-10-06", TaskType: "daily_verse", Detail: "untrusted whole range"}
	if _, _, err := a.createAdmittedCheckin(t.Context(), record, 1); err != nil {
		t.Fatal(err)
	}
	if record.Detail != target.VerseRef {
		t.Fatalf("checkin=%s target=%s", record.Detail, target.VerseRef)
	}
	if _, err := a.dailyReciteTarget(r, 1, "2026-10-07"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("exhausted target=%v", err)
	}
	record.LogicalDate = "2026-10-07"
	if _, _, err := a.createAdmittedCheckin(t.Context(), record, 1); !errors.Is(err, errDailyTaskDisabled) {
		t.Fatalf("exhausted checkin=%v", err)
	}
}
