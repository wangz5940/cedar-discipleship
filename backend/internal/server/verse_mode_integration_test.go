//go:build integration

package server

import (
	"agp/backend/internal/audit"
	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminVerseCadenceThroughWebAndBot(t *testing.T) {
	for _, mode := range []string{"weekly", "daily"} {
		t.Run(mode, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'verse','Verse',NOW(),NOW());
    INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'member','Member','member',NOW(),NOW());
    INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(1,1,'Member',NOW(),NOW(),NOW())`)
			a := &app{db: db, location: time.UTC, learning: learning.NewService(learning.NewMySQLRepository(db)), checkins: checkin.NewService(checkin.NewMySQLRepository(db)), users: user.NewService(user.NewMySQLRepository(db)), audits: audit.NewService(audit.NewMySQLRepository(db))}
			request := func(body string) *http.Request {
				r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
				r.SetPathValue("code", "verse")
				return r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
			}
			saved := httptest.NewRecorder()
			a.handleAdminCreateStudyWeek(saved, request(fmt.Sprintf(`{"start_date":"2026-09-21","end_date":"2026-09-27","verse_enabled":true,"verse_mode":%q,"verse_ref":"罗马书 8:1","recite_text":"如今那些在基督耶稣里的"}`, mode)))
			if saved.Code != http.StatusOK {
				t.Fatalf("save=%d %s", saved.Code, saved.Body)
			}
			weeks, err := a.learning.ListWeeks(t.Context(), 1)
			if err != nil || len(weeks) != 1 || weeks[0].VerseMode != mode {
				t.Fatalf("reload=%+v err=%v", weeks, err)
			}
			tasks, err := a.learning.WeekTasks(t.Context(), 1, weeks[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			taskID := tasks[0]["id"].(uint64)
			kind := tasks[0]["task_type"].(string)
			target, err := a.reciteTarget(request(""), 1, taskID)
			if err != nil || target.VerseRef != "罗马书 8:1" {
				t.Fatalf("recite target=%+v err=%v", target, err)
			}
			for _, step := range []struct {
				date, transport string
				want            int
			}{
				{"2026-09-22", "web", http.StatusCreated},
				{"2026-09-22", "bot", http.StatusOK},
				{"2026-09-23", "web", 0},
				{"2026-09-23", "bot", http.StatusOK},
			} {
				want := step.want
				if want == 0 {
					want = http.StatusOK
					if mode == "daily" {
						want = http.StatusCreated
					}
				}
				response := httptest.NewRecorder()
				if step.transport == "web" {
					a.handleCreateCheckin(response, request(fmt.Sprintf(`{"task_type":%q,"task_id":%d,"week_id":%d,"logical_date":%q}`, kind, taskID, weeks[0].ID, step.date)))
				} else {
					a.handleBotCreateCheckin(response, request(fmt.Sprintf(`{"name":"Member","type":%q,"logicalDate":%q}`, kind, step.date)))
				}
				if response.Code != want {
					t.Fatalf("%s %s=%d want=%d body=%s", step.transport, step.date, response.Code, want, response.Body)
				}
			}
			bad := httptest.NewRecorder()
			a.handleAdminCreateStudyWeek(bad, request(`{"start_date":"2026-09-21","end_date":"2026-09-27","verse_mode":"hourly"}`))
			if bad.Code != http.StatusBadRequest {
				t.Fatalf("invalid cadence=%d %s", bad.Code, bad.Body)
			}
		})
	}
}
