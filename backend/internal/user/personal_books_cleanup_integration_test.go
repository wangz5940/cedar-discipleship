//go:build integration

package user

import (
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestDeleteGroupRemovesOnlyItsPersonalBooks(t *testing.T) {
	db := testdb.Open(t)
	for _, migration := range []string{
		"003_ministry_groups.sql",
		"004_ministry_attendance.sql",
		"005_ministry_catalog_and_pins.sql",
		"006_ministry_content_deletions.sql",
		"011_refresh_sessions.sql",
		"016_refresh_session_group_version.sql",
	} {
		testdb.Apply(t, db, migration)
	}
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES(1,'deleted','Deleted',NOW(),NOW()),(2,'kept','Kept',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES(1,'reader','Reader','reader',NOW(),NOW());
		INSERT INTO personal_daily_books(group_id,user_id,logical_date,title,path,content_type,page_start,page_end)
		VALUES(1,1,'2026-10-11','Deleted book','/api/assets/1/download','pdf',1,2),
		      (2,1,'2026-10-11','Kept book','/api/assets/2/download','pdf',3,4);
		INSERT INTO personal_book_completions(group_id,user_id,logical_date,completed_at)
		VALUES(1,1,'2026-10-11',NOW()),(1,1,'2026-10-10',NOW()),(2,1,'2026-10-11',NOW())`)

	if _, err := NewService(NewMySQLRepository(db)).DeleteGroup(t.Context(), 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"personal_daily_books", "personal_book_completions"} {
		t.Run(table, func(t *testing.T) {
			for groupID, want := range map[int]int{1: 0, 2: 1} {
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE group_id=?", groupID).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != want {
					t.Errorf("group %d records = %d, want %d", groupID, count, want)
				}
			}
		})
	}
	var userCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id=1`).Scan(&userCount); err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Errorf("user count = %d, want 1", userCount)
	}
}
