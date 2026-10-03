//go:build integration

package server

import (
	"agp/backend/internal/audit"
	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDailyVerseWebBotAndRecitation(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'verse','Verse',NOW(),NOW()),(2,'other','Other',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'member','Member','member',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(1,1,'Member',NOW(),NOW(),NOW())`)
	a := &app{db: db, location: time.UTC, learning: learning.NewService(learning.NewMySQLRepository(db)),
		checkins: checkin.NewService(checkin.NewMySQLRepository(db)), users: user.NewService(user.NewMySQLRepository(db)),
		audits: audit.NewService(audit.NewMySQLRepository(db))}
	call := func(handler http.HandlerFunc, path, body string, groupID uint64, status int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.SetPathValue("code", "verse")
		req.SetPathValue("id", strings.TrimPrefix(path, "/checkins/"))
		req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: groupID}))
		res := httptest.NewRecorder()
		handler(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: status %d want %d: %s", path, body, res.Code, status, res.Body)
		}
		var value map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	config := `{"task_sections":{"daily":{"devotion":{"enabled":false},"scripture":{"enabled":false},"verse":{"enabled":true,"plans":[
		{"date":"2026-09-22","verse_ref":"约翰福音 3:16","recite_text":"神爱世人"},
		{"date":"2026-09-23","verse_ref":"约翰福音 3:16","recite_text":"甚至将他的独生子赐给他们"}
	]}}}}`
	call(a.handleAdminSaveLearningConfig, "/", config, 1, http.StatusOK)
	call(a.handleAdminSaveLearningConfig, "/", strings.ReplaceAll(config, "2026-09-23", "2026-09-22"), 1, http.StatusBadRequest)
	call(a.handleAdminSaveLearningConfig, "/", strings.ReplaceAll(config, "2026-09-23", "2026-02-30"), 1, http.StatusBadRequest)
	// Daily tasks work without a week, and supplied weekly IDs never become their identity.
	body := `{"task_type":"daily_verse","logical_date":"2026-09-22","task_id":999,"week_id":999,"part":"wrong","detail":"wrong"}`
	first := call(a.handleCreateCheckin, "/", body, 1, http.StatusCreated)
	same := call(a.handleCreateCheckin, "/", body, 1, http.StatusOK)
	if first["id"] != same["id"] {
		t.Fatal("duplicate daily completion")
	}
	call(a.handleBotCreateCheckin, "/", `{"name":"Member","type":"每日背经","logicalDate":"2026-09-23"}`, 1, http.StatusCreated)
	call(a.handleBotCreateCheckin, "/", `{"name":"Member","type":"每日背经","logicalDate":"2026-09-23"}`, 1, http.StatusOK)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records WHERE task_type='daily_verse'
		AND week_id IS NULL AND task_id IS NULL AND part='' AND detail='约翰福音 3:16'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("daily identity count=%d err=%v", count, err)
	}
	weekID, err := a.learning.SaveWeek(t.Context(), 1, 0, learning.WeekInput{
		StartDate: "2026-09-21", EndDate: "2026-09-27", VerseEnabled: true,
		VerseMode: "daily", VerseRef: "约翰福音 3:16", ReciteText: "周原文",
	}, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := a.learning.WeekTasks(t.Context(), 1, weekID)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("weekly tasks=%v err=%v", tasks, err)
	}
	taskID := tasks[0]["id"].(uint64)
	call(a.handleCreateCheckin, "/", fmt.Sprintf(`{"task_type":"weekly_verse","logical_date":"2026-09-22","task_id":%d,"week_id":%d}`, taskID, weekID), 1, http.StatusCreated)
	for _, date := range []string{"2026-09-22", "2026-09-23"} {
		settings, err := a.groupLearningConfig(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		hub, err := a.learning.TodayHub(t.Context(), 1, 1, date, settings, time.Now())
		if err != nil || len(hub.Tasks) != 2 || hub.Progress.Completed != 2 {
			t.Fatalf("date=%s hub=%+v err=%v", date, hub, err)
		}
	}
	call(a.handleDeleteOwnCheckin, fmt.Sprintf("/checkins/%.0f", first["id"]), "", 1, http.StatusOK)
	call(a.handleCreateCheckin, "/", body, 1, http.StatusCreated)
	call(a.handleCreateCheckin, "/", strings.ReplaceAll(body, "2026-09-22", "2026-09-24"), 1, http.StatusBadRequest)
	call(a.handleCreateCheckin, "/", body, 2, http.StatusBadRequest)

	// Identical verse references on different dates and in the week have independent histories.
	for _, date := range []string{"2026-09-22", "2026-09-23"} {
		for attempt := 1; attempt <= 2; attempt++ {
			got := call(a.handleCreateReciteAttempt, "/", fmt.Sprintf(`{"task_type":"daily_verse","logical_date":%q,"blank_percent":50,"blank_count":4,"correct_count":2}`, date), 1, http.StatusCreated)
			if got["attempt_no"] != float64(attempt) {
				t.Fatalf("date %s attempt=%v", date, got)
			}
		}
	}
	weekly := call(a.handleCreateReciteAttempt, "/", fmt.Sprintf(`{"task_id":%d,"blank_percent":50,"blank_count":4,"correct_count":4}`, taskID), 1, http.StatusCreated)
	if weekly["attempt_no"] != float64(1) {
		t.Fatal("daily attempts leaked into weekly attempts")
	}
	for _, query := range []string{"task_type=daily_verse&logical_date=2026-09-22", "task_type=daily_verse&logical_date=2026-09-23", fmt.Sprintf("task_id=%d", taskID)} {
		want := 2
		if strings.HasPrefix(query, "task_id=") {
			want = 1
		}
		got := call(a.handleListReciteAttempts, "/?"+query, "", 1, http.StatusOK)
		if len(got["attempts"].([]any)) != want {
			t.Fatalf("history %s: %v", query, got)
		}
		ranking := call(a.handleReciteLeaderboard, "/?"+query, "", 1, http.StatusOK)["leaderboard"].([]any)
		if len(ranking) != 1 || ranking[0].(map[string]any)["attempts"] != float64(want) {
			t.Fatalf("ranking %s: %v", query, ranking)
		}
	}
	call(a.handleListReciteAttempts, "/?task_type=daily_verse&logical_date=2026-09-22", "", 2, http.StatusNotFound)
	call(a.handleListReciteAttempts, "/?task_type=daily_verse&logical_date=2026-09-22&user_id=2", "", 1, http.StatusForbidden)
	call(a.handleCreateReciteAttempt, "/", `{"task_type":"daily_verse","logical_date":"2026-09-24","blank_percent":50,"blank_count":4,"correct_count":2}`, 1, http.StatusNotFound)
	call(a.handleAdminSaveLearningConfig, "/", strings.Replace(config, `"verse":{"enabled":true`, `"verse":{"enabled":false`, 1), 1, http.StatusOK)
	call(a.handleCreateCheckin, "/", body, 1, http.StatusBadRequest)
	call(a.handleCreateReciteAttempt, "/", `{"task_type":"daily_verse","logical_date":"2026-09-22","blank_percent":50,"blank_count":4,"correct_count":2}`, 1, http.StatusNotFound)
}

func TestDailyVerseMigrationPreservesHistoryAndIsIdempotent(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_tasks(id,group_id,week_id,task_type,title,content,sort_order,created_at,updated_at)
		VALUES(9,1,7,'daily_verse','经文','原文',1,NOW(),NOW());
		INSERT INTO checkin_records(id,group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,part,created_by,created_at,updated_at)
		VALUES(1,1,1,9,7,'2026-09-22',NOW(),'daily_verse','verse:7',1,NOW(),NOW()),
		(2,1,1,NULL,NULL,'2026-09-22',NOW(),'daily_verse','',1,NOW(),NOW()),
		(3,1,1,9,7,'2026-09-22',NOW(),'weekly_verse','',1,NOW(),NOW())`)
	testdb.Apply(t, db, "021_daily_verse.sql")
	testdb.Apply(t, db, "021_daily_verse.sql")
	for _, id := range []int{1, 2, 3} {
		var kind string
		if err := db.QueryRow(`SELECT task_type FROM checkin_records WHERE id=?`, id).Scan(&kind); err != nil {
			t.Fatal(err)
		}
		want := "weekly_verse"
		if id == 2 {
			want = "daily_verse"
		}
		if kind != want {
			t.Fatalf("record %d type=%s", id, kind)
		}
	}
	var kind string
	if err := db.QueryRow(`SELECT task_type FROM study_tasks WHERE id=9`).Scan(&kind); err != nil || kind != "weekly_verse" {
		t.Fatalf("task type=%s err=%v", kind, err)
	}
}
