//go:build integration

package learning

import (
	"strings"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestReplaceWeekTasksRequiresActiveAssetBinding(t *testing.T) {
	for _, test := range []struct {
		name      string
		deletedAt string
		wantError bool
	}{
		{name: "active binding", deletedAt: "NULL"},
		{name: "deleted binding", deletedAt: "NOW()", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
				VALUES (1,'a','A',NOW(),NOW());
				INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
				VALUES (1,1,'2026-09-21','2026-09-27',NOW(),NOW());
				INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
				VALUES (1,1,'book','Book','book.pdf','team-a-resources/objects/00000000000000000000000000000001/book.pdf',1,NOW(),NOW())`)
			testdb.Exec(t, db, `INSERT INTO asset_bindings
				(asset_id,group_id,resource_key,asset_kind,deleted_at,created_at,updated_at)
				VALUES (1,1,'00000000000000000000000000000001','owned',`+test.deletedAt+`,NOW(),NOW())`)

			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			err = ReplaceWeekTasksTx(t.Context(), tx, 1, 1, []TaskDraft{{
				TaskType:  "weekly_book",
				Title:     "Book",
				AssetID:   1,
				UsageType: "book",
			}}, time.Now())
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "asset_not_found") {
					t.Fatalf("error = %v, want asset_not_found", err)
				}
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					t.Fatal(rollbackErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM task_assets WHERE group_id=1 AND asset_id=1`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("task asset count = %d, want 1", count)
			}
		})
	}
}
