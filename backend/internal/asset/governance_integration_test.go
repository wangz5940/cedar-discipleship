//go:build integration

package asset

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestDeleteOwnedResourceRejectsActiveTaskReference(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'owner','Owner',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (1,1,'2026-09-21','2026-09-27',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
		VALUES (1,1,1,'weekly_book','Book',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES
		  (1,1,'book','Book','book.pdf','team-owner-resources/objects/00000000000000000000000000000001/book.pdf',1,NOW(),NOW()),
		  (2,1,'book','Unused','unused.pdf','team-owner-resources/objects/00000000000000000000000000000002/unused.pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
		VALUES
		  (1,1,'00000000000000000000000000000001','owned',NOW(),NOW()),
		  (2,1,'00000000000000000000000000000002','owned',NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
		VALUES (1,1,1,'book',NOW())`)

	repo := NewMySQLRepository(db)
	if _, err := repo.BatchDelete(t.Context(), 1, 1, BatchDeleteInput{AssetIDs: []uint64{2, 1}}, time.Now()); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("batch delete error = %v, want %v", err, ErrAssetInUse)
	}
	assertTaskResourceDownloadable(t, db, repo, 1, 1, 1)
	if _, err := repo.FindDownloadTarget(t.Context(), 1, 2); err != nil {
		t.Fatalf("earlier unreferenced deletion was not rolled back: %v", err)
	}
}

func TestRemoveImportedResourceRejectsActiveTaskReference(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'owner','Owner',NOW(),NOW()),(2,'consumer','Consumer',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (2,2,'2026-09-21','2026-09-27',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
		VALUES (2,2,2,'weekly_book','Book',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES
		  (1,1,'book','Book','book.pdf','team-owner-resources/objects/00000000000000000000000000000001/book.pdf',1,NOW(),NOW()),
		  (2,2,'book','Book','book.pdf','team-owner-resources/objects/00000000000000000000000000000001/book.pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,created_at,updated_at)
		VALUES
		  (1,1,'00000000000000000000000000000001','owned',NULL,NULL,NOW(),NOW()),
		  (2,2,'00000000000000000000000000000002','imported',1,NOW(),NOW(),NOW());
		INSERT INTO asset_share_grants(asset_id,owner_group_id,consumer_group_id,permission,status,created_by,created_at)
		VALUES (1,1,2,'import','active',1,NOW());
		INSERT INTO asset_dependencies(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,dependency_type,status,created_at,updated_at)
		VALUES (2,2,1,1,'import','active',NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
		VALUES (2,2,2,'book',NOW())`)

	repo := NewMySQLRepository(db)
	if err := repo.RemoveImport(t.Context(), 2, 2, 1, time.Now()); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("remove import error = %v, want %v", err, ErrAssetInUse)
	}
	assertTaskResourceDownloadable(t, db, repo, 2, 2, 2)
}

func TestDeleteWaitsForConcurrentTaskReference(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'owner','Owner',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (1,1,'2026-09-21','2026-09-27',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
		VALUES (1,1,1,'weekly_book','Book',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES (1,1,'book','Book','book.pdf','team-owner-resources/objects/00000000000000000000000000000001/book.pdf',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
		VALUES (1,1,'00000000000000000000000000000001','owned',NOW(),NOW())`)

	linkTx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var assetID uint64
	if err := linkTx.QueryRowContext(t.Context(), `SELECT asset_id FROM asset_bindings
		WHERE asset_id=1 AND group_id=1 AND deleted_at IS NULL FOR SHARE`).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	repo := NewMySQLRepository(db)
	go func() {
		_, err := repo.BatchDelete(context.Background(), 1, 1, BatchDeleteInput{AssetIDs: []uint64{1}}, time.Now())
		errCh <- err
	}()
	testdb.WaitForLockWait(t, db)
	if _, err := linkTx.ExecContext(t.Context(), `INSERT INTO task_assets
		(group_id,task_id,asset_id,usage_type,created_at) VALUES (1,1,1,'book',NOW())`); err != nil {
		t.Fatal(err)
	}
	if err := linkTx.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrAssetInUse) {
			t.Fatalf("concurrent delete error = %v, want %v", err, ErrAssetInUse)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent delete did not finish")
	}
	assertTaskResourceDownloadable(t, db, repo, 1, 1, 1)
}

func assertTaskResourceDownloadable(
	t *testing.T,
	db *sql.DB,
	repo *MySQLRepository,
	groupID, weekID, assetID uint64,
) {
	t.Helper()
	if _, err := repo.FindDownloadTarget(t.Context(), groupID, assetID); err != nil {
		t.Fatalf("task resource is no longer downloadable: %v", err)
	}
	tasks, err := learning.NewMySQLRepository(db).ListTasks(t.Context(), groupID, weekID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || len(tasks[0].Assets) != 1 || tasks[0].Assets[0].ID != assetID {
		t.Fatalf("task resource changed after rejected removal: %+v", tasks)
	}
}
