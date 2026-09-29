//go:build integration

package backup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"agp/backend/internal/asset"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestBackupPreservesAssetAuthority(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,visibility,created_by,created_at,updated_at)
		VALUES (1,1,'video','Source','a.mp4','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/a.mp4','selected_groups',1,NOW(),NOW()),
		       (2,2,'video','Imported','a.mp4','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/a.mp4','imported',1,NOW(),NOW()),
		       (3,2,'video','Private','b.mp4','team-b-resources/objects/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/b.mp4','group',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,source_asset_id,created_at,updated_at)
		VALUES (1,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','owned',NULL,NOW(),NOW()),
		       (2,2,'cccccccccccccccccccccccccccccccc','imported',1,NOW(),NOW()),
		       (3,2,'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','owned',NULL,NOW(),NOW());
		INSERT INTO asset_share_grants(asset_id,owner_group_id,consumer_group_id,status,created_by,created_at,revoked_by,revoked_at)
		VALUES (1,1,2,'revoked',1,NOW(),1,NOW()),(3,2,NULL,'revoked',1,NOW(),1,NOW());
		INSERT INTO asset_dependencies(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,status,created_at,updated_at)
		VALUES (2,2,1,1,'active',NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	t.Run("existing import and private resource", func(t *testing.T) {
		assets, err := repo.BackupAssets(t.Context(), 2)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := repo.importBackupAssetsTx(t.Context(), tx, 2, 1, assets, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		var kind string
		var source uint64
		if err := db.QueryRow(`SELECT asset_kind,COALESCE(source_asset_id,0) FROM asset_bindings WHERE asset_id=2`).Scan(&kind, &source); err != nil {
			t.Fatal(err)
		}
		if kind != "imported" || source != 1 {
			t.Errorf("import became %q source=%d", kind, source)
		}
		var active int
		if err := db.QueryRow(`SELECT COUNT(*) FROM asset_share_grants WHERE status='active'`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active != 0 {
			t.Errorf("restore activated %d grants", active)
		}
	})
	t.Run("unregistered path rejected", func(t *testing.T) {
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		_, err = repo.importBackupAssetsTx(t.Context(), tx, 2, 1, []Asset{{
			ID: 99, Category: "video", Title: "Forged", OriginalName: "c.mp4",
			StoragePath: "team-a-resources/objects/dddddddddddddddddddddddddddddddd/c.mp4",
		}}, time.Now())
		if err == nil {
			t.Fatal("unregistered path was accepted as owned")
		}
	})
	t.Run("startup does not broaden grants", func(t *testing.T) {
		testdb.Apply(t, db, "007_resource_sharing.sql")
		var active int
		if err := db.QueryRow(`SELECT COUNT(*) FROM asset_share_grants WHERE status='active'`).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if active != 0 {
			t.Errorf("startup activated %d grants", active)
		}
		var visibility string
		if err := db.QueryRow("SELECT visibility FROM assets WHERE id=3").Scan(&visibility); err != nil {
			t.Fatal(err)
		}
		if visibility != "group" {
			t.Errorf("private asset became %q", visibility)
		}
	})
}

func TestBackupMembersSingleConnection(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'leader','Leader','leader',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'Leader',NOW(),NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at)
		VALUES (1,1,'group_leader',NOW())`)
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	members, err := NewMySQLRepository(db).BackupMembers(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0].Active == nil || !*members[0].Active ||
		len(members[0].Roles) != 1 || members[0].Roles[0] != "group_leader" {
		t.Fatalf("backup roles changed: %+v", members)
	}
}

func TestLocalBackupSnapshotPreservesLearningData(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW());
		INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES (1,'{"task_sections":{"daily":{"mode":"automatic"}}}',NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	input := learning.WeekInput{
		StartDate:      "2026-09-21",
		EndDate:        "2026-09-27",
		Title:          "整周学习",
		VerseRef:       "约 1:1",
		ReciteText:     "太初有道",
		BookEnabled:    true,
		WeeklyCheckin:  true,
		VideoEnabled:   true,
		VerseEnabled:   true,
		OutlineEnabled: true,
		Readings:       []learning.TaskBinding{{Title: "本周读物", URL: "https://example.org/book.pdf"}},
		Videos:         []learning.TaskBinding{{Title: "本周视频", URL: "https://example.org/video.mp4"}},
		Outline:        learning.TaskBinding{Title: "本周讲义", URL: "https://example.org/outline.pdf"},
	}
	if err := repo.ReplaceStudyWeeks(t.Context(), 1, []learning.WeekInput{input}, time.Now()); err != nil {
		t.Fatal(err)
	}
	wantWeeks, err := learning.NewService(learning.NewMySQLRepository(db)).ListWeekInputs(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := NewService(repo).LocalBackup(t.Context(), 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payload.Weeks, wantWeeks) {
		t.Fatalf("backup weeks = %+v, want %+v", payload.Weeks, wantWeeks)
	}
	sections, ok := payload.Settings["task_sections"].(map[string]any)
	if !ok {
		t.Fatalf("backup settings changed: %+v", payload.Settings)
	}
	daily, ok := sections["daily"].(map[string]any)
	if !ok || daily["mode"] != "automatic" {
		t.Fatalf("backup settings changed: %+v", payload.Settings)
	}
}

func TestBackupImportsOnlyAuthorizedRegisteredSources(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW()),(3,'c','C',NOW(),NOW())`)
	resources := asset.NewMySQLRepository(db)
	path := "team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/a.mp4"
	sourceID, err := resources.Create(t.Context(), &asset.Asset{
		GroupID: 1, Category: "video", Title: "Source", OriginalName: "a.mp4",
		StoragePath: path, Visibility: "all_groups",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewMySQLRepository(db)
	item := Asset{ID: sourceID, Category: "video", Title: "Source", OriginalName: "a.mp4", StoragePath: path}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	mapped, err := repo.importBackupAssetsTx(t.Context(), tx, 2, 1, []Asset{item}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	importedID := mapped[sourceID]
	imported, err := resources.FindByID(t.Context(), 2, importedID)
	if err != nil || imported.AssetKind != asset.AssetKindImported || imported.SourceAssetID != sourceID {
		t.Fatalf("authorized import = %+v, err=%v", imported, err)
	}
	if _, err := resources.FindDownloadTarget(t.Context(), 2, importedID); err != nil {
		t.Fatalf("authorized source cannot be downloaded: %v", err)
	}
	testdb.Exec(t, db, `UPDATE asset_share_grants SET status='revoked',revoked_at=NOW() WHERE asset_id=?`, sourceID)
	if _, err := resources.FindDownloadTarget(t.Context(), 2, importedID); err == nil {
		t.Fatal("revoked imported source still downloadable")
	}
	tx2, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback()
	if _, err := repo.importBackupAssetsTx(t.Context(), tx2, 3, 1, []Asset{item}, time.Now()); err == nil {
		t.Fatal("revoked source was imported by another group")
	}
}

func TestInvalidBackupRollsBackMemberChanges(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'admin','Admin','admin',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'Admin',NOW(),NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at)
		VALUES (1,1,'group_admin',NOW())`)
	payload := Payload{
		Members: []Member{{Username: "new-member", DisplayName: "New", NamePinyin: "new"}},
		Assets:  []Asset{{ID: 9, StoragePath: "unregistered/private.mp4"}},
	}
	if err := NewMySQLRepository(db).ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err == nil {
		t.Fatal("invalid backup was accepted")
	}
	var members, admins, createdUsers int
	if err := db.QueryRow("SELECT COUNT(*) FROM group_members WHERE group_id=1").Scan(&members); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM user_group_roles WHERE group_id=1 AND role='group_admin'").Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username='new-member'").Scan(&createdUsers); err != nil {
		t.Fatal(err)
	}
	if members != 1 || admins != 1 || createdUsers != 0 {
		t.Fatalf("partial restore escaped rollback: members=%d admins=%d new users=%d", members, admins, createdUsers)
	}
}
