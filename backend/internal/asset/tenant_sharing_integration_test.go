//go:build integration

package asset

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestSharingStaysWithinTenant(t *testing.T) {
	db := testdb.Open(t)
	path := "team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/file.pdf"
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at) VALUES(2,'B',1,NOW(),NOW());
		INSERT INTO study_groups(id,tenant_id,code,name,created_at,updated_at)
		VALUES(1,1,'a','A',NOW(),NOW()),(2,1,'a2','A2',NOW(),NOW()),(3,2,'b','B',NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES(1,1,'book','Book','file.pdf',?,1,NOW(),NOW())`, path)
	testdb.Exec(t, db, `
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
		VALUES(1,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','owned',NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{
		Scope: ShareScopeSelectedGroups, ConsumerGroupIDs: []uint64{3},
	}, time.Now()); !errors.Is(err, ErrInvalidShareScope) {
		t.Fatalf("cross-tenant share error = %v", err)
	}
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopeAllGroups}, time.Now()); err != nil {
		t.Fatal(err)
	}
	inside, err := repo.SharedResources(t.Context(), 2, SharedFilter{})
	if err != nil || len(inside) != 1 || inside[0].AssetID != 1 {
		t.Fatalf("same-tenant shared resources = %+v, err = %v", inside, err)
	}
	outside, err := repo.SharedResources(t.Context(), 3, SharedFilter{})
	if err != nil || len(outside) != 0 {
		t.Fatalf("cross-tenant shared resources = %+v, err = %v", outside, err)
	}
	if _, err := repo.sharedSource(t.Context(), 3, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant source error = %v", err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := repo.RestoreReferenceTx(t.Context(), tx, 3, 1, path, time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant backup reference error = %v", err)
	}
}

func TestImportedResourceSurvivesTenantMove(t *testing.T) {
	db := testdb.Open(t)
	path := "team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/file.pdf"
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at) VALUES(2,'B',1,NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,tenant_id,code,name,created_at,updated_at)
		VALUES(1,1,'owner','Owner',NOW(),NOW()),(2,1,'consumer','Consumer',NOW(),NOW()),(3,2,'other','Other',NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES(1,1,'book','Book','file.pdf',?,1,NOW(),NOW())`, path)
	testdb.Exec(t, db, `INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
		VALUES(1,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','owned',NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	now := time.Now()
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopeAllGroups}, now); err != nil {
		t.Fatal(err)
	}
	imported, err := repo.Import(t.Context(), 2, 1, ImportInput{SourceAssetID: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); err != nil {
		t.Fatalf("same-tenant import cannot be read: %v", err)
	}
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopePrivate}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("same-tenant revoked import remains readable: %v", err)
	}
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopeAllGroups}, now); err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, `UPDATE study_groups SET tenant_id=2 WHERE id=2`)
	items, err := repo.List(t.Context(), 2, 0)
	if err != nil || len(items) != 1 || items[0].ID != imported.ID {
		t.Fatalf("imported resource disappeared after move: %+v, %v", items, err)
	}
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); err != nil {
		t.Fatalf("imported resource cannot be read after move: %v", err)
	}
	shared, err := repo.SharedResources(t.Context(), 2, SharedFilter{})
	if err != nil || len(shared) != 0 {
		t.Fatalf("unimported cross-tenant resources visible: %+v, %v", shared, err)
	}
	if _, err := repo.ImportPreview(t.Context(), 2, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-tenant import preview visible: %v", err)
	}
	if _, err := repo.Import(t.Context(), 2, 1, ImportInput{SourceAssetID: 1}, now); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("new cross-tenant import allowed: %v", err)
	}
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopePrivate}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("revoked cross-tenant import remains readable: %v", err)
	}
	if err := repo.SaveShareSettings(t.Context(), 1, 1, 1, ShareInput{Scope: ShareScopeAllGroups}, now); err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, `UPDATE study_groups SET tenant_id=2 WHERE id=1`)
	testdb.Exec(t, db, `UPDATE study_groups SET tenant_id=1 WHERE id=2`)
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); err != nil {
		t.Fatalf("import lost when source group moved: %v", err)
	}
	shared, err = repo.SharedResources(t.Context(), 2, SharedFilter{})
	if err != nil || len(shared) != 0 {
		t.Fatalf("moved source group visible for new imports: %+v, %v", shared, err)
	}
	if err := repo.RemoveImport(t.Context(), 2, imported.ID, 1, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDownloadTarget(t.Context(), 2, imported.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("removed import remains readable: %v", err)
	}
}
