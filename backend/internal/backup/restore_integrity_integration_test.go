//go:build integration

package backup

import (
	"testing"
	"time"

	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestImportLocalBackupPreservesRecitationHistoryAndAdvancesConfigRevision(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'admin','Admin','admin',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'Admin',NOW(),NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES (1,1,'member',1,NOW(),NOW());
		INSERT INTO group_settings(group_id,settings,created_at,updated_at)
		VALUES (1,'{"_revision":5}',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,title,verse_ref,created_at,updated_at)
		VALUES (7,1,'2026-09-28','2026-10-04','本周','约3:16',NOW(),NOW());
		INSERT INTO recite_attempts(group_id,user_id,week_id,verse_ref,logical_date,blank_percent,blank_count,correct_count,accuracy,score,attempt_no,created_at)
		VALUES (1,1,7,'约3:16','2026-10-01',50,1,1,100,100,1,NOW()),
		       (2,1,7,'约3:16','2026-10-01',50,1,1,100,100,1,NOW())`)
	repo := NewMySQLRepository(db)
	payload, err := NewService(repo).LocalBackup(t.Context(), 1, time.Now().Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	// A backup's old revision must not make an already-open page current again.
	payload.Settings["_revision"] = 0
	if err := repo.ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	var newWeek, historyWeek, otherWeek uint64
	if err := db.QueryRow(`SELECT id FROM study_weeks WHERE group_id=1`).Scan(&newWeek); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT week_id FROM recite_attempts WHERE group_id=1`).Scan(&historyWeek); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT week_id FROM recite_attempts WHERE group_id=2`).Scan(&otherWeek); err != nil {
		t.Fatal(err)
	}
	if newWeek == 7 || historyWeek != newWeek || otherWeek != 7 {
		t.Fatalf("restored week=%d history=%d other group=%d", newWeek, historyWeek, otherWeek)
	}
	config, err := learning.NewMySQLRepository(db).LearningConfig(t.Context(), 1)
	if err != nil || config["_revision"] != float64(6) {
		t.Fatalf("config=%v err=%v", config, err)
	}
	if err := repo.ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT id FROM study_weeks WHERE group_id=1`).Scan(&newWeek); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT week_id FROM recite_attempts WHERE group_id=1`).Scan(&historyWeek); err != nil {
		t.Fatal(err)
	}
	if historyWeek != newWeek {
		t.Fatalf("history lost on repeated restore: week=%d history=%d", newWeek, historyWeek)
	}
}

func TestImportLocalBackupIgnoresLegacyFeedbackPayload(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'admin','Admin','admin',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,1,'Admin',NOW(),NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES (1,1,'member',1,NOW(),NOW());
		INSERT INTO feedbacks(group_id,user_id,name,contact,message,page,user_agent,created_at,updated_at)
		VALUES (1,1,'','','existing private feedback','','',NOW(),NOW())`)

	payload := Payload{
		Members:   []Member{{Username: "admin", DisplayName: "Admin"}},
		Feedbacks: []Feedback{{Username: "admin", Message: "legacy backup feedback"}},
	}
	if err := NewMySQLRepository(db).ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	var count int
	var message string
	if err := db.QueryRow(`SELECT COUNT(*),MAX(message) FROM feedbacks WHERE group_id=1`).Scan(&count, &message); err != nil {
		t.Fatal(err)
	}
	if count != 1 || message != "existing private feedback" {
		t.Fatalf("feedback changed during group restore: count=%d message=%q", count, message)
	}
}

