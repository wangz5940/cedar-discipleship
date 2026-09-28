//go:build integration

package server

import (
	"context"
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/backup"
	"agp/backend/internal/learning"
	"agp/backend/internal/statistics"
	"agp/backend/internal/testdb"
)

func TestDailySummaryExport(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		custom    bool
		weekly    string
		expected  string
		completed string
		rate      string
	}{
		{name: "separate", mode: "separate", expected: "4", completed: "2", rate: "50.00%"},
		{name: "combined", mode: "combined", expected: "2", completed: "1", rate: "50.00%"},
		{name: "custom hole", mode: "separate", custom: true, expected: "0", completed: "0", rate: "0.00%"},
		{name: "retired aggregate input", custom: true, weekly: "aggregate", expected: "4", completed: "1", rate: "25.00%"},
		{name: "two books", custom: true, weekly: "books", expected: "4", completed: "1", rate: "25.00%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testdb.Open(t)
			testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
				VALUES (1,'first','First','first',NOW(),NOW()),(2,'second','Second','second',NOW(),NOW()),
				       (3,'former','Former','former',NOW(),NOW());
				INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
				VALUES (1,1,'First',1,NOW(),NOW(),NOW()),(1,2,'Second',1,NOW(),NOW(),NOW()),
				       (1,3,'Former',0,NOW(),NOW(),NOW())`)
			a := &app{
				db: db, location: time.UTC,
				learning:   learning.NewService(learning.NewMySQLRepository(db)),
				backups:    backup.NewService(backup.NewMySQLRepository(db)),
				statistics: statistics.NewService(statistics.NewMySQLRepository(db)),
			}
			daily := map[string]any{"checkin_mode": tc.mode}
			if tc.custom {
				daily["devotion"] = map[string]any{"plan_mode": "custom", "plans": []any{}}
				daily["scripture"] = map[string]any{"enabled": false}
			}
			if err := a.learning.SaveLearningConfig(t.Context(), 1, map[string]any{
				"task_sections": map[string]any{"daily": daily},
			}); err != nil {
				t.Fatal(err)
			}
			var weekID any
			if tc.weekly != "" {
				id, err := a.learning.SaveWeek(t.Context(), 1, 0, learning.WeekInput{
					StartDate: "2026-09-21", EndDate: "2026-09-27", Title: "Week",
					BookEnabled: true, WeeklyCheckin: tc.weekly == "aggregate",
					Readings: []learning.TaskBinding{{Title: "A"}, {Title: "B"}},
				}, false, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				weekID = id
			}
			for _, userID := range []int{1, 3} {
				for _, taskType := range []string{
					"daily_devotion", "daily_scripture", "weekly_checkin", "weekly_book",
					"weekly_video", "weekly_verse", "weekly_outline",
				} {
					part := ""
					if taskType == "weekly_book" {
						part = "A"
					}
					_, err := db.Exec(`INSERT INTO checkin_records
						(group_id,user_id,week_id,task_type,logical_date,checkin_time,part,detail,created_by,created_at,updated_at)
						VALUES (1,?,?,?,'2026-09-22',NOW(),?,'A',1,NOW(),NOW())`, userID, weekID, taskType, part)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			req := httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
			res := httptest.NewRecorder()
			a.handleAdminExportDailySummaryCSV(res, req)
			if res.Code != 200 {
				t.Fatalf("export: %d %s", res.Code, res.Body)
			}
			rows, err := csv.NewReader(strings.NewReader(res.Body.String())).ReadAll()
			if err != nil || len(rows) != 2 {
				t.Fatalf("csv: %v %v", rows, err)
			}
			columns := map[string]string{}
			for i, header := range rows[0] {
				columns[header] = rows[1][i]
			}
			for header, want := range map[string]string{
				"日期": "2026-09-22", "组内有效成员数": "2", "当日打卡人数": "2", "总打卡数": "14",
				"灵修数": "2", "读经数": "2", "整周签到数": "2", "读物数": "2",
				"视频数": "2", "背经数": "2", "提纲数": "2",
				"当前成员应完成任务数": tc.expected, "当前成员已完成任务数": tc.completed, "完成率": tc.rate,
			} {
				if got := columns[header]; got != want {
					t.Errorf("%s=%q want %q", header, got, want)
				}
			}
		})
	}
}
