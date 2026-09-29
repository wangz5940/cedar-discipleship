//go:build integration

package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/audit"
	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func TestWebAndBotDailyAdmission(t *testing.T) {
	tests := []struct {
		name     string
		daily    string
		taskType string
		want     int
	}{
		{"legacy defaults", `{}`, "daily_devotion", http.StatusCreated},
		{"all disabled", `{"devotion":{"enabled":false},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusBadRequest},
		{"missing custom date", `{"checkin_mode":"separate","devotion":{"enabled":true,"plan_mode":"custom","plans":[{"date":"2026-09-22","title":"Plan"}]},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusBadRequest},
		{"custom date present", `{"checkin_mode":"separate","devotion":{"enabled":true,"plan_mode":"custom","plans":[{"date":"2026-09-23","title":"Plan"}]},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusCreated},
		{"combined scripture remains available", `{"checkin_mode":"combined","devotion":{"enabled":true,"plan_mode":"custom","plans":[]},"scripture":{"enabled":true}}`, "daily_devotion", http.StatusCreated},
		{"separate scripture", `{"checkin_mode":"separate","devotion":{"enabled":false},"scripture":{"enabled":true,"type":"checkin"}}`, "daily_scripture", http.StatusCreated},
		{"combined rejects separate scripture", `{}`, "daily_scripture", http.StatusBadRequest},
		{"historical combined devotion", `{"devotion":{"numbered_start_date":"2026-09-24","schedule_history":[{"numbered_start_date":"2026-05-27"}]},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusCreated},
		{"historical separate devotion", `{"checkin_mode":"separate","devotion":{"numbered_start_date":"2026-09-24","schedule_history":[{"numbered_start_date":"2026-05-27"}]},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusCreated},
		{"historical scripture", `{"checkin_mode":"separate","devotion":{"enabled":false},"scripture":{"start_date":"2026-09-24","schedule_history":[{"start_date":"2026-05-27"}]}}`, "daily_scripture", http.StatusCreated},
		{"before earliest schedule", `{"devotion":{"numbered_start_date":"2026-09-25","schedule_history":[{"numbered_start_date":"2026-09-24"}]},"scripture":{"enabled":false}}`, "daily_devotion", http.StatusBadRequest},
	}
	for _, tt := range tests {
		for _, transport := range []string{"web", "bot"} {
			t.Run(tt.name+"/"+transport, func(t *testing.T) {
				db := testdb.Open(t)
				testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
					VALUES (1,'a','A',NOW(),NOW());
					INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
					VALUES (1,'member','Member','member',NOW(),NOW());
					INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
					VALUES (1,1,'Member',NOW(),NOW(),NOW())`)
				testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at)
					VALUES (1,?,NOW(),NOW())`, `{"task_sections":{"daily":`+tt.daily+`}}`)
				checkins := checkin.NewService(checkin.NewMySQLRepository(db))
				a := &app{
					location: time.UTC, db: db,
					users:    user.NewService(user.NewMySQLRepository(db)),
					learning: learning.NewService(learning.NewMySQLRepository(db)),
					checkins: checkins, audits: audit.NewService(audit.NewMySQLRepository(db)),
				}
				for attempt := 0; attempt < 2; attempt++ {
					body := fmt.Sprintf(`{"task_type":%q,"logical_date":"2026-09-23","part":"ignore","task_id":99,"week_id":99}`, tt.taskType)
					if transport == "bot" {
						body = fmt.Sprintf(`{"name":"Member","type":%q,"logicalDate":"2026-09-23"}`, tt.taskType)
					}
					req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
					req.SetPathValue("code", "a")
					req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
					response := httptest.NewRecorder()
					if transport == "bot" {
						a.handleBotCreateCheckin(response, req)
					} else {
						a.handleCreateCheckin(response, req)
					}
					want := tt.want
					if attempt == 1 && want == http.StatusCreated {
						want = http.StatusOK
					}
					if response.Code != want {
						t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body)
					}
				}
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records`).Scan(&count); err != nil {
					t.Fatal(err)
				}
				wantCount := 0
				if tt.want == http.StatusCreated {
					wantCount = 1
				}
				if count != wantCount {
					t.Fatalf("persisted %d records, want %d", count, wantCount)
				}
				if count == 1 {
					var part string
					var taskID, weekID uint64
					if err := db.QueryRow(`SELECT part,COALESCE(task_id,0),COALESCE(week_id,0) FROM checkin_records`).Scan(&part, &taskID, &weekID); err != nil {
						t.Fatal(err)
					}
					if part != "" || taskID != 0 || weekID != 0 {
						t.Fatalf("daily target not normalized: part=%q task=%d week=%d", part, taskID, weekID)
					}
				}
			})
		}
	}
}

