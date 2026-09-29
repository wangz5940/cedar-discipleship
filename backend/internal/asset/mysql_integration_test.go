//go:build integration

package asset_test

import (
	"database/sql"
	"testing"
	"time"

	"agp/backend/internal/asset"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestRenamePreservesResourceIdentityAndPropagationRules(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (1,2,'2026-09-21','2026-09-27',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,content,created_at,updated_at)
		VALUES (1,2,1,'weekly_book','旧名称 36-40页','',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,mime_type,created_by,created_at,updated_at)
		VALUES
		  (1,1,'book','旧名称','source.pdf','team-owner-resources/objects/00000000000000000000000000000001/source.pdf','application/pdf',1,NOW(),NOW()),
		  (2,2,'book','旧名称','source.pdf','team-owner-resources/objects/00000000000000000000000000000001/source.pdf','application/pdf',1,NOW(),NOW()),
		  (3,3,'book','旧名称','source.pdf','team-owner-resources/objects/00000000000000000000000000000001/source.pdf','application/pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES
		  (1,1,'00000000000000000000000000000001','owned',NULL,NULL,NULL,NOW(),NOW()),
		  (2,2,'00000000000000000000000000000002','imported',1,NOW(),NULL,NOW(),NOW()),
		  (3,3,'00000000000000000000000000000003','imported',1,NOW(),NOW(),NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
		VALUES (2,1,2,'book',NOW());
		INSERT INTO checkin_records(group_id,user_id,task_id,week_id,logical_date,checkin_time,task_type,detail,part,created_by,created_at,updated_at)
		VALUES (2,7,1,1,'2026-09-22',NOW(),'weekly_book','旧名称 36-40页','旧名称 36-40页',7,NOW(),NOW())`)

	repo := asset.NewMySQLRepository(db)
	renamedAt := time.Date(2026, 9, 25, 1, 2, 3, 0, time.UTC)
	if err := repo.Rename(t.Context(), 1, 1, "来源组新名称", renamedAt); err != nil {
		t.Fatal(err)
	}
	assertAssetFields(t, db, 1, "来源组新名称", "source.pdf",
		"team-owner-resources/objects/00000000000000000000000000000001/source.pdf")
	assertAssetFields(t, db, 2, "来源组新名称", "source.pdf",
		"team-owner-resources/objects/00000000000000000000000000000001/source.pdf")
	assertAssetFields(t, db, 3, "旧名称", "source.pdf",
		"team-owner-resources/objects/00000000000000000000000000000001/source.pdf")

	if err := repo.Rename(t.Context(), 2, 2, "本组自定义名称", renamedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertAssetFields(t, db, 1, "来源组新名称", "source.pdf",
		"team-owner-resources/objects/00000000000000000000000000000001/source.pdf")
	assertAssetFields(t, db, 2, "本组自定义名称", "source.pdf",
		"team-owner-resources/objects/00000000000000000000000000000001/source.pdf")

	tasks, err := learning.NewMySQLRepository(db).ListTasks(t.Context(), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Title != "旧名称 36-40页" ||
		len(tasks[0].Assets) != 1 || tasks[0].Assets[0].ID != 2 ||
		tasks[0].Assets[0].Title != "本组自定义名称" {
		t.Fatalf("task identity or display title changed unexpectedly: %+v", tasks)
	}

	var taskAssetID uint64
	if err := db.QueryRow(`SELECT asset_id FROM task_assets WHERE group_id=2 AND task_id=1`).Scan(&taskAssetID); err != nil {
		t.Fatal(err)
	}
	if taskAssetID != 2 {
		t.Fatalf("task asset ID = %d, want 2", taskAssetID)
	}
	var checkinTaskID, checkinWeekID uint64
	var checkinPart string
	if err := db.QueryRow(`SELECT task_id,week_id,part FROM checkin_records
		WHERE group_id=2 AND user_id=7 AND logical_date='2026-09-22'`).
		Scan(&checkinTaskID, &checkinWeekID, &checkinPart); err != nil {
		t.Fatal(err)
	}
	if checkinTaskID != 1 || checkinWeekID != 1 || checkinPart != "旧名称 36-40页" {
		t.Fatalf("check-in identity changed: task=%d week=%d part=%q", checkinTaskID, checkinWeekID, checkinPart)
	}
}

func TestChangeCategoryPropagatesOnlyThroughActiveOwnedBindings(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,mime_type,created_by,created_at,updated_at)
		VALUES
		  (1,1,'markdown','课程','course.pdf','team-owner-resources/objects/00000000000000000000000000000001/course.pdf','application/pdf',1,NOW(),NOW()),
		  (2,2,'markdown','课程','course.pdf','team-owner-resources/objects/00000000000000000000000000000001/course.pdf','application/pdf',1,NOW(),NOW()),
		  (3,3,'markdown','课程','course.pdf','team-owner-resources/objects/00000000000000000000000000000001/course.pdf','application/pdf',1,NOW(),NOW()),
		  (4,1,'book','第二课','second.pdf','team-source-resources/objects/00000000000000000000000000000005/second.pdf','application/pdf',1,NOW(),NOW()),
		  (5,2,'book','第二课','second.pdf','team-source-resources/objects/00000000000000000000000000000005/second.pdf','application/pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES
		  (1,1,'00000000000000000000000000000001','owned',NULL,NULL,NULL,NOW(),NOW()),
		  (2,2,'00000000000000000000000000000002','imported',1,NOW(),NULL,NOW(),NOW()),
		  (3,3,'00000000000000000000000000000003','imported',1,NOW(),NOW(),NOW(),NOW()),
		  (4,1,'00000000000000000000000000000004','imported',5,NOW(),NULL,NOW(),NOW()),
		  (5,2,'00000000000000000000000000000005','owned',NULL,NULL,NULL,NOW(),NOW())`)

	repo := asset.NewMySQLRepository(db)
	changedAt := time.Date(2026, 9, 25, 1, 2, 3, 0, time.UTC)
	if err := repo.ChangeCategory(t.Context(), 1, 1, "handout", changedAt); err != nil {
		t.Fatal(err)
	}
	assertAssetCategory(t, db, 1, "handout")
	assertAssetCategory(t, db, 2, "handout")
	assertAssetCategory(t, db, 3, "markdown")

	if err := repo.ChangeCategory(t.Context(), 2, 2, "passage", changedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertAssetCategory(t, db, 1, "handout")
	assertAssetCategory(t, db, 2, "passage")

	result, err := repo.BatchChangeCategory(t.Context(), 1, asset.BatchCategoryInput{
		AssetIDs: []uint64{1, 4},
		Category: "passage",
	}, changedAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 2 {
		t.Fatalf("batch result = %+v, want 2 updates", result)
	}
	assertAssetCategory(t, db, 1, "passage")
	assertAssetCategory(t, db, 2, "passage")
	assertAssetCategory(t, db, 3, "markdown")
	assertAssetCategory(t, db, 4, "passage")
	assertAssetCategory(t, db, 5, "book")

	if _, err := repo.BatchChangeCategory(t.Context(), 1, asset.BatchCategoryInput{
		AssetIDs: []uint64{4, 999},
		Category: "book",
	}, changedAt.Add(3*time.Minute)); err == nil {
		t.Fatal("BatchChangeCategory() error = nil, want missing asset")
	}
	assertAssetCategory(t, db, 4, "passage")
}

func assertAssetCategory(t *testing.T, db *sql.DB, assetID uint64, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT category FROM assets WHERE id=?`, assetID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("asset %d category = %q, want %q", assetID, got, want)
	}
}

func assertAssetFields(t *testing.T, db *sql.DB, assetID uint64, wantTitle, wantOriginalName, wantStoragePath string) {
	t.Helper()
	var title, originalName, storagePath string
	if err := db.QueryRow(`SELECT title,original_name,storage_path FROM assets WHERE id=?`, assetID).
		Scan(&title, &originalName, &storagePath); err != nil {
		t.Fatal(err)
	}
	if title != wantTitle || originalName != wantOriginalName || storagePath != wantStoragePath {
		t.Fatalf("asset %d = title %q original %q path %q", assetID, title, originalName, storagePath)
	}
}
