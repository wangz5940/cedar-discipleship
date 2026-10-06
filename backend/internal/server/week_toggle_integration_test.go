//go:build integration

package server

import (
	"agp/backend/internal/audit"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWeekTogglePreservesTasksAndHistory(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'toggle','Toggle',NOW(),NOW());
 INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'toggle','Toggle','toggle',NOW(),NOW())`)
	a := &app{db: db, location: time.UTC, learning: learning.NewService(learning.NewMySQLRepository(db)), audits: audit.NewService(audit.NewMySQLRepository(db))}
	id, err := a.learning.SaveWeek(t.Context(), 1, 0, learning.WeekInput{StartDate: "2026-10-05", EndDate: "2026-10-11", VerseEnabled: true, VerseRef: "约3:16", ReciteText: "神爱世人"}, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var taskID uint64
	if err := db.QueryRow(`SELECT id FROM study_tasks WHERE week_id=?`, id).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, fmt.Sprintf(`INSERT INTO checkin_records(id,group_id,user_id,week_id,task_id,logical_date,checkin_time,task_type,created_by,created_at,updated_at) VALUES(1,1,1,%d,%d,'2026-10-06',NOW(),'weekly_verse',1,NOW(),NOW())`, id, taskID))
	call := func(group uint64, body string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body))
		r.SetPathValue("id", fmt.Sprint(id))
		r = r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: group}))
		w := httptest.NewRecorder()
		a.handleAdminStudyWeekEnabled(w, r)
		if w.Code != want {
			t.Fatalf("status=%d body=%s", w.Code, w.Body)
		}
	}
	call(1, `{"field":"verse_enabled","enabled":false}`, http.StatusOK)
	call(1, `{"field":"verse_enabled","enabled":true}`, http.StatusOK)
	call(1, `{"field":"verse_enabled","enabled":true}`, http.StatusOK)
	call(2, `{"field":"verse_enabled","enabled":false}`, http.StatusNotFound)
	call(1, `{"field":"title","enabled":false}`, http.StatusBadRequest)
	var tasks, records int
	var restoredID uint64
	var enabled bool
	if err := db.QueryRow(`SELECT COUNT(*),MIN(id),MIN(enabled) FROM study_tasks WHERE week_id=?`, id).Scan(&tasks, &restoredID, &enabled); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records WHERE id=1 AND task_id=? AND deleted_at IS NULL`, taskID).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || restoredID != taskID || !enabled || records != 1 {
		t.Fatalf("tasks=%d id=%d enabled=%v records=%d", tasks, restoredID, enabled, records)
	}
	// A previously disabled plan may have persisted scripture but no task yet.
	id, err = a.learning.SaveWeek(t.Context(), 1, 0, learning.WeekInput{StartDate: "2026-10-12", EndDate: "2026-10-18", VerseRef: "罗8:1", ReciteText: "如今那些在基督耶稣里的"}, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call(1, `{"field":"verse_enabled","enabled":true}`, http.StatusOK)
	call(1, `{"field":"verse_enabled","enabled":true}`, http.StatusOK)
	if err := db.QueryRow(`SELECT COUNT(*) FROM study_tasks WHERE week_id=? AND task_type='weekly_verse' AND enabled=1`, id).Scan(&tasks); err != nil || tasks != 1 {
		t.Fatalf("enabled tasks=%d err=%v", tasks, err)
	}
}
