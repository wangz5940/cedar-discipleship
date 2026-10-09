//go:build integration

package server

import (
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLearningRemindersPersistenceAndBoundaries(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "024_learning_reminders.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'reminder','Reminder',NOW(),NOW()),(2,'other','Other',NOW(),NOW());
 INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'sender','Sender','sender',NOW(),NOW()),(2,'recipient','Recipient','recipient',NOW(),NOW()),(3,'other-sender','Other sender','other',NOW(),NOW()),(4,'outsider','Outsider','outside',NOW(),NOW());
 INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(1,1,'Sender',NOW(),NOW(),NOW()),(1,2,'Recipient',NOW(),NOW(),NOW()),(1,3,'Other sender',NOW(),NOW(),NOW()),(2,4,'Outsider',NOW(),NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,is_super_admin,created_at,updated_at) VALUES(5,'admin','Admin','admin',1,NOW(),NOW())`)
	date := time.Now().UTC().Format("2006-01-02")
	config := fmt.Sprintf(`{"task_sections":{"daily":{"devotion":{"enabled":false},"scripture":{"enabled":false},"verse":{"enabled":true,"plans":[{"date":"%s","verse_ref":"约3:16","recite_text":"神爱世人"}]}}}}`, date)
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES(1,?,NOW(),NOW())`, config)
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES(2,?,NOW(),NOW())`, config)
	a := &app{db: db, location: time.UTC, learning: learning.NewService(learning.NewMySQLRepository(db))}
	call := func(actor uint64, method, path, body string, status int, groups ...uint64) map[string]any {
		t.Helper()
		groupID := uint64(1)
		if len(groups) > 0 {
			groupID = groups[0]
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: actor, CurrentGroupID: groupID, IsSuperAdmin: actor == 5}))
		res := httptest.NewRecorder()
		switch {
		case strings.HasSuffix(path, "/read-all"):
			a.handleReadAllLearningReminders(res, req)
		case method == "GET":
			a.handleLearningReminders(res, req)
		case method == "PUT":
			a.handleLearningReminderPreference(res, req)
		case strings.HasSuffix(path, "/read"):
			req.SetPathValue("id", strings.Split(path, "/")[2])
			a.handleReadLearningReminder(res, req)
		default:
			a.handleCreateLearningReminder(res, req)
		}
		if res.Code != status {
			t.Fatalf("%s %s: %d want %d %s", method, path, res.Code, status, res.Body.String())
		}
		var data map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		return data
	}
	call(1, "POST", "/reminders", `{"user_id":1}`, 400)
	call(1, "POST", "/reminders", `{"user_id":4}`, 403)
	call(4, "POST", "/reminders", `{"user_id":2}`, 403)
	call(1, "POST", "/reminders", `{"user_id":2}`, 201)
	call(1, "POST", "/reminders", `{"user_id":2}`, 409)
	call(3, "POST", "/reminders", `{"user_id":2}`, 201)
	feed := call(2, "GET", "/reminders", "", 200)
	items := feed["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("feed=%v", feed)
	}
	id := items[0].(map[string]any)["id"].(float64)
	call(1, "POST", fmt.Sprintf("/reminders/%.0f/read", id), "", 404)
	call(2, "POST", fmt.Sprintf("/reminders/%.0f/read", id), "", 200)
	call(2, "POST", fmt.Sprintf("/reminders/%.0f/read", id), "", 200)
	feed = call(2, "GET", "/reminders", "", 200)
	if len(feed["items"].([]any)) != 1 {
		t.Fatal("read cleared unrelated notification")
	}
	if len(feed["deliveries"].([]any)) != 2 {
		t.Fatal("reading on one device prevented delivery to another device")
	}
	call(2, "PUT", "/reminders/preference", `{"muted":true}`, 200)
	feed = call(2, "GET", "/reminders", "", 200)
	if feed["muted"] != true || len(feed["items"].([]any)) != 1 {
		t.Fatal("mute lost unread")
	}
	testdb.Apply(t, db, "024_learning_reminders.sql")
	feed = call(2, "GET", "/reminders", "", 200)
	if feed["muted"] != true {
		t.Fatal("migration replay lost preference")
	}
	call(1, "POST", "/reminders/read-all", fmt.Sprintf(`{"through_id":%.0f}`, id), 200)
	if len(call(2, "GET", "/reminders", "", 200)["items"].([]any)) != 1 {
		t.Fatal("another user cleared the recipient's reminders")
	}
	call(2, "POST", "/reminders/read-all", `{"through_id":0}`, 400)
	call(2, "POST", "/reminders/read-all", fmt.Sprintf(`{"through_id":%.0f}`, id), 200)
	if len(call(2, "GET", "/reminders", "", 200)["items"].([]any)) != 0 {
		t.Fatal("clear-all did not persist read state")
	}
	call(5, "POST", "/reminders", `{"user_id":5}`, 400)
	call(5, "POST", "/reminders", `{"user_id":4}`, 403)
	call(5, "POST", "/reminders", `{"user_id":2}`, 201)
	call(5, "POST", "/reminders", `{"user_id":2}`, 409)
	adminFeed := call(5, "GET", "/reminders", "", 200)
	if len(adminFeed["items"].([]any)) != 0 || len(adminFeed["sent_ids"].([]any)) != 1 {
		t.Fatal("admin must see their sent state without reading other members' reminders")
	}
	adminReminder := call(2, "GET", "/reminders", "", 200)["items"].([]any)[0].(map[string]any)
	call(5, "POST", fmt.Sprintf("/reminders/%.0f/read", adminReminder["id"].(float64)), "", 404)
	if adminReminder["sender"] != "Admin" {
		t.Fatal("admin reminder lost the sender identity")
	}
	call(1, "POST", "/reminders", `{"user_id":4}`, 403, 2)
	call(5, "POST", "/reminders", `{"user_id":4}`, 201, 2)
	if len(call(4, "GET", "/reminders", "", 200, 2)["items"].([]any)) != 1 {
		t.Fatal("admin could not remind a member of another selected group")
	}
	testdb.Exec(t, db, `UPDATE group_members SET status=0 WHERE group_id=2 AND user_id=4`)
	call(5, "POST", "/reminders", `{"user_id":4}`, 403, 2)
	testdb.Exec(t, db, `UPDATE group_settings SET settings='{"task_sections":{"daily":{"devotion":{"enabled":false},"scripture":{"enabled":false},"verse":{"enabled":false}}}}' WHERE group_id=1`)
	call(5, "POST", "/reminders", `{"user_id":3}`, 409)
	call(2, "POST", "/reminders", `{"user_id":3}`, 409)
}
