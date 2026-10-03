//go:build integration

package checkin_test

import (
	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
	"errors"
	"testing"
	"time"
)

func TestVerseCadenceSaveCheckinAndReload(t *testing.T) {
	for _, mode := range []string{"", "weekly", "daily"} {
		t.Run("mode="+mode, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at) VALUES(1,'verse','Verse',NOW(),NOW());
    INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(1,'member','Member','member',NOW(),NOW())`)
			service := learning.NewService(learning.NewMySQLRepository(db))
			input := learning.WeekInput{StartDate: "2026-09-21", EndDate: "2026-09-27", VerseEnabled: true, VerseMode: mode, VerseRef: "罗马书 8:1", ReciteText: "如今那些在基督耶稣里的"}
			weekID, err := service.SaveWeek(t.Context(), 1, 0, input, false, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			weeks, err := service.ListWeekInputs(t.Context(), 1)
			if err != nil {
				t.Fatal(err)
			}
			wantMode := "weekly"
			wantType := "weekly_verse"
			if mode == "daily" {
				wantMode = "daily"
				wantType = "daily_verse"
			}
			if len(weeks) != 1 || weeks[0].VerseMode != wantMode {
				t.Fatalf("saved weeks=%+v", weeks)
			}
			tasks, err := service.WeekTasks(t.Context(), 1, weekID)
			if err != nil {
				t.Fatal(err)
			}
			taskID := tasks[0]["id"].(uint64)
			checkins := checkin.NewService(checkin.NewMySQLRepository(db))
			record := checkin.Record{GroupID: 1, UserID: 1, WeekID: weekID, TaskID: taskID, TaskType: wantType, LogicalDate: "2026-09-22"}
			first, existing, err := checkins.Create(t.Context(), &record, 1)
			if err != nil || existing {
				t.Fatalf("first=%d existing=%v err=%v", first, existing, err)
			}
			same, existing, err := checkins.Create(t.Context(), &record, 1)
			if err != nil || !existing || same != first {
				t.Fatalf("duplicate=%d existing=%v err=%v", same, existing, err)
			}
			settings := map[string]any{"task_sections": map[string]any{"daily": map[string]any{"devotion": map[string]any{"enabled": false}, "scripture": map[string]any{"enabled": false}}}}
			for _, date := range []string{"2026-09-22", "2026-09-23"} {
				hub, err := service.TodayHub(t.Context(), 1, 1, date, settings, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				want := mode != "daily" || date == "2026-09-22"
				if len(hub.Tasks) != 1 || hub.Tasks[0].Completed != want {
					t.Fatalf("date=%s hub=%+v", date, hub)
				}
			}
			record.LogicalDate = "2026-09-23"
			second, existing, err := checkins.Create(t.Context(), &record, 1)
			if err != nil || existing != (mode != "daily") || (second == first) != (mode != "daily") {
				t.Fatalf("next day=%d existing=%v err=%v", second, existing, err)
			}
			record.LogicalDate = "2026-09-28"
			if _, _, err := checkins.Create(t.Context(), &record, 1); !errors.Is(err, checkin.ErrInvalidWeeklyTarget) {
				t.Fatalf("out of week=%v", err)
			}
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			_, backup, err := learning.BackupLearningDataTx(t.Context(), tx, 1)
			_ = tx.Rollback()
			if err != nil || len(backup) != 1 || backup[0].VerseMode != wantMode {
				t.Fatalf("backup=%+v err=%v", backup, err)
			}
			// Editing a populated week retains its explicit force-confirmation boundary.
			if _, err := service.SaveWeek(t.Context(), 1, weekID, weeks[0], false, time.Now()); !errors.Is(err, learning.ErrWeekHasCheckins) {
				t.Fatalf("expected protected week: %v", err)
			}
			// Replacing tasks in the same mode must not lose the completion identity.
			if _, err := service.SaveWeek(t.Context(), 1, weekID, weeks[0], true, time.Now()); err != nil {
				t.Fatal(err)
			}
			hub, err := service.TodayHub(t.Context(), 1, 1, "2026-09-22", settings, time.Now())
			if err != nil || !hub.Tasks[0].Completed {
				t.Fatalf("replacement hub=%+v err=%v", hub, err)
			}
			input.VerseMode = "daily"
			if mode == "daily" {
				input.VerseMode = "weekly"
			}
			if _, err := service.SaveWeek(t.Context(), 1, weekID, input, true, time.Now()); err != nil {
				t.Fatal(err)
			}
			hub, err = service.TodayHub(t.Context(), 1, 1, "2026-09-22", settings, time.Now())
			if err != nil || hub.Tasks[0].Completed {
				t.Fatalf("old cadence completed new cadence: %+v err=%v", hub, err)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records WHERE deleted_at IS NULL`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			wantCount := 1
			if mode == "daily" {
				wantCount = 2
			}
			if count != wantCount {
				t.Fatalf("lost history: %d", count)
			}
		})
	}
}