func TestCheckinDeleteHandlersOnlyAuditDeletedRecords(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'first','First','first',NOW(),NOW()),(2,'second','Second','second',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'First',NOW(),NOW(),NOW()),(1,2,'Second',NOW(),NOW(),NOW());
		INSERT INTO checkin_records
		  (id,group_id,user_id,logical_date,checkin_time,task_type,created_by,created_at,updated_at)
		VALUES
		  (1,1,1,'2026-09-21',NOW(),'daily_devotion',1,NOW(),NOW()),
		  (2,1,2,'2026-09-22',NOW(),'daily_devotion',2,NOW(),NOW()),
		  (3,2,2,'2026-09-23',NOW(),'daily_devotion',2,NOW(),NOW()),
		  (4,1,2,'2026-09-24',NOW(),'daily_devotion',2,NOW(),NOW())`)
	a := &app{
		db:       db,
		users:    user.NewService(user.NewMySQLRepository(db)),
		checkins: checkin.NewService(checkin.NewMySQLRepository(db)),
		audits:   audit.NewService(audit.NewMySQLRepository(db)),
	}
	webRequest := func(id string) *http.Request {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		req.SetPathValue("id", id)
		return req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
	}
	botRequest := func(id string) *http.Request {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		req.SetPathValue("code", "a")
		req.SetPathValue("id", id)
		return req
	}

	notFound := []struct {
		name    string
		handler http.HandlerFunc
		request *http.Request
	}{
		{name: "own missing", handler: a.handleDeleteOwnCheckin, request: webRequest("99")},
		{name: "own other user", handler: a.handleDeleteOwnCheckin, request: webRequest("2")},
		{name: "own other group", handler: a.handleDeleteOwnCheckin, request: webRequest("3")},
		{name: "admin missing", handler: a.handleAdminDeleteCheckin, request: webRequest("99")},
		{name: "admin other group", handler: a.handleAdminDeleteCheckin, request: webRequest("3")},
		{name: "bot missing", handler: a.handleBotDeleteCheckin, request: botRequest("99")},
		{name: "bot other group", handler: a.handleBotDeleteCheckin, request: botRequest("3")},
	}
	for _, tt := range notFound {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			tt.handler(response, tt.request)
			if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "checkin_not_found") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
	var auditCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&auditCount); err != nil || auditCount != 0 {
		t.Fatalf("audit count after misses = %d, err=%v", auditCount, err)
	}

	successes := []struct {
		name    string
		handler http.HandlerFunc
		request *http.Request
	}{
		{name: "own", handler: a.handleDeleteOwnCheckin, request: webRequest("1")},
		{name: "admin", handler: a.handleAdminDeleteCheckin, request: webRequest("2")},
		{name: "bot", handler: a.handleBotDeleteCheckin, request: botRequest("4")},
	}
	for _, tt := range successes {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			tt.handler(response, tt.request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&auditCount); err != nil || auditCount != len(successes) {
		t.Fatalf("audit count after deletes = %d, err=%v", auditCount, err)
	}
}