func TestImportLocalBackupIntegrity(t *testing.T) {
	t.Run("existing global profiles and group names", func(t *testing.T) {
		db := testdb.Open(t)
		testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
			VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
			INSERT INTO users(id,username,display_name,name_pinyin,is_super_admin,created_at,updated_at)
			VALUES (1,'same','Same','same',0,NOW(),NOW()),
			       (2,'other','Other','other',0,NOW(),NOW()),
			       (3,'super','Super','super',1,NOW(),NOW());
			INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
			VALUES (1,1,'Same',NOW(),NOW(),NOW()),(2,2,'Other group',NOW(),NOW(),NOW())`)
		testdb.Apply(t, db, "015_tenants.sql")
		testdb.Exec(t, db, `INSERT INTO tenant_members(tenant_id,user_id,role,created_at,updated_at) VALUES(1,3,'member',NOW(),NOW())`)
		payload := Payload{Members: []Member{
			{Username: "same", DisplayName: "Local same", NamePinyin: "changed"},
			{Username: "other", DisplayName: "Local other", NamePinyin: "changed"},
			{Username: "super", DisplayName: "Local super", NamePinyin: "changed"},
			{Username: "new", DisplayName: "New", NamePinyin: "new"},
		}}
		repo := NewMySQLRepository(db)
		if err := repo.ImportLocalBackup(t.Context(), 1, 3, payload, time.Now()); err != nil {
			t.Fatal(err)
		}
		for _, want := range []struct{ username, global, pinyin, local string }{
			{"same", "Same", "same", "Local same"},
			{"other", "Other", "other", "Local other"},
			{"super", "Super", "super", "Local super"},
			{"new", "New", "new", "New"},
		} {
			var global, pinyin, local string
			if err := db.QueryRow(`SELECT u.display_name,u.name_pinyin,m.member_name
				FROM users u JOIN group_members m ON m.user_id=u.id
				WHERE u.username=? AND m.group_id=1`, want.username).Scan(&global, &pinyin, &local); err != nil {
				t.Fatal(err)
			}
			if global != want.global || pinyin != want.pinyin || local != want.local {
				t.Errorf("%s profile=%q/%q local=%q, want %+v", want.username, global, pinyin, local, want)
			}
		}
		// A second export/import must also preserve the distinct group name.
		members, err := repo.BackupMembers(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.ImportLocalBackup(t.Context(), 1, 3, Payload{Members: members}, time.Now()); err != nil {
			t.Fatal(err)
		}
		var local string
		if err := db.QueryRow(`SELECT member_name FROM group_members WHERE group_id=1 AND user_id=2`).Scan(&local); err != nil {
			t.Fatal(err)
		}
		if local != "Local other" {
			t.Errorf("group name lost on round trip: %q", local)
		}
	})
	t.Run("former member history round trip", func(t *testing.T) {
		sourceDB := testdb.Open(t)
		testdb.Exec(t, sourceDB, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
			VALUES (1,'a','A',NOW(),NOW());
			INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
			VALUES (1,'admin','Admin','admin',NOW(),NOW()),
			       (2,'former','Former','former',NOW(),NOW());
			INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
			VALUES (1,1,'Admin',1,NOW(),NOW(),NOW()),
			       (1,2,'Former',0,NOW(),NOW(),NOW());
			INSERT INTO checkin_records(group_id,user_id,logical_date,checkin_time,task_type,detail,note,created_by,created_at,updated_at)
			VALUES (1,2,'2026-09-01',NOW(),'daily_devotion','History','Keep me',1,NOW(),NOW())`)
		sourceRepo := NewMySQLRepository(sourceDB)
		payload, err := NewService(sourceRepo).LocalBackup(t.Context(), 1, time.Now().Format(time.RFC3339))
		if err != nil {
			t.Fatal(err)
		}
		if len(payload.Members) != 2 || len(payload.Checkins) != 1 {
			t.Fatalf("invalid fixture export: %+v", payload)
		}
		var former Member
		for _, member := range payload.Members {
			if member.Username == "former" {
				former = member
				break
			}
		}
		if former.Active == nil || *former.Active || former.DisplayName != "Former" || former.MemberName != "Former" {
			t.Fatalf("former member identity not exported: %+v", former)
		}

		targetDB := testdb.Open(t)
		testdb.Exec(t, targetDB, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
			VALUES (1,'a','A',NOW(),NOW());
			INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
			VALUES (1,'admin','Admin','admin',NOW(),NOW());
			INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
			VALUES (1,1,'Admin',1,NOW(),NOW(),NOW());
			INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
			VALUES (1,1,'member',1,NOW(),NOW())`)
		targetRepo := NewMySQLRepository(targetDB)
		if err := targetRepo.ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err != nil {
			t.Fatal(err)
		}
		checkins, err := targetRepo.BackupCheckins(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(checkins) != 1 || checkins[0].Username != "former" || checkins[0].Note != "Keep me" {
			t.Fatalf("history lost: %+v", checkins)
		}
		var status int
		if err := targetDB.QueryRow(`SELECT m.status
			FROM group_members m JOIN users u ON u.id=m.user_id
			WHERE m.group_id=1 AND u.username='former'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != 0 {
			t.Fatal("historical identity was reactivated")
		}
		var displayName, memberName string
		if err := targetDB.QueryRow(`SELECT u.display_name,m.member_name
			FROM group_members m JOIN users u ON u.id=m.user_id
			WHERE m.group_id=1 AND u.username='former'`).Scan(&displayName, &memberName); err != nil {
			t.Fatal(err)
		}
		if displayName != "Former" || memberName != "Former" {
			t.Fatalf("historical identity changed: display=%q member=%q", displayName, memberName)
		}
		var tenantMemberships int
		if err := targetDB.QueryRow(`SELECT COUNT(*) FROM tenant_members tm
			JOIN users u ON u.id=tm.user_id WHERE u.username='former' AND tm.status=1`).Scan(&tenantMemberships); err != nil {
			t.Fatal(err)
		}
		if tenantMemberships != 0 {
			t.Fatal("historical identity was reactivated in tenant membership")
		}
	})
	t.Run("unresolved identity rolls back entire restore", func(t *testing.T) {
		db := testdb.Open(t)
		testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
			VALUES (1,'a','A',NOW(),NOW());
			INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
			VALUES (1,'admin','Admin','admin',NOW(),NOW()),(2,'unrelated','Unrelated','unrelated',NOW(),NOW());
			INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
			VALUES (1,1,'Admin',NOW(),NOW(),NOW());
			INSERT INTO checkin_records(group_id,user_id,logical_date,checkin_time,task_type,detail,created_by,created_at,updated_at)
			VALUES (1,1,'2026-09-01',NOW(),'daily_devotion','Original',1,NOW(),NOW())`)
		testdb.Apply(t, db, "015_tenants.sql")
		repo := NewMySQLRepository(db)
		payload := Payload{
			Members:  []Member{{Username: "admin", DisplayName: "Changed"}},
			Checkins: []Checkin{{Username: "unrelated", LogicalDate: "2026-09-01", TaskType: "daily_devotion"}},
		}
		if err := repo.ImportLocalBackup(t.Context(), 1, 1, payload, time.Now()); err == nil {
			t.Fatal("accepted cross-tenant historical identity")
		}
		checkins, err := repo.BackupCheckins(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(checkins) != 1 || checkins[0].Detail != "Original" {
			t.Fatalf("failed import changed history: %+v", checkins)
		}
		var local string
		if err := db.QueryRow(`SELECT member_name FROM group_members WHERE group_id=1 AND user_id=1`).Scan(&local); err != nil {
			t.Fatal(err)
		}
		if local != "Admin" {
			t.Fatalf("member change escaped rollback: %q", local)
		}
	})
}

func TestBackupAssetsExcludesDeletedBindings(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES (1,1,'video','Active','a.mp4','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/a.mp4',1,NOW(),NOW()),
		       (2,1,'video','Deleted','b.mp4','team-a-resources/objects/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/b.mp4',1,NOW(),NOW());
		INSERT INTO asset_bindings(asset_id,group_id,resource_key,asset_kind,deleted_at,created_at,updated_at)
		VALUES (1,1,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','owned',NULL,NOW(),NOW()),
		       (2,1,'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb','owned',NOW(),NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	assets, err := repo.BackupAssets(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].ID != 1 {
		t.Errorf("backup contains inactive resources: %+v", assets)
	}
	if err := repo.ImportLocalBackup(t.Context(), 1, 1, Payload{Assets: assets}, time.Now()); err != nil {
		t.Fatalf("unchanged export cannot be restored: %v", err)
	}
	var deleted bool
	if err := db.QueryRow(`SELECT deleted_at IS NOT NULL FROM asset_bindings WHERE asset_id=2`).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("restore revived deleted resource")
	}
}
