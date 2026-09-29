//go:build integration

package checkin_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"agp/backend/internal/checkin"
	"agp/backend/internal/learning"
	"agp/backend/internal/testdb"
)

func TestVideoReplacementAndSingleConnectionTasks(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'first','First','first',NOW(),NOW()),(2,'second','Second','second',NOW(),NOW());
		INSERT INTO study_weeks(id,group_id,start_date,end_date,created_at,updated_at)
		VALUES (1,1,'2026-09-21','2026-09-27',NOW(),NOW()),(2,1,'2026-09-28','2026-10-04',NOW(),NOW());
		INSERT INTO study_tasks(id,group_id,week_id,task_type,title,created_at,updated_at)
		VALUES (1,1,1,'weekly_video','A',NOW(),NOW()),(2,1,1,'weekly_video','B',NOW(),NOW()),
		       (3,1,2,'weekly_video','A again',NOW(),NOW()),(4,1,1,'weekly_video','Legacy',NOW(),NOW());
		INSERT INTO assets(id,group_id,category,title,original_name,storage_path,created_by,created_at,updated_at)
		VALUES (1,1,'video','A','A.mp4','a',1,NOW(),NOW()),(2,1,'video','B','B.mp4','b',1,NOW(),NOW()),
		       (3,2,'video','Foreign','foreign.mp4','f',1,NOW(),NOW());
		INSERT INTO task_assets(group_id,task_id,asset_id,usage_type,sort_order,created_at)
		VALUES (1,1,1,'video',0,NOW()),(1,2,2,'video',0,NOW()),(1,3,1,'video',0,NOW()),
		       (1,2,3,'video',1,NOW())`)
	repo := checkin.NewMySQLRepository(db)
	service := checkin.NewService(repo)
	learningService := learning.NewService(learning.NewMySQLRepository(db))
	for i, date := range []string{"2026-09-22", "2026-09-23"} {
		t.Run(date, func(t *testing.T) {
			userID := uint64(i + 1)
			a := &checkin.Record{GroupID: 1, UserID: userID, TaskID: 1, WeekID: 1, TaskType: "weekly_video", LogicalDate: "2026-09-22"}
			first, existing, err := service.Create(t.Context(), a, userID)
			if err != nil || existing {
				t.Fatalf("A: id=%d existing=%v err=%v", first, existing, err)
			}
			assertVideoCompletion(t, learningService, userID, date, 2, false)
			b := &checkin.Record{GroupID: 1, UserID: userID, TaskID: 2, WeekID: 1, TaskType: "weekly_video", LogicalDate: date}
			second, existing, err := service.Create(t.Context(), b, userID)
			if err != nil || existing || second == first {
				t.Fatalf("B: id=%d existing=%v err=%v", second, existing, err)
			}
			assertVideoCompletion(t, learningService, userID, date, 2, true)
			assertVideoCompletion(t, learningService, userID, "2026-09-29", 3, true)
			again, existing, err := service.Create(t.Context(), b, userID)
			if err != nil || !existing || again != second {
				t.Fatalf("B duplicate: id=%d existing=%v err=%v", again, existing, err)
			}
			carried, err := repo.FindExistingWeeklyTask(t.Context(), 1, userID, 3, 2, "weekly_video")
			if err != nil || carried != first {
				t.Fatalf("same asset carryover: id=%d err=%v", carried, err)
			}
			if err := service.DeleteOwn(t.Context(), 1, userID, second); err != nil {
				t.Fatal(err)
			}
			assertVideoCompletion(t, learningService, userID, date, 2, false)
			if _, existing, err := service.Create(t.Context(), b, userID); err != nil || existing {
				t.Fatalf("recreate B: existing=%v err=%v", existing, err)
			}
		})
	}
	t.Run("legacy week fallback", func(t *testing.T) {
		id, err := repo.FindExistingWeeklyTask(t.Context(), 1, 1, 4, 1, "weekly_video")
		if err != nil || id == 0 {
			t.Fatalf("legacy fallback: id=%d err=%v", id, err)
		}
		_, err = repo.FindExistingWeeklyTask(t.Context(), 1, 999, 2, 1, "weekly_video")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("new user: %v", err)
		}
	})
	t.Run("tasks with one connection", func(t *testing.T) {
		db.SetMaxOpenConns(1)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		tasks, err := learning.NewMySQLRepository(db).ListTasks(ctx, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 3 || tasks[0].ID != 1 || len(tasks[0].Assets) != 1 ||
			tasks[1].ID != 2 || len(tasks[1].Assets) != 1 || tasks[1].Assets[0].ID != 2 ||
			tasks[2].ID != 4 || len(tasks[2].Assets) != 0 {
			t.Fatalf("task ordering, resources or isolation changed: %+v", tasks)
		}
	})
}

func TestDeleteReportsMissingOrOutOfScopeRecords(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `INSERT INTO checkin_records
		(id,group_id,user_id,logical_date,checkin_time,task_type,created_by,created_at,updated_at)
		VALUES
		  (1,1,10,'2026-09-22',NOW(),'daily_devotion',10,NOW(),NOW()),
		  (2,1,20,'2026-09-23',NOW(),'daily_devotion',20,NOW(),NOW())`)
	repo := checkin.NewMySQLRepository(db)

	for name, err := range map[string]error{
		"own missing":       repo.DeleteOwn(t.Context(), 1, 10, 99),
		"own other user":    repo.DeleteOwn(t.Context(), 1, 10, 2),
		"own other group":   repo.DeleteOwn(t.Context(), 2, 10, 1),
		"admin missing":     repo.DeleteAny(t.Context(), 1, 99),
		"admin other group": repo.DeleteAny(t.Context(), 2, 2),
	} {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("%s error = %v, want sql.ErrNoRows", name, err)
		}
	}

	if err := repo.DeleteOwn(t.Context(), 1, 10, 1); err != nil {
		t.Fatalf("delete own: %v", err)
	}
	if err := repo.DeleteOwn(t.Context(), 1, 10, 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("repeat delete own error = %v, want sql.ErrNoRows", err)
	}
	if err := repo.DeleteAny(t.Context(), 1, 2); err != nil {
		t.Fatalf("admin delete: %v", err)
	}
	if err := repo.DeleteAny(t.Context(), 1, 2); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("repeat admin delete error = %v, want sql.ErrNoRows", err)
	}

	var deletedCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records
		WHERE deleted_at IS NOT NULL AND active_key=id`).Scan(&deletedCount); err != nil {
		t.Fatal(err)
	}
	if deletedCount != 2 {
		t.Fatalf("deleted rows = %d, want 2", deletedCount)
	}
}

func assertVideoCompletion(t *testing.T, service *learning.Service, userID uint64, date string, taskID uint64, want bool) {
	t.Helper()
	content, err := service.TodayContent(t.Context(), 1, date, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	today, err := service.TodayHubFromContent(t.Context(), 1, userID, content)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, task := range today.Tasks {
		if task.TaskID == taskID {
			found = true
			if task.Completed != want {
				t.Fatalf("today task %d completed=%v want=%v", taskID, task.Completed, want)
			}
		}
	}
	if !found {
		t.Fatalf("today task %d missing", taskID)
	}
	completions, err := service.GroupTaskCompletionsFromContent(t.Context(), 1, content)
	if err != nil {
		t.Fatal(err)
	}
	completed := false
	for _, item := range completions.Items {
		if item.UserID == userID && item.TaskID == taskID && item.Completed {
			completed = true
		}
	}
	if completed != want {
		t.Fatalf("statistics task %d completed=%v want=%v", taskID, completed, want)
	}
}
