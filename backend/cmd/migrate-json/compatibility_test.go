package main

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestLegacyConfigFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/learning.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]oldConfig
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for name, cfg := range fixtures {
		t.Run(name, func(t *testing.T) {
			opt := options{}
			if name == "zwmouss" {
				opt.dailyCheckinMode, opt.weeklyCheckinMode = "separate", "aggregate"
				opt.devotionMode = "date"
			}
			if err := normalizeLegacyConfig(&cfg, opt); err != nil {
				t.Fatal(err)
			}
			var sections map[string]any
			if err := json.Unmarshal(cfg.TaskSections, &sections); err != nil {
				t.Fatal(err)
			}
			daily := mapValue(sections, "daily")
			if stringValue(daily["path"]) == "" || stringValue(mapValue(daily, "devotion")["path"]) == "" {
				t.Fatal("lost original devotion path")
			}
			tasks := tasksForWeek(cfg.WeeklySchedule[0])
			counts := map[string]int{}
			for _, task := range tasks {
				counts[task.Type]++
				if task.Type == "weekly_book" && name != "zk" && !task.Optional {
					t.Fatalf("aggregate content is required: %+v", task)
				}
			}
			wantCheckin := 1
			if name == "zk" {
				wantCheckin = 0
			}
			if counts["weekly_checkin"] != wantCheckin {
				t.Fatalf("task counts = %v", counts)
			}
			if name != "zk" && counts["weekly_video"] != 0 {
				t.Fatalf("empty video task: %v", tasks)
			}
			switch name {
			case "zw1", "zwlingyi":
				scripture := mapValue(daily, "scripture")
				books := scripture["books"].([]any)
				if scripture["book"] != "撒母耳记下" || scripture["book_id"] != "10" ||
					numberValue(scripture["start_chapter"]) != 24 || numberValue(scripture["chapters_per_day"]) != 2 ||
					len(books) != 57 || mapValue(books[56], "")["book_id"] != "66" {
					t.Fatalf("scripture = %+v", scripture)
				}
				if name == "zwlingyi" && (!tasks[len(tasks)-1].Enabled || counts["weekly_book"] != 0) {
					t.Fatalf("disabled readings should not create completion tasks: %+v", tasks)
				}
			case "zwmouss":
				scripture := mapValue(daily, "scripture")
				if scripture["type"] != "checkin" || scripture["books"] != nil || scripture["book"] != nil ||
					daily["checkin_mode"] != "separate" {
					t.Fatalf("invented daily plan: %+v", daily)
				}
			case "zk":
				var content map[string]any
				if err := json.Unmarshal([]byte(tasks[0].Content), &content); err != nil {
					t.Fatal(err)
				}
				if content["reading_path"] != "/weekly_task.md" || content["source_title"] != tasks[0].Title {
					t.Fatalf("missing exact markdown source: %v", content)
				}
				if tasks[1].Content != "http://nas.restinhim.online:5777/Newtestament/L1.mp4" {
					t.Fatalf("lost external video: %+v", tasks[1])
				}
			}
			settings, err := learningSettings(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.ClassRepShares) > 0 && len(settings["class_rep_shares"]) == 0 {
				t.Fatal("lost global shares")
			}
		})
	}
}

