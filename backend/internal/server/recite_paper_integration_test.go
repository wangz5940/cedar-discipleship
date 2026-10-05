//go:build integration

package server

import (
	"agp/backend/internal/audit"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecitePaperSaveReplayAndAccess(t *testing.T) {
	a, db := passwordSessionFixture(t)
	a.location = time.UTC
	a.learning = learning.NewService(learning.NewMySQLRepository(db))
	a.audits = audit.NewService(audit.NewMySQLRepository(db))
	testdb.Exec(t, db, `UPDATE refresh_sessions SET current_group_id=1;
		INSERT INTO study_weeks(id,group_id,start_date,end_date,title,verse_ref,recite_text,created_at,updated_at)
		VALUES(7,1,'2026-09-28','2026-10-04','本周','弗1:16','已修改的配置',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,content,created_at,updated_at)
		VALUES(9,1,7,'weekly_verse','弗1:16','已修改的配置',NOW(),NOW());
		INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES(1,'{"task_sections":{"daily":{"verse":{"enabled":true,"plans":[{"date":"2026-10-04","verse_ref":"弗1:16","recite_text":"已修改的配置"}]}}}}',NOW(),NOW())`)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/recite-attempts/{id}/paper", a.auth(a.handleGetRecitePaper))
	mux.HandleFunc("POST /api/recite-attempts", a.auth(a.handleCreateReciteAttempt))
	mux.HandleFunc("GET /api/recite-attempts", a.auth(a.handleListReciteAttempts))
	mux.HandleFunc("GET /api/recite-leaderboard", a.auth(a.handleReciteLeaderboard))
	call := func(actor, group uint64, method, path, body string, status int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if actor != 0 {
			session := (actor-1)*2 + 1
			testdb.Exec(t, db, `UPDATE refresh_sessions SET current_group_id=? WHERE id=?`, group, session)
			token, err := a.signToken(tokenClaims{UserID: actor, CurrentGroupID: group, SessionID: session})
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s actor=%d group=%d: status=%d want=%d body=%s", method, path, actor, group, res.Code, status, res.Body)
		}
		var value map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	paperJSON := `{"version":1,"text":"【弗1:16】就为你们不住地感谢　神。","blank_indexes":[1],"answers":["就为你们不住的感谢神"]}`
	var expected map[string]any
	if err := json.Unmarshal([]byte(paperJSON), &expected); err != nil {
		t.Fatal(err)
	}
	for _, identity := range []struct{ body, query string }{
		{`"task_id":9`, "task_id=9"},
		{`"task_type":"daily_verse","logical_date":"2026-10-04"`, "task_type=daily_verse&logical_date=2026-10-04"},
	} {
		body := fmt.Sprintf(`{%s,"blank_percent":100,"blank_count":10,"correct_count":9,"paper":%s}`, identity.body, paperJSON)
		saved := call(1, 1, "POST", "/api/recite-attempts", body, http.StatusCreated)
		if saved["score"] != float64(90) || saved["has_paper"] != true {
			t.Fatalf("saved=%v", saved)
		}
		path := fmt.Sprintf("/api/recite-attempts/%.0f/paper", saved["id"])
		for _, actor := range []uint64{1, 4} {
			got := call(actor, 1, "GET", path, "", http.StatusOK)
			if !reflect.DeepEqual(got["paper"], expected) {
				t.Fatalf("snapshot changed: got=%v want=%v", got, expected)
			}
		}
		call(0, 1, "GET", path, "", http.StatusUnauthorized)
		call(2, 1, "GET", path, "", http.StatusForbidden)
		call(3, 1, "GET", path, "", http.StatusForbidden) // A group leader cannot read another member's answers.
		call(2, 2, "GET", path, "", http.StatusNotFound)
		testdb.Exec(t, db, `UPDATE group_members SET status=0 WHERE group_id=1 AND user_id=1`)
		call(4, 1, "GET", path, "", http.StatusNotFound)
		testdb.Exec(t, db, `UPDATE group_members SET status=1 WHERE group_id=1 AND user_id=1`)
		list := call(1, 1, "GET", "/api/recite-attempts?"+identity.query, "", http.StatusOK)
		item := list["attempts"].([]any)[0].(map[string]any)
		if item["has_paper"] != true || item["paper"] != nil {
			t.Fatalf("list must return availability only: %v", item)
		}
		ranking := call(1, 1, "GET", "/api/recite-leaderboard?"+identity.query, "", http.StatusOK)
		if strings.Contains(fmt.Sprint(ranking), "就为你") {
			t.Fatal("answers leaked into ranking")
		}
		// Old clients remain valid; NULL snapshots do not invent historical answers.
		oldBody := fmt.Sprintf(`{%s,"blank_percent":50,"blank_count":2,"correct_count":1}`, identity.body)
		old := call(1, 1, "POST", "/api/recite-attempts", oldBody, http.StatusCreated)
		if old["score"] != float64(25) || old["attempt_no"] != float64(2) {
			t.Fatalf("legacy score or numbering changed: %v", old)
		}
		oldPaper := call(1, 1, "GET", fmt.Sprintf("/api/recite-attempts/%.0f/paper", old["id"]), "", http.StatusOK)
		if oldPaper["paper"] != nil {
			t.Fatal("legacy paper should be null")
		}
		// Replaying the migration preserves both JSON and NULL rows.
		testdb.Apply(t, db, "023_recite_paper.sql")
		testdb.Apply(t, db, "023_recite_paper.sql")
		got := call(1, 1, "GET", path, "", http.StatusOK)
		if !reflect.DeepEqual(got["paper"], expected) {
			t.Fatal("migration changed saved paper")
		}
	}
	call(1, 1, "GET", "/api/recite-attempts/invalid/paper", "", http.StatusBadRequest)
	call(1, 1, "GET", "/api/recite-attempts/999999/paper", "", http.StatusNotFound)
}
