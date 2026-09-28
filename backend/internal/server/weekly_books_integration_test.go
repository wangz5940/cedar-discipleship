//go:build integration

package server

import (
	"context"
	"encoding/json"
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

func TestWebAndBotMultipleBooksOnSameDay(t *testing.T) {
	for _, transport := range []string{"web", "bot"} {
		t.Run(transport, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES (1,'a','A',NOW(),NOW());
				INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
				VALUES (1,'member','Member','member',NOW(),NOW());
				INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
				VALUES (1,1,'Member',NOW(),NOW(),NOW());
				INSERT INTO study_weeks(id,group_id,start_date,end_date,book_enabled,created_at,updated_at)
				VALUES (1,1,'2026-09-21','2026-09-27',1,NOW(),NOW())`)
			titles := []string{"第一本读物", strings.Repeat("长", 70)}
			for i, title := range titles {
				testdb.Exec(t, db, `INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
					VALUES (?,1,1,'weekly_book',?,NOW(),NOW())`, i+1, title)
			}
			testdb.Exec(t, db, `INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
				VALUES (3,1,1,'weekly_checkin','旧整周任务',NOW(),NOW())`)
			checkins := checkin.NewService(checkin.NewMySQLRepository(db))
			a := &app{
				db: db, location: time.UTC,
				users: user.NewService(user.NewMySQLRepository(db)), checkins: checkins,
				learning: learning.NewService(learning.NewMySQLRepository(db)),
				audits:   audit.NewService(audit.NewMySQLRepository(db)),
			}
			for attempt := 0; attempt < 2; attempt++ {
				for i, title := range titles {
					payload := map[string]any{"task_type": "weekly_book", "task_id": i + 1, "week_id": 1, "logical_date": "2026-09-23", "detail": title}
					if transport == "bot" {
						payload = map[string]any{"type": "weekly_book", "name": "Member", "logicalDate": "2026-09-23", "detail": title}
					}
					body, err := json.Marshal(payload)
					if err != nil {
						t.Fatal(err)
					}
					req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
					req.SetPathValue("code", "a")
					req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
					response := httptest.NewRecorder()
					if transport == "bot" {
						a.handleBotCreateCheckin(response, req)
					} else {
						a.handleCreateCheckin(response, req)
					}
					want := http.StatusCreated
					if attempt > 0 {
						want = http.StatusOK
					}
					if response.Code != want {
						t.Fatalf("book=%d attempt=%d status=%d body=%s", i, attempt, response.Code, response.Body)
					}
				}
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records`).Scan(&count); err != nil || count != 2 {
				t.Fatalf("records=%d err=%v", count, err)
			}
			for i, title := range titles {
				var part string
				if err := db.QueryRow(`SELECT part FROM checkin_records WHERE task_id=?`, i+1).Scan(&part); err != nil {
					t.Fatal(err)
				}
				runes := []rune(title)
				if len(runes) > 64 {
					runes = runes[:64]
				}
				if part != string(runes) {
					t.Errorf("book=%d part=%q", i, part)
				}
			}
		})
	}
}
