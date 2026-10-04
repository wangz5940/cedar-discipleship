//go:build integration

package checkin_test

import (
	"testing"

	"agp/backend/internal/checkin"
	"agp/backend/internal/testdb"
)

func TestIndependentWeeklyVerseChangedReferencePreservesBothCompletions(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "period_identity"
		if legacy {
			name = "legacy_empty_part"
		}
		t.Run(name, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
				VALUES (1,'verse','Verse','verse',NOW(),NOW())`)
			service := checkin.NewService(checkin.NewMySQLRepository(db))
			original := checkin.Record{GroupID: 1, UserID: 1, TaskType: "daily_verse", Detail: "约3:16",
				LogicalDate: "2026-10-01", PeriodStart: "2026-09-29", PeriodEnd: "2026-10-05"}
			if legacy {
				original.PeriodStart, original.PeriodEnd = "", ""
			}
			firstID, existed, err := service.Create(t.Context(), &original, 1)
			if err != nil || existed {
				t.Fatalf("original completion: %d %v %v", firstID, existed, err)
			}
			original.PeriodStart, original.PeriodEnd = "2026-09-29", "2026-10-05"
			if id, existed, err := service.Create(t.Context(), &original, 1); err != nil || !existed || id != firstID {
				t.Fatalf("original retry: %d %v %v", id, existed, err)
			}
			replacement := original
			replacement.Detail = "诗23:1"
			secondID, existed, err := service.Create(t.Context(), &replacement, 1)
			if err != nil || existed || secondID == firstID {
				t.Fatalf("changed reference completion: %d %v %v", secondID, existed, err)
			}
			for _, date := range []string{"2026-10-01", "2026-10-02"} {
				replacement.LogicalDate = date
				if id, existed, err := service.Create(t.Context(), &replacement, 1); err != nil || !existed || id != secondID {
					t.Fatalf("changed reference retry on %s: %d %v %v", date, id, existed, err)
				}
			}
			var count, originalCount, replacementCount int
			if err := db.QueryRow(`SELECT COUNT(*),SUM(detail='约3:16'),SUM(detail='诗23:1')
				FROM checkin_records WHERE group_id=1 AND user_id=1 AND logical_date='2026-10-01' AND deleted_at IS NULL`).
				Scan(&count, &originalCount, &replacementCount); err != nil {
				t.Fatal(err)
			}
			if count != 2 || originalCount != 1 || replacementCount != 1 {
				t.Fatalf("completion history: total=%d original=%d replacement=%d", count, originalCount, replacementCount)
			}
		})
	}
}
