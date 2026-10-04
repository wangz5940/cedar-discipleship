//go:build integration

package checkin_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/checkin"
	"agp/backend/internal/testdb"
)

type simultaneousWeeklyRepository struct {
	checkin.Repository
	ready chan struct{}
	start <-chan struct{}
}

func (r *simultaneousWeeklyRepository) wait(ctx context.Context) (uint64, error) {
	r.ready <- struct{}{}
	select {
	case <-r.start:
		return 0, sql.ErrNoRows
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (r *simultaneousWeeklyRepository) FindExistingWeeklyTask(ctx context.Context, _, _, _, _ uint64, _ string) (uint64, error) {
	return r.wait(ctx)
}

func (r *simultaneousWeeklyRepository) FindExistingWeeklyBook(ctx context.Context, _, _, _, _ uint64, _, _ string) (uint64, error) {
	return r.wait(ctx)
}

func TestConcurrentWeeklyCompletionUsesOneIdentity(t *testing.T) {
	for _, kind := range []string{"weekly_book", "weekly_verse", "weekly_outline", "weekly_video", "carried_video"} {
		t.Run(kind, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
				VALUES (1,'member','Member','member',NOW(),NOW());
				INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
				VALUES (1,1,'2026-09-21','2026-09-27',NOW(),NOW()),(2,1,'2026-09-28','2026-10-04',NOW(),NOW())`)
			taskType := kind
			if kind == "carried_video" {
				taskType = "weekly_video"
			}
			testdb.Exec(t, db, `INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
				VALUES (1,1,1,?,'Title',NOW(),NOW()),(2,1,2,?,'Title',NOW(),NOW())`, taskType, taskType)
			if taskType == "weekly_video" {
				testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
					VALUES (1,1,'video','Title','a.mp4','a',1,NOW(),NOW());
					INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,created_at)
					VALUES (1,1,1,'video',NOW()),(1,2,1,'video',NOW())`)
			}
			gate := make(chan struct{})
			repo := &simultaneousWeeklyRepository{
				Repository: checkin.NewMySQLRepository(db), ready: make(chan struct{}, 2), start: gate,
			}
			service := checkin.NewService(repo)
			type result struct {
				id       uint64
				existing bool
				err      error
			}
			results := make(chan result, 2)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for i, date := range []string{"2026-09-22", "2026-09-23"} {
				record := checkin.Record{GroupID: 1, UserID: 1, TaskID: 1, WeekID: 1, TaskType: taskType, Part: "Title", Detail: "Title", LogicalDate: date}
				if kind == "carried_video" && i == 1 {
					record.TaskID, record.WeekID, record.LogicalDate = 2, 2, "2026-09-29"
				}
				go func() {
					id, existing, err := service.Create(ctx, &record, 1)
					results <- result{id, existing, err}
				}()
			}
			for range 2 {
				select {
				case <-repo.ready:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			close(gate)
			first, second := <-results, <-results
			if first.err != nil || second.err != nil || first.id != second.id || first.existing == second.existing {
				t.Errorf("concurrent results=%+v %+v", first, second)
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records WHERE deleted_at IS NULL`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Errorf("active completions=%d want=1", count)
			}
			if errors.Is(first.err, context.DeadlineExceeded) || errors.Is(second.err, context.DeadlineExceeded) {
				t.Fatal("weekly write deadlocked")
			}
		})
	}
}

func TestConcurrentIndependentWeeklyVerseAcrossDates(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'verse','Verse','verse',NOW(),NOW())`)
	repo := checkin.NewMySQLRepository(db)
	type result struct {
		id       uint64
		existing bool
		err      error
	}
	results := make(chan result, 8)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	for i := range 8 {
		record := checkin.Record{GroupID: 1, UserID: 1, TaskType: "daily_verse", Detail: "约3:16",
			LogicalDate: []string{"2026-09-30", "2026-10-01"}[i%2], PeriodStart: "2026-09-29", PeriodEnd: "2026-10-05"}
		go func() {
			<-start
			id, existing, err := repo.Create(ctx, &record, 1)
			results <- result{id, existing, err}
		}()
	}
	close(start)
	var id uint64
	created := 0
	for range 8 {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		if id == 0 {
			id = got.id
		}
		if got.id != id {
			t.Fatalf("different identities: %d and %d", id, got.id)
		}
		if !got.existing {
			created++
		}
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records WHERE group_id=1 AND user_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if created != 1 || count != 1 {
		t.Fatalf("created=%d rows=%d", created, count)
	}
}
