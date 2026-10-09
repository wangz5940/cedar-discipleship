//go:build integration

package server

import (
	"agp/backend/internal/testdb"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

type pushTestClient struct {
	calls  int
	status int
}

func (c *pushTestClient) Do(r *http.Request) (*http.Response, error) {
	c.calls++
	return &http.Response{StatusCode: c.status, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestLearningPushPersistenceAndIsolation(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "024_learning_reminders.sql")
	testdb.Apply(t, db, "026_unlimited_superadmin_reminders.sql")
	testdb.Apply(t, db, "027_apple_reminder_resume.sql")
	testdb.Apply(t, db, "027_apple_reminder_resume.sql")
	testdb.Apply(t, db, "025_learning_web_push.sql")
	testdb.Apply(t, db, "025_learning_web_push.sql") // Startup replays migrations; restarting must remain safe.
	a := &app{db: db}
	if err := a.initLearningPush(); err != nil {
		t.Fatal(err)
	}
	public := a.learningPush.public
	if err := a.initLearningPush(); err != nil {
		t.Fatal(err)
	}
	if public != a.learningPush.public {
		t.Fatal("push identity changed across restart")
	}
	_, deviceKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatal(err)
	}
	subscription := webpush.Subscription{Endpoint: "https://web.push.apple.com/test-device", Keys: webpush.Keys{P256dh: deviceKey, Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
	if !validLearningPushSubscription(subscription) {
		t.Fatal("valid device rejected")
	}
	for _, endpoint := range []string{"http://web.push.apple.com/a", "https://127.0.0.1/a", "https://web.push.apple.com.evil.example/a", "https://web.push.apple.com:8080/a", "https://user@web.push.apple.com/a"} {
		invalid := subscription
		invalid.Endpoint = endpoint
		if validLearningPushSubscription(invalid) {
			t.Fatal("unsafe push destination accepted")
		}
	}
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'push','Push',NOW(),NOW()); INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'sender','Sender','sender',NOW(),NOW()),(2,'recipient','Recipient','recipient',NOW(),NOW()),(3,'outsider','Outsider','outside',NOW(),NOW()); INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(1,1,'Sender',NOW(),NOW(),NOW()),(1,2,'Recipient',NOW(),NOW(),NOW())`)
	data, _ := json.Marshal(subscription)
	subscribe := func(actor uint64, status int) {
		t.Helper()
		r := httptest.NewRequest("PUT", "/subscription", strings.NewReader(string(data)))
		r = r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: actor, CurrentGroupID: 1}))
		w := httptest.NewRecorder()
		a.handleLearningPushSubscribe(w, r)
		if w.Code != status {
			t.Fatalf("subscribe status=%d body=%s", w.Code, w.Body.String())
		}
	}
	subscribe(3, 403)
	subscribe(2, 200)
	subscribe(2, 200)
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM learning_push_subscriptions`).Scan(&count); err != nil || count != 1 {
		t.Fatal("device subscription not idempotent", err)
	}
	testdb.Exec(t, db, `INSERT INTO learning_reminders(group_id,sender_id,recipient_id,logical_date,created_at) VALUES(1,1,2,UTC_DATE(),UTC_TIMESTAMP(3)+INTERVAL 1 SECOND)`)
	client := &pushTestClient{status: 201}
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("expected push while page closed, got %d", client.calls)
	}
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("successful notification repeated")
	}
	testdb.Exec(t, db, `INSERT INTO learning_reminders(group_id,sender_id,recipient_id,logical_date,created_at) VALUES(1,3,2,UTC_DATE(),UTC_TIMESTAMP(3)+INTERVAL 1 SECOND); INSERT INTO learning_reminder_preferences(group_id,user_id,muted) VALUES(1,2,TRUE)`)
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("muted recipient received push")
	}
	resume := httptest.NewRequest("PUT", "/preference", strings.NewReader(`{"muted":false}`))
	resume = resume.WithContext(context.WithValue(resume.Context(), currentUserKey, currentUser{ID: 2, CurrentGroupID: 1}))
	w := httptest.NewRecorder()
	a.handleLearningReminderPreference(w, resume)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("Apple backlog replayed after unmuting")
	}
	testdb.Exec(t, db, `INSERT INTO learning_reminders(group_id,sender_id,recipient_id,logical_date,created_at,daily_limit_slot) VALUES(1,1,2,UTC_DATE(),UTC_TIMESTAMP(3)+INTERVAL 1 SECOND,NULL)`)
	// Re-saving an already unmuted preference must not suppress a newer reminder.
	resumeAgain := httptest.NewRequest("PUT", "/preference", strings.NewReader(`{"muted":false}`))
	resumeAgain = resumeAgain.WithContext(context.WithValue(resumeAgain.Context(), currentUserKey, currentUser{ID: 2, CurrentGroupID: 1}))
	a.handleLearningReminderPreference(httptest.NewRecorder(), resumeAgain)
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatal("new Apple reminder suppressed after unmuting")
	}
	testdb.Exec(t, db, `DELETE FROM learning_push_deliveries WHERE reminder_id=(SELECT MAX(id) FROM learning_reminders); DELETE FROM learning_reminders WHERE daily_limit_slot IS NULL`)
	client.calls = 1
	var unread int
	if err = db.QueryRow(`SELECT COUNT(*) FROM learning_reminders WHERE sender_id=3 AND read_at IS NULL`).Scan(&unread); err != nil || unread != 1 {
		t.Fatal("unmuting changed unread history", err)
	}
	testdb.Exec(t, db, `UPDATE learning_push_subscriptions SET subscription=JSON_SET(subscription,'$.endpoint','https://updates.push.services.mozilla.com/test-device')`)
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatal("desktop delivery changed")
	}
	testdb.Exec(t, db, `DELETE FROM learning_push_deliveries WHERE reminder_id=(SELECT MAX(id) FROM learning_reminders); UPDATE learning_reminder_preferences SET apple_push_after_id=0; UPDATE learning_push_subscriptions SET subscription=JSON_SET(subscription,'$.endpoint','https://web.push.apple.com/test-device')`)
	client.calls = 1
	testdb.Exec(t, db, `UPDATE learning_reminder_preferences SET muted=FALSE; UPDATE learning_reminders SET read_at=UTC_TIMESTAMP(3) WHERE sender_id=3`)
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("read reminder received push")
	}
	testdb.Exec(t, db, `UPDATE learning_reminders SET read_at=NULL WHERE sender_id=3`)
	client.status = 503
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatal("failed push not attempted")
	}
	testdb.Exec(t, db, `UPDATE learning_push_deliveries SET next_attempt_at=UTC_TIMESTAMP(3) WHERE finished=FALSE`)
	client.status = 410
	if err = a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM learning_push_subscriptions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("expired subscription retained", err)
	}
}
