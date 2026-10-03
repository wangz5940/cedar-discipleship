//go:build integration

package learning

import (
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestOVCMWeekBindingPersistsInMySQL(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'ovcm-test','课程测试',NOW(),NOW())`)
	service := NewService(NewMySQLRepository(db))
	const url = "https://ovcm.net/tx2026/#/course/ds10tg/01"
	id, err := service.SaveWeek(t.Context(), 1, 0, WeekInput{
		StartDate: "2026-10-05", EndDate: "2026-10-11", VideoEnabled: true,
		Videos: []TaskBinding{{Title: "01.起初的话1", URL: url}},
	}, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	weeks, err := service.ListWeeks(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(weeks) != 1 || weeks[0].ID != id || len(weeks[0].Videos) != 1 || weeks[0].Videos[0].URL != url {
		t.Fatalf("reloaded weeks = %+v, want saved OVCM lesson", weeks)
	}
	otherGroup, err := service.ListWeeks(t.Context(), 2)
	if err != nil || len(otherGroup) != 0 {
		t.Fatalf("other group's weeks = %+v, err=%v", otherGroup, err)
	}
}
