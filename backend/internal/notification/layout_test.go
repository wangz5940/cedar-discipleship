package notification

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNotificationLayout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		daily   bool
		entries []Entry
		want    string
	}{
		{
			name:  "daily title and one line per person",
			daily: true,
			entries: []Entry{
				{RecordID: 1, UserID: 1, Name: "张三", TaskType: "daily_devotion"},
				{RecordID: 2, UserID: 2, Name: "李四", TaskType: "daily_devotion"},
				{RecordID: 3, UserID: 3, Name: "王五", TaskType: "daily_devotion"},
				{RecordID: 4, UserID: 4, Name: "赵六", TaskType: "daily_devotion"},
			},
			want: "每日灵修\n1 张三\n2 李四\n3 王五\n4 【新】赵六",
		},
		{
			name: "weekly title and grouped content",
			entries: []Entry{
				{RecordID: 1, UserID: 1, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
				{RecordID: 2, UserID: 1, Name: "张三", TaskType: "weekly_book", BookName: "史剧"},
				{RecordID: 3, UserID: 2, Name: "李四", TaskType: "weekly_book", BookName: "史剧"},
				{RecordID: 4, UserID: 2, Name: "李四", TaskType: "weekly_video"},
			},
			want: "本周任务\n1 张三 基督 史剧\n2 李四 史剧 【新】视频",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCheckins(tt.entries, 4, tt.daily)
			if got != tt.want {
				t.Fatalf("message = %q, want %q", got, tt.want)
			}
			if strings.Count(got, "【新】") != 1 {
				t.Fatal("message must mark only the triggering checkin")
			}
		})
	}
}

func TestSplitMessagePreservesMemberLines(t *testing.T) {
	t.Parallel()
	lines := []string{"本周任务"}
	for i := 1; i <= 400; i++ {
		lines = append(lines, fmt.Sprintf("%d 张三 基督 史剧 视频", i))
	}
	text := strings.Join(lines, "\n")
	chunks := splitMessage(text)
	if len(chunks) < 2 {
		t.Fatal("fixture must require splitting")
	}
	if strings.Join(chunks, "\n") != text {
		t.Fatal("chunks must split between complete member lines")
	}
	for _, chunk := range chunks {
		if len(chunk) > 3500 {
			t.Fatal("chunk exceeds byte limit")
		}
	}
}

func TestSnapshotIncludesRecordsBeforeBotJoined(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	connector := &sourceConnector{t: t, steps: []queryStep{
		{
			contains: []string{"group_id=? AND id=? AND logical_date=?"},
			args:     []any{int64(1), int64(4), "2026-09-09"}, columns: 4,
			rows: [][]driver.Value{{"daily_devotion", date, date.Add(time.Hour), nil}},
		},
		{
			contains: []string{"c.logical_date BETWEEN ? AND ?", "c.id<=?", "c.deleted_at IS NULL"},
			args:     []any{int64(1), "2026-09-09", "2026-09-09", int64(4)}, columns: 6,
			rows: [][]driver.Value{
				{int64(1), int64(1), "张三", "daily_devotion", "", ""},
				{int64(2), int64(2), "李四", "daily_devotion", "", ""},
				{int64(3), int64(3), "王五", "daily_devotion", "", ""},
				{int64(4), int64(4), "赵六", "daily_devotion", "", ""},
			},
		},
	}}
	db := sql.OpenDB(connector)
	defer db.Close()
	source := NewCheckinSource(db, time.FixedZone("CST", 8*3600))
	snapshot, err := source.Snapshot(t.Context(), Event{RecordID: 4, GroupID: 1, LogicalDate: "2026-09-09"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"张三", "李四", "王五", "赵六"} {
		if !strings.Contains(snapshot.Text, name) {
			t.Fatalf("summary omitted %s", name)
		}
	}
	if strings.Count(snapshot.Text, "【新】") != 1 || !strings.Contains(snapshot.Text, "【新】赵六") {
		t.Fatalf("incorrect new marker: %q", snapshot.Text)
	}
	if len(connector.steps) != 0 {
		t.Fatal("summary query was not executed")
	}
}
