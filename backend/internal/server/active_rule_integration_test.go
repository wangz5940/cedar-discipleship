//go:build integration

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"agp/backend/internal/audit"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestActiveRulePreservesConcurrentLearningConfig(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES (1,'{"task_sections":{"daily":{"devotion":{"title":"old"}}}}',NOW(),NOW())`)
	a := &app{
		learning: learning.NewService(learning.NewMySQLRepository(db)),
		audits:   audit.NewService(audit.NewMySQLRepository(db)),
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), `UPDATE group_settings
		SET settings='{"task_sections":{"daily":{"devotion":{"title":"new"}}},"other_setting":true}' WHERE group_id=1`); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		ctx = context.WithValue(ctx, currentUserKey, currentUser{ID: 1, CurrentGroupID: 1})
		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"mode":"all","task_types":["daily_devotion"]}`)).WithContext(ctx)
		response := httptest.NewRecorder()
		a.handleDashboardActiveRule(response, req)
		done <- response
	}()
	testdb.WaitForLockWait(t, db)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	response := <-done
	if response.Code != http.StatusOK {
		t.Fatalf("update active rule: %d %s", response.Code, response.Body)
	}
	var title, mode, other string
	if err := db.QueryRow(`SELECT settings->>'$.task_sections.daily.devotion.title',
		settings->>'$.active_member_rule.mode',COALESCE(settings->>'$.other_setting','missing')
		FROM group_settings WHERE group_id=1`).Scan(&title, &mode, &other); err != nil {
		t.Fatal(err)
	}
	if title != "new" || mode != "all" || other != "true" {
		t.Fatalf("settings overwritten: title=%s mode=%s other=%s", title, mode, other)
	}
	for _, groupID := range []uint64{2, 3} {
		if groupID == 3 {
			testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES (3,'null',NOW(),NOW())`)
		}
		if err := a.learning.SaveActiveMemberRule(t.Context(), groupID, map[string]any{"mode": "any", "task_types": []string{"daily_devotion"}}); err != nil {
			t.Fatal(err)
		}
		settings, err := a.learning.LearningConfig(t.Context(), groupID)
		if err != nil {
			t.Fatal(err)
		}
		if rule := activeMemberRuleFromSettings(settings); rule.Mode != "any" || len(rule.TaskTypes) != 1 {
			t.Fatalf("initial settings group=%d rule=%+v", groupID, rule)
		}
	}
}

func TestLearningConfigPreservesConcurrentActiveRule(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES (1,'{"task_sections":{"daily":{"devotion":{"title":"old","numbered_start_date":"2026-01-01"}}},"active_member_rule":{"mode":"any","task_types":["daily_devotion"]},"obsolete_setting":true}',NOW(),NOW())`)
	service := learning.NewService(learning.NewMySQLRepository(db))

	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), `UPDATE group_settings
		SET settings=JSON_SET(settings,'$.active_member_rule',
			JSON_OBJECT('mode','all','task_types',JSON_ARRAY('weekly_book')))
		WHERE group_id=1`); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		done <- service.SaveLearningConfig(ctx, 1, map[string]any{
			"task_sections": map[string]any{
				"daily": map[string]any{
					"devotion": map[string]any{
						"title":               "new",
						"numbered_start_date": "2026-02-01",
					},
				},
			},
			"active_member_rule": map[string]any{
				"mode":       "any",
				"task_types": []string{"daily_scripture"},
			},
			"other_setting": true,
		})
	}()
	testdb.WaitForLockWait(t, db)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	var title, previousStart, mode, taskType, other, obsolete string
	if err := db.QueryRow(`SELECT settings->>'$.task_sections.daily.devotion.title',
		settings->>'$.task_sections.daily.devotion.schedule_history[0].numbered_start_date',
		COALESCE(settings->>'$.active_member_rule.mode','missing'),
		COALESCE(settings->>'$.active_member_rule.task_types[0]','missing'),
		settings->>'$.other_setting',
		COALESCE(settings->>'$.obsolete_setting','missing')
		FROM group_settings WHERE group_id=1`).Scan(&title, &previousStart, &mode, &taskType, &other, &obsolete); err != nil {
		t.Fatal(err)
	}
	if title != "new" || previousStart != "2026-01-01" ||
		mode != "all" || taskType != "weekly_book" || other != "true" || obsolete != "missing" {
		t.Fatalf(
			"settings overwritten: title=%s history=%s mode=%s task=%s other=%s obsolete=%s",
			title,
			previousStart,
			mode,
			taskType,
			other,
			obsolete,
		)
	}
}

func TestConcurrentLearningConfigCreationPreservesActiveRule(t *testing.T) {
	db := testdb.Open(t)
	service := learning.NewService(learning.NewMySQLRepository(db))

	for round := range 10 {
		groupID := uint64(round + 10)
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wait sync.WaitGroup
		wait.Go(func() {
			<-start
			errs <- service.SaveLearningConfig(t.Context(), groupID, map[string]any{
				"task_sections": map[string]any{"daily": map[string]any{"checkin_mode": "task"}},
				"active_member_rule": map[string]any{
					"mode":       "any",
					"task_types": []string{"stale"},
				},
			})
		})
		wait.Go(func() {
			<-start
			errs <- service.SaveActiveMemberRule(t.Context(), groupID, map[string]any{
				"mode":       "all",
				"task_types": []string{"weekly_book"},
			})
		})
		close(start)
		wait.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: concurrent creation failed: %v", round, err)
			}
		}

		var checkinMode, mode, taskType string
		if err := db.QueryRow(`SELECT settings->>'$.task_sections.daily.checkin_mode',
			settings->>'$.active_member_rule.mode',
			settings->>'$.active_member_rule.task_types[0]'
			FROM group_settings WHERE group_id=?`, groupID).Scan(&checkinMode, &mode, &taskType); err != nil {
			t.Fatal(err)
		}
		if checkinMode != "task" || mode != "all" || taskType != "weekly_book" {
			t.Fatalf(
				"round %d: settings overwritten: checkin=%s mode=%s task=%s",
				round,
				checkinMode,
				mode,
				taskType,
			)
		}
	}
}
