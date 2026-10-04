//go:build integration

package checkin_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestTaskLookupIndexBoundsHistoricalScan(t *testing.T) {
	db := testdb.Open(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 1; i <= 1000; i++ {
		if _, err := tx.Exec(`INSERT INTO checkin_records(group_id,user_id,task_id,logical_date,checkin_time,task_type,part,created_by,created_at,updated_at)
			VALUES (1,1,?,'2026-10-01',NOW(),'weekly_book',?,1,NOW(),NOW())`, i, fmt.Sprintf("reading-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	query := `EXPLAIN FORMAT=JSON SELECT id FROM checkin_records WHERE group_id=1 AND user_id=1 AND task_id=500 AND task_type='weekly_book' AND deleted_at IS NULL ORDER BY logical_date,id LIMIT 1`
	planRows := func() int {
		var raw string
		if err := db.QueryRow(query).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var plan struct {
			Query struct {
				Ordering struct {
					Table struct {
						Rows int `json:"rows_examined_per_scan"`
					} `json:"table"`
				} `json:"ordering_operation"`
				Table struct {
					Rows int `json:"rows_examined_per_scan"`
				} `json:"table"`
			} `json:"query_block"`
		}
		if err := json.Unmarshal([]byte(raw), &plan); err != nil {
			t.Fatal(err)
		}
		if plan.Query.Table.Rows > 0 {
			return plan.Query.Table.Rows
		}
		return plan.Query.Ordering.Table.Rows
	}
	before := planRows()
	testdb.Apply(t, db, "022_checkin_task_lookup.sql")
	after := planRows()
	if before <= after || after < 1 || after > 2 {
		t.Fatalf("historical lookup scanned rows before=%d after=%d", before, after)
	}
	t.Logf("MySQL estimated rows per task lookup: %d -> %d", before, after)
}

func TestReadingReplacementPreservesCompletionAndIdentity(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "022_checkin_task_lookup.sql")
	testdb.Apply(t, db, "022_checkin_task_lookup.sql")
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'member','Member','member',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,book_enabled,video_enabled,verse_enabled,outline_enabled,created_at,updated_at)
		VALUES (185,1,'2026-10-01','2026-10-07',1,0,0,0,NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,content,created_at,updated_at)
		VALUES (1436,1,185,'weekly_book','灵命四季 13-27页','/api/assets/474/download',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES (474,1,'book','灵命四季','book.pdf','book.pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
		VALUES (474,1,'00000000000000000000000000000474','owned',NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
		VALUES (1,1436,474,'reading',NOW())`)
	checkins := checkin.NewService(checkin.NewMySQLRepository(db))
	original := checkin.Record{GroupID: 1, UserID: 1, TaskID: 1436, WeekID: 185, TaskType: "weekly_book", Part: "灵命四季 13-27页", Detail: "灵命四季 13-27页", LogicalDate: "2026-10-01"}
	id, existed, err := checkins.Create(t.Context(), &original, 1)
	if err != nil || existed {
		t.Fatalf("original: %d %v %v", id, existed, err)
	}
	repo := learning.NewMySQLRepository(db)
	service := learning.NewService(repo)
	input := learning.WeekInput{StartDate: "2026-10-01", EndDate: "2026-10-07", Title: "本周阅读", BookEnabled: true, Readings: []learning.TaskBinding{{Title: "灵命四季 13-15页", AssetID: 474, URL: "/api/assets/474/download"}}}
	if _, err := service.SaveWeek(t.Context(), 1, 185, input, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	tasks, err := repo.ListTasks(t.Context(), 1, 185)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("tasks: %+v %v", tasks, err)
	}
	settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false}}}}
	content, err := service.TodayContent(t.Context(), 1, "2026-10-04", settings, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	hub, err := service.TodayHubFromContent(t.Context(), 1, 1, content)
	if err != nil || len(hub.Tasks) != 1 || !hub.Tasks[0].Completed || hub.Tasks[0].Record.ID != id {
		t.Fatalf("hub: %+v %v", hub, err)
	}
	calendar, err := service.CalendarProgress(t.Context(), 1, 1, "2026-10", settings, time.UTC)
	if err != nil || calendar["2026-10-04"].Completed != 1 {
		t.Fatalf("calendar: %+v %v", calendar, err)
	}
	completion, err := service.GroupTaskCompletionsFromContent(t.Context(), 1, content)
	if err != nil || len(completion.Items) != 1 || !completion.Items[0].Completed {
		t.Fatalf("group completion: %+v %v", completion, err)
	}
	repeated := checkin.Record{GroupID: 1, UserID: 1, TaskID: tasks[0].ID, WeekID: 185, TaskType: "weekly_book", Part: "灵命四季 13-15页", Detail: "灵命四季 13-15页", LogicalDate: "2026-10-04"}
	secondID, existed, err := checkins.Create(t.Context(), &repeated, 1)
	if err != nil || !existed || secondID != id {
		t.Fatalf("repeated: %d %v %v", secondID, existed, err)
	}
	var count int
	var part string
	if err := db.QueryRow(`SELECT COUNT(*),MIN(part) FROM checkin_records WHERE group_id=1 AND user_id=1`).Scan(&count, &part); err != nil || count != 1 || part != original.Part {
		t.Fatalf("historical data altered: %d %q %v", count, part, err)
	}
	if _, err := checkin.NewMySQLRepository(db).FindExistingWeeklyBook(t.Context(), 1, 2, tasks[0].ID, 185, repeated.Part, repeated.Detail); err == nil {
		t.Fatal("another account inherited completion")
	}
	testdb.Exec(t, db, `UPDATE study_tasks SET title='灵命四季 13-28页' WHERE id=?`, tasks[0].ID)
	if _, err := checkin.NewMySQLRepository(db).FindExistingWeeklyBook(t.Context(), 1, 1, tasks[0].ID, 185, "灵命四季 13-28页", "灵命四季 13-28页"); err == nil {
		t.Fatal("unfinished larger range inherited completion")
	}
}

func TestReadingReplacementCoverageUsesUntruncatedDetail(t *testing.T) {
	db := testdb.Open(t)
	oldTitle := strings.Repeat("旧读物名称", 15) + " 13-27页"
	newTitle := strings.Repeat("新读物名称", 15) + " 13-15页"
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'reader','Reader','reader',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (1,1,'2026-10-01','2026-10-07',NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
		VALUES (1,1,1,'reading',NOW()),(1,2,1,'reading',NOW())`)
	testdb.Exec(t, db, `INSERT INTO study_tasks(id,group_id,week_id,task_type,title,content,created_at,updated_at)
		VALUES (1,1,1,'weekly_book',?,'/api/assets/1/download',NOW(),NOW()),
		       (2,1,1,'weekly_book',?,'/api/assets/1/download',NOW(),NOW())`, oldTitle, newTitle)
	service := checkin.NewService(checkin.NewMySQLRepository(db))
	original := checkin.Record{GroupID: 1, UserID: 1, TaskID: 1, WeekID: 1, TaskType: "weekly_book",
		Part: oldTitle, Detail: oldTitle, LogicalDate: "2026-10-01"}
	firstID, existed, err := service.Create(t.Context(), &original, 1)
	if err != nil || existed {
		t.Fatalf("original reading: %d %v %v", firstID, existed, err)
	}
	testdb.Exec(t, db, `UPDATE study_tasks SET week_id=NULL,enabled=0 WHERE id=1`)
	replacement := checkin.Record{GroupID: 1, UserID: 1, TaskID: 2, WeekID: 1, TaskType: "weekly_book",
		Part: newTitle, Detail: newTitle, LogicalDate: "2026-10-04"}
	if id, existed, err := service.Create(t.Context(), &replacement, 1); err != nil || !existed || id != firstID {
		t.Fatalf("narrowed long reading: %d %v %v", id, existed, err)
	}
	var count int
	var detail string
	if err := db.QueryRow(`SELECT COUNT(*),MIN(detail) FROM checkin_records WHERE group_id=1 AND user_id=1`).Scan(&count, &detail); err != nil {
		t.Fatal(err)
	}
	if count != 1 || detail != oldTitle {
		t.Fatalf("reading history changed: rows=%d detail=%q", count, detail)
	}
}
