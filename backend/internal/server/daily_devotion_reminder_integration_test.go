//go:build integration

package server

import (
	"agp/backend/internal/testdb"
	"context"
	"encoding/base64"
	"encoding/json"
	webpush "github.com/SherClockHolmes/webpush-go"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDailyDevotionReminderSchedule(t *testing.T) {
	db := testdb.Open(t)
	for _, name := range []string{"025_learning_web_push.sql", "026_unlimited_superadmin_reminders.sql", "027_apple_reminder_resume.sql", "028_daily_devotion_reminders.sql", "028_daily_devotion_reminders.sql", "029_devotion_reminder_slots.sql", "029_devotion_reminder_slots.sql"} {
		testdb.Apply(t, db, name)
	}
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'daily','Daily',NOW(),NOW()),(2,'off','Off',NOW(),NOW());
 INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES(2,'{"checkin_notifications":{"daily_enabled":false}}',NOW(),NOW());
 INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'a','A','a',NOW(),NOW()),(2,'b','B','b',NOW(),NOW()),(3,'c','C','c',NOW(),NOW());
 INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(1,1,'A',NOW(),NOW(),NOW()),(1,2,'B',NOW(),NOW(),NOW()),(2,3,'C',NOW(),NOW(),NOW());
 INSERT INTO learning_reminder_preferences(group_id,user_id,muted) VALUES(1,2,TRUE)`)
	a := &app{db: db, location: time.FixedZone("Asia/Shanghai", 8*60*60)}
	if err := a.initLearningPush(); err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(a.location)
	morning := time.Date(now.Year(), now.Month(), now.Day(), 7, 30, 0, 0, a.location)
	_, key, _ := webpush.GenerateVAPIDKeys()
	sub := webpush.Subscription{Endpoint: "https://web.push.apple.com/daily", Keys: webpush.Keys{P256dh: key, Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
	data, _ := json.Marshal(sub)
	testdb.Exec(t, db, `INSERT INTO learning_push_subscriptions(endpoint_hash,group_id,user_id,subscription,created_at) VALUES(UNHEX(SHA2('daily',256)),1,1,?,?)`, data, morning.Add(-time.Hour))
	run := func(at time.Time) {
		t.Helper()
		if err := a.enqueueDailyDevotionReminders(context.Background(), at); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		var got int
		if err := db.QueryRow(`SELECT COUNT(*) FROM learning_reminders`).Scan(&got); err != nil || got != want {
			t.Fatalf("reminders=%d want=%d err=%v", got, want, err)
		}
	}
	run(morning.Add(-time.Second))
	count(0)
	run(morning)
	count(1)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := a.enqueueDailyDevotionReminders(context.Background(), morning.Add(5*time.Second)); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	count(1)
	req := httptest.NewRequest("GET", "/api/learning-reminders", nil)
	req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
	res := httptest.NewRecorder()
	a.handleLearningReminders(res, req)
	if res.Code != 200 || !strings.Contains(res.Body.String(), "每日灵修") {
		t.Fatalf("scheduled reminder API: status=%d body=%s", res.Code, res.Body.String())
	}
	testdb.Exec(t, db, `UPDATE learning_reminder_preferences SET muted=FALSE WHERE group_id=1 AND user_id=2`)
	run(morning.Add(10 * time.Second))
	count(1)
	client := &pushTestClient{status: 201}
	testdb.Exec(t, db, `INSERT INTO group_settings(group_id,settings,created_at,updated_at) VALUES(1,'{"checkin_notifications":{"daily_enabled":false}}',NOW(),NOW())`)
	if err := a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Fatal("disabled switch delivered scheduled push")
	}
	testdb.Exec(t, db, `UPDATE group_settings SET settings='{"checkin_notifications":{"daily_enabled":true}}' WHERE group_id=1`)
	if err := a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatalf("system push deliveries=%d", client.calls)
	}
	if err := a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 {
		t.Fatal("system push repeated")
	}
	evening := morning.Add(11 * time.Hour)
	run(evening.Add(-time.Second))
	count(1)
	run(evening)
	count(3)
	if err := a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatalf("evening push blocked by morning delivery: calls=%d", client.calls)
	}
	if err := a.deliverLearningPush(context.Background(), client); err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatal("evening push repeated")
	}
	run(evening.Add(5 * time.Second))
	count(3)
	run(evening.Add(time.Minute))
	count(3)
	run(morning.Add(24*time.Hour + time.Minute))
	count(3)
	run(morning.Add(24 * time.Hour))
	count(5)
	testdb.Exec(t, db, `UPDATE group_settings SET settings='{"checkin_notifications":{"daily_enabled":true}}' WHERE group_id=2;UPDATE group_members SET status=0 WHERE group_id=2`)
	run(morning.Add(48 * time.Hour))
	count(7)
}

func TestDevotionReminderSlotMigrationPreservesMorning(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO daily_devotion_reminder_runs(group_id,logical_date) VALUES(1,'2026-10-10')`)
	testdb.Apply(t, db, "029_devotion_reminder_slots.sql")
	testdb.Apply(t, db, "029_devotion_reminder_slots.sql")
	testdb.Exec(t, db, `INSERT IGNORE INTO daily_devotion_reminder_runs(group_id,logical_date,reminder_hour) VALUES(1,'2026-10-10',7),(1,'2026-10-10',18),(1,'2026-10-10',18)`)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM daily_devotion_reminder_runs WHERE group_id=1 AND logical_date='2026-10-10'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration lost morning or duplicated evening: count=%d error=%v", count, err)
	}
}
