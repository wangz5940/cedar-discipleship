//go:build integration

package main

import (
	"testing"

	"agp/backend/internal/asset"
	"agp/backend/internal/testdb"
)

func TestCleanupDuplicateResourceBindings(t *testing.T) {
	for _, hasConsumer := range []bool{false, true} {
		name := "unreferenced duplicates"
		if hasConsumer {
			name = "imported source identity"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
				VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
				INSERT INTO assets(id,group_id,category,title,original_name,storage_path,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
				VALUES (1,1,'book','Lesson 1-2页','Lesson.pdf','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/Lesson.pdf',20,REPEAT('a',64),'all_groups',1,NOW(),NOW()),
				       (2,1,'book','Lesson','Lesson.pdf','team-a-resources/objects/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/Lesson.pdf',20,REPEAT('a',64),'group',1,NOW(),NOW());
				INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
				VALUES (1,1,REPEAT('a',32),'owned',NOW(),NOW()),(2,1,REPEAT('b',32),'owned',NOW(),NOW());
				INSERT INTO asset_share_grants(asset_id,owner_group_id,consumer_group_id,status,created_by,created_at)
				VALUES (1,1,NULL,'active',1,NOW());
				INSERT INTO study_tasks(id,group_id,task_type,title,content,required,enabled,sort_order,created_at,updated_at)
				VALUES (10,1,'weekly_book','Lesson','',1,1,0,NOW(),NOW());
				INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,sort_order,created_at)
				VALUES (1,10,1,'reading',7,NOW());
				INSERT INTO group_settings(group_id,settings,created_at,updated_at)
				VALUES (1,'{"task_sections":{"daily":{"path":"/api/assets/1/download","other":"/api/assets/99/download"}}}',NOW(),NOW())`)
			if hasConsumer {
				testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,visibility,created_by,created_at,updated_at)
					VALUES (3,2,'book','Imported','Lesson.pdf','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/Lesson.pdf','imported',1,NOW(),NOW());
					INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,source_asset_id,created_at,updated_at)
					VALUES (3,2,REPEAT('c',32),'imported',1,NOW(),NOW());
					INSERT INTO asset_dependencies(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,status,created_at,updated_at)
					VALUES (2,3,1,1,'active',NOW(),NOW())`)
				if _, err := asset.NewMySQLRepository(db).FindDownloadTarget(t.Context(), 2, 3); err != nil {
					t.Fatalf("fixture download: %v", err)
				}
			}
			want := 1
			if hasConsumer {
				want = 0
			}
			for _, dryRun := range []bool{true, false} {
				n, err := cleanupDuplicateResourceBindings(t.Context(), db, 1, dryRun)
				if err != nil {
					t.Fatal(err)
				}
				if n != want {
					t.Errorf("dryRun=%v removed=%d want=%d", dryRun, n, want)
				}
				wantAssetID := uint64(1)
				if !hasConsumer && !dryRun {
					wantAssetID = 2
				}
				var linkedAssetID uint64
				var usageType string
				var sortOrder int
				if err := db.QueryRow(`SELECT asset_id,usage_type,sort_order FROM task_assets WHERE group_id=1 AND task_id=10`).Scan(
					&linkedAssetID,
					&usageType,
					&sortOrder,
				); err != nil {
					t.Fatal(err)
				}
				if linkedAssetID != wantAssetID || usageType != "reading" || sortOrder != 7 {
					t.Fatalf("dryRun=%v task asset = (%d,%q,%d), want (%d,reading,7)",
						dryRun, linkedAssetID, usageType, sortOrder, wantAssetID)
				}
				var settingsPath string
				if err := db.QueryRow(`SELECT JSON_UNQUOTE(JSON_EXTRACT(settings,'$.task_sections.daily.path'))
					FROM group_settings WHERE group_id=1`).Scan(&settingsPath); err != nil {
					t.Fatal(err)
				}
				wantSettingsPath := "/api/assets/1/download"
				if !hasConsumer && !dryRun {
					wantSettingsPath = "/api/assets/2/download"
				}
				if settingsPath != wantSettingsPath {
					t.Fatalf("dryRun=%v settings path = %q, want %q", dryRun, settingsPath, wantSettingsPath)
				}
			}
			if hasConsumer {
				if _, err := asset.NewMySQLRepository(db).FindDownloadTarget(t.Context(), 2, 3); err != nil {
					t.Fatalf("cleanup broke existing import: %v", err)
				}
				var dependencies, grants int
				if err := db.QueryRow(`SELECT COUNT(*) FROM asset_dependencies WHERE provider_asset_id=1 AND status='active'`).Scan(&dependencies); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT COUNT(*) FROM asset_share_grants WHERE asset_id=1 AND status='active'`).Scan(&grants); err != nil {
					t.Fatal(err)
				}
				if dependencies != 1 || grants != 1 {
					t.Fatalf("authority changed: dependencies=%d grants=%d", dependencies, grants)
				}
			}
		})
	}

	t.Run("settings failure rolls back duplicate cleanup", func(t *testing.T) {
		db := testdb.Open(t)
		testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
			VALUES (1,'a','A',NOW(),NOW());
			INSERT INTO assets(id,group_id,category,title,original_name,storage_path,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
			VALUES (1,1,'book','Lesson 1-2页','Lesson.pdf','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/Lesson.pdf',20,REPEAT('a',64),'all_groups',1,NOW(),NOW()),
			       (2,1,'book','Lesson','Lesson.pdf','team-a-resources/objects/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/Lesson.pdf',20,REPEAT('a',64),'group',1,NOW(),NOW());
			INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,created_at,updated_at)
			VALUES (1,1,REPEAT('a',32),'owned',NOW(),NOW()),(2,1,REPEAT('b',32),'owned',NOW(),NOW());
			INSERT INTO study_tasks(id,group_id,task_type,title,content,required,enabled,sort_order,created_at,updated_at)
			VALUES (10,1,'weekly_book','Lesson','',1,1,0,NOW(),NOW());
			INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,sort_order,created_at)
			VALUES (1,10,1,'reading',7,NOW());
			INSERT INTO group_settings(group_id,settings,created_at,updated_at)
			VALUES (1,'{"task_sections":{"daily":{"path":"/api/assets/1/download"}}}',NOW(),NOW());
			CREATE TRIGGER fail_group_settings_update BEFORE UPDATE ON group_settings
			FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='settings update failed'`)

		if _, err := cleanupDuplicateResourceBindings(t.Context(), db, 1, false); err == nil {
			t.Fatal("cleanupDuplicateResourceBindings() error = nil, want settings update error")
		}

		var linkedAssetID uint64
		if err := db.QueryRow(`SELECT asset_id FROM task_assets WHERE group_id=1 AND task_id=10`).Scan(&linkedAssetID); err != nil {
			t.Fatal(err)
		}
		if linkedAssetID != 1 {
			t.Fatalf("task asset after rollback = %d, want 1", linkedAssetID)
		}
		var active int
		if err := db.QueryRow(`SELECT deleted_at IS NULL FROM asset_bindings WHERE group_id=1 AND asset_id=1`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active != 1 {
			t.Fatal("duplicate binding was retired despite settings update failure")
		}
	})
}