func TestModesAreExplicitAndDoNotUseGroupNames(t *testing.T) {
	cfg := oldConfig{TaskSections: json.RawMessage(`{"daily":{},"weekly":{},"buttons":{"book1":"周任务"}}`)}
	if err := normalizeLegacyConfig(&cfg, options{groupCode: "zw1"}); err != nil {
		t.Fatal(err)
	}
	var sections map[string]any
	_ = json.Unmarshal(cfg.TaskSections, &sections)
	if mapValue(sections, "weekly")["checkin_mode"] != "per_reading" {
		t.Fatal("a label or group name alone is not an aggregate marker")
	}
	if err := normalizeLegacyConfig(&cfg, options{dailyCheckinMode: "typo"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestChineseRecordsAndExactTaskIdentity(t *testing.T) {
	var rec oldRecord
	if err := json.Unmarshal([]byte(`{"Id":7,"姓名":"fixture","打卡时间":"2026-09-20T10:00:00+08:00","是否补签":"是","每日灵修":"已撤回","每日读经":"已完成","周任务":"已完成","打卡详情":"【补签】9月18日 周任务"}`), &rec); err != nil {
		t.Fatal(err)
	}
	rows := checkinRowsForRecord(rec)
	if rec.Name != "fixture" || rec.LogicalDate != "2026-09-18" || !isRetro(rec.IsRetro) ||
		len(rows) != 2 || rows[0].TaskType != "daily_scripture" || rows[1].TaskType != "weekly_checkin" {
		t.Fatalf("record = %+v, rows = %+v", rec, rows)
	}
	candidates := []recordTask{{ID: 1, Type: "weekly_book", Title: "第一章"}, {ID: 2, Type: "weekly_book", Title: "第二章"}}
	if _, err := resolveRecordTask(checkinRow{TaskType: "weekly_book"}, candidates); err == nil {
		t.Fatal("ambiguous books must not inherit completion")
	}
	got, err := resolveRecordTask(checkinRow{TaskType: "weekly_book", Part: "第二章"}, candidates)
	if err != nil || got.TaskID != 2 {
		t.Fatalf("exact task = %+v, %v", got, err)
	}
	candidates = append(candidates, recordTask{ID: 3, Type: "weekly_checkin", Title: "本周"})
	got, err = resolveRecordTask(checkinRow{TaskType: "weekly_book", Part: "周任务"}, candidates)
	if err != nil || got.TaskID != 3 || got.TaskType != "weekly_checkin" {
		t.Fatalf("aggregate task = %+v, %v", got, err)
	}
	if _, err := resolveRecordTask(checkinRow{TaskType: "weekly_video"}, candidates); err == nil {
		t.Fatal("missing task must fail instead of inserting a nil identity")
	}
}

func TestUnmatchedWeeklyHistoryRequiresOptIn(t *testing.T) {
	row := checkinRow{TaskType: "weekly_video", Detail: "旧站周视频", Part: "第 3 周"}
	if _, err := resolveRecordTaskForImport(row, nil, false); err == nil {
		t.Fatal("default import must reject a weekly record without a configured task")
	}
	got, err := resolveRecordTaskForImport(row, nil, true)
	if err != nil || got != row || nullableID(got.TaskID) != nil {
		t.Fatalf("unmatched history must retain type/detail/part and a NULL task ID: %+v, %v", got, err)
	}
	matched, err := resolveRecordTaskForImport(row, []recordTask{{ID: 12, Type: "weekly_video", Title: "本周视频"}}, true)
	if err != nil || matched.TaskID != 12 {
		t.Fatalf("matched task must keep its identity: %+v, %v", matched, err)
	}
	ambiguous := []recordTask{{ID: 12, Type: "weekly_video"}, {ID: 13, Type: "weekly_video"}}
	if _, err := resolveRecordTaskForImport(row, ambiguous, true); err == nil {
		t.Fatal("opt-in must not discard an ambiguous task identity")
	}
}

func TestLegacyTimezoneCheckinTime(t *testing.T) {
	for _, value := range []string{"2026-04-08T04:41:20+00:00", "2026-04-08 04:41:20", "2026-04-08 04:41:20+00:00"} {
		got, err := parseTime(value)
		if err != nil || got.UTC().Format(time.RFC3339) != "2026-04-08T04:41:20Z" {
			t.Fatalf("parseTime(%q) = %v, %v", value, got, err)
		}
	}
	dateOnly, err := parseTime("2026-05-01")
	if err != nil || dateOnly.UTC().Format(time.RFC3339) != "2026-05-01T00:00:00Z" {
		t.Fatalf("date-only checkin time = %v, %v", dateOnly, err)
	}
}

func TestNoEmptyPlaceholdersAndHTMLInference(t *testing.T) {
	if tasks := tasksForWeek(oldWeek{}); len(tasks) != 0 {
		t.Fatalf("empty week has tasks: %+v", tasks)
	}
	cfg := oldConfig{TaskSections: json.RawMessage(`{}`), WeeklySchedule: []oldWeek{{
		Readings: []oldAssetRef{{Title: "external", URL: "https://example.org/read.htm"}},
	}}}
	if err := normalizeLegacyConfig(&cfg, options{}); err != nil {
		t.Fatal(err)
	}
	if cfg.WeeklySchedule[0].Readings[0].Type != "iframe" {
		t.Fatal("HTML type not inferred")
	}
}
