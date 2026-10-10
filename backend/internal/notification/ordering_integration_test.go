//go:build integration

package notification

import (
	"reflect"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestCheckinSummariesDoNotRegressAfterFullSnapshot(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'one','张三','one',NOW(),NOW()),(2,'two','李四','two',NOW(),NOW()),
		       (3,'former','退组成员','former',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
		VALUES (1,1,'张三',1,NOW(),NOW(),NOW()),(1,2,'李四',1,NOW(),NOW(),NOW()),
		       (1,3,'退组成员',0,NOW(),NOW(),NOW()),(2,1,'其他组',1,NOW(),NOW(),NOW());
		INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES (1,'{"checkin_notifications":{"weekly_enabled":false}}',NOW(),NOW());
		INSERT INTO checkin_records(id,group_id,user_id,task_type,logical_date,checkin_time,source,created_by,created_at,updated_at)
		VALUES (10,1,1,'daily_devotion','2026-09-27','2026-09-27 08:00:00','web',1,NOW(),NOW()),
		       (20,1,2,'daily_devotion','2026-09-27','2026-09-27 08:01:00','web',2,NOW(),NOW()),
		       (99,1,3,'daily_devotion','2026-09-27','2026-09-27 08:02:00','web',3,NOW(),NOW()),
		       (100,2,1,'daily_devotion','2026-09-27','2026-09-27 08:03:00','web',1,NOW(),NOW()),
		       (130,1,1,'daily_devotion','2026-09-26','2026-09-26 08:00:00','web',1,NOW(),NOW())`)
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	source := NewCheckinSource(db, time.UTC)
	full, err := source.Snapshot(t.Context(), Event{GroupID: 1, Initial: "daily", OccurredAt: now})
	if err != nil || full.CoveredRecordID != 20 || full.Text != "每日灵修\n1 张三\n2 李四" {
		t.Fatalf("full snapshot=%+v err=%v", full, err)
	}
	sender := &fakeSender{}
	dir := t.TempDir()
	targets := map[uint64][]Target{1: {{ChatID: 99, ChatType: 3}}}
	queue, err := NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.EnqueueInitial(now); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	queue, err = NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint64{10, 20} {
		if err := queue.Enqueue(Event{RecordID: id, GroupID: 1, LogicalDate: "2026-09-27", OccurredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	for range 3 {
		queue.processNext(t.Context(), now)
	}
	if !reflect.DeepEqual(sender.messages, []string{full.Text}) {
		t.Fatalf("delayed events regressed progress: %#v", sender.messages)
	}
	testdb.Exec(t, db, `UPDATE checkin_records SET deleted_at=NOW(),active_key=id WHERE id=20`)
	if err := queue.WakeInitial(1, now); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		queue.processNext(t.Context(), now)
	}
	testdb.Exec(t, db, `INSERT INTO checkin_records(id,group_id,user_id,task_type,logical_date,checkin_time,source,created_by,created_at,updated_at)
		VALUES (140,1,2,'daily_devotion','2026-09-27','2026-09-27 09:00:00','web',2,NOW(),NOW())`)
	if err := queue.Enqueue(Event{RecordID: 140, GroupID: 1, LogicalDate: "2026-09-27", OccurredAt: now}); err != nil {
		t.Fatal(err)
	}
	queue.processNext(t.Context(), now)
	if !reflect.DeepEqual(sender.messages, []string{full.Text, "每日灵修\n1 张三", full.Text}) {
		t.Fatalf("cancellation/recheckin messages=%#v", sender.messages)
	}
}

func TestDailyNotificationFormatStaysStableBeforeAndAfterVerseCheckin(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
 VALUES (1,'one','喜悦','one',NOW(),NOW()),(2,'two','合聪','two',NOW(),NOW());
 INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
 VALUES (1,1,'喜悦',1,NOW(),NOW(),NOW()),(1,2,'合聪',1,NOW(),NOW(),NOW());
 INSERT INTO group_settings(group_id,settings,created_at,updated_at)
 VALUES (1,'{"task_sections":{"daily":{"verse":{"enabled":true,"plans":[{"date":"2026-10-10","verse_ref":"约3:16"}]}}}}',NOW(),NOW());
 INSERT INTO checkin_records(id,group_id,user_id,task_type,logical_date,checkin_time,source,created_by,created_at,updated_at)
 VALUES (1,1,1,'daily_devotion','2026-10-10','2026-10-10 08:30:00','web',1,NOW(),NOW()),
 (2,1,2,'daily_devotion','2026-10-10','2026-10-10 08:35:00','web',2,NOW(),NOW())`)
	source := NewCheckinSource(db, time.UTC)
	before, err := source.Snapshot(t.Context(), Event{GroupID: 1, RecordID: 2, LogicalDate: "2026-10-10"})
	if err != nil || before.Text != "每日任务\n1 喜悦 灵修\n2 合聪 【新】灵修" {
		t.Fatalf("before verse: %q err=%v", before.Text, err)
	}
	testdb.Exec(t, db, `INSERT INTO checkin_records(id,group_id,user_id,task_type,logical_date,checkin_time,source,created_by,created_at,updated_at)
 VALUES (3,1,2,'daily_verse','2026-10-10','2026-10-10 08:40:00','web',2,NOW(),NOW()),
 (4,1,1,'daily_verse','2026-10-10','2026-10-10 10:44:00','web',1,NOW(),NOW())`)
	for _, tc := range []struct {
		name string
		id   uint64
		want string
	}{
		{"first verse", 3, "每日任务\n1 喜悦 灵修\n2 合聪 灵修 【新】背经"},
		{"second verse", 4, "每日任务\n1 喜悦 灵修 【新】背经\n2 合聪 灵修 背经"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := source.Snapshot(t.Context(), Event{GroupID: 1, RecordID: tc.id, LogicalDate: "2026-10-10"})
			if err != nil || snapshot.Text != tc.want {
				t.Fatalf("snapshot=%q err=%v", snapshot.Text, err)
			}
		})
	}
}
