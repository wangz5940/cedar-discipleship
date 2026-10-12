//go:build integration

package learning

import (
	"errors"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestDuplicateWeekPreservesExistingPlan(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES (1,'first','first',NOW(),NOW()),(2,'second','second',NOW(),NOW())`)
	service := NewService(NewMySQLRepository(db))
	input := WeekInput{StartDate: "2026-10-12", EndDate: "2026-10-18", BookEnabled: true, Readings: []TaskBinding{{Title: "original", URL: "https://example.com/book.pdf"}}}
	id, err := service.SaveWeek(t.Context(), 1, 0, input, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input.Readings[0].Title = "replacement"
	_, err = service.SaveWeek(t.Context(), 1, 0, input, true, time.Now())
	if !errors.Is(err, ErrWeekAlreadyExists) {
		t.Fatalf("duplicate save error = %v, want week_already_exists", err)
	}
	weeks, err := service.ListWeeks(t.Context(), 1)
	if err != nil || len(weeks) != 1 || weeks[0].ID != id || weeks[0].Readings[0].Title != "original" {
		t.Fatalf("existing plan changed: %+v, %v", weeks, err)
	}
	if _, err := service.SaveWeek(t.Context(), 2, 0, input, false, time.Now()); err != nil {
		t.Fatalf("other group: %v", err)
	}
	input.StartDate = "2026-10-19"
	input.EndDate = "2026-10-25"
	other, err := service.SaveWeek(t.Context(), 1, 0, input, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	input.StartDate = "2026-10-12"
	input.EndDate = "2026-10-18"
	_, err = service.SaveWeek(t.Context(), 1, other, input, false, time.Now())
	if !errors.Is(err, ErrWeekAlreadyExists) {
		t.Fatalf("update collision = %v", err)
	}
}
