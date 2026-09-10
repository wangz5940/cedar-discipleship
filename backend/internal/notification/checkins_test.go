package notification

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFormatCheckins(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		entries []Entry
		id      uint64
		daily   bool
		want    string
	}{
		{
			name: "first daily checkin has no new marker",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "daily_devotion"},
			},
			id: 1, daily: true, want: "每日灵修\n1 张三",
		},
		{
			name: "daily summary has no new marker",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "daily_devotion"},
				{RecordID: 2, UserID: 20, Name: "李四", TaskType: "daily_devotion"},
			},
			id: 2, daily: true, want: "每日灵修\n1 张三\n2 李四",
		},
		{
			name: "first book is new",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "《基督是一切》"},
			},
			id: 1, want: "本周任务\n1 张三 【新】基督",
		},
		{
			name: "existing member adds another book",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
				{RecordID: 2, UserID: 20, Name: "李四", TaskType: "weekly_book", BookName: "史剧"},
				{RecordID: 3, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "史剧"},
			},
			id: 3, want: "本周任务\n1 张三 基督 【新】史剧\n2 李四 史剧",
		},
		{
			name: "existing member adds video",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
				{RecordID: 2, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "史剧"},
				{RecordID: 3, UserID: 20, Name: "李四", TaskType: "weekly_book", BookName: "史剧"},
				{RecordID: 4, UserID: 20, Name: "李四", TaskType: "weekly_video"},
			},
			id: 4, want: "本周任务\n1 张三 基督 史剧\n2 李四 史剧 【新】视频",
		},
		{
			name: "first video is new and same names remain separate",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基"},
				{RecordID: 2, UserID: 20, Name: "张三", TaskType: "weekly_video"},
			},
			id: 2, want: "本周任务\n1 张三 基\n2 张三 【新】视频",
		},
		{
			name: "same book merges with new marker",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
				{RecordID: 2, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
			},
			id: 2, want: "本周任务\n1 张三 【新】基督",
		},
		{
			name: "unsupported checkin does not notify",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_book", BookName: "基督是一切"},
				{RecordID: 2, UserID: 10, Name: "张三", TaskType: "weekly_verse"},
			},
			id: 2, want: "",
		},
		{
			name: "deleted trigger does not notify",
			entries: []Entry{
				{RecordID: 1, UserID: 10, Name: "张三", TaskType: "weekly_video"},
			},
			id: 2, want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatCheckins(tt.entries, tt.id, tt.daily); got != tt.want {
				t.Fatalf("FormatCheckins() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEligible(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, taskType, date, today, start, end string
		want                                    bool
	}{
		{"today", "daily_devotion", "2026-09-09", "2026-09-09", "", "", true},
		{"daily retro", "daily_devotion", "2026-09-08", "2026-09-09", "", "", false},
		{"future daily", "daily_devotion", "2026-09-10", "2026-09-09", "", "", false},
		{"current week retro day", "weekly_book", "2026-09-07", "2026-09-09", "2026-09-07", "2026-09-13", true},
		{"previous week", "weekly_book", "2026-09-06", "2026-09-09", "2026-08-31", "2026-09-06", false},
		{"week last day", "weekly_video", "2026-09-13", "2026-09-13", "2026-09-07", "2026-09-13", true},
		{"week first day", "weekly_video", "2026-09-07", "2026-09-07", "2026-09-07", "2026-09-13", true},
		{"unsupported", "weekly_outline", "2026-09-09", "2026-09-09", "2026-09-07", "2026-09-13", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eligible(tt.taskType, tt.date, tt.today, tt.start, tt.end); got != tt.want {
				t.Fatalf("eligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBookName(t *testing.T) {
	t.Parallel()
	if got := bookName("《基督是一切》36-40页", `{"book_name":"基督是一切"}`); got != "基督是一切" {
		t.Fatalf("bookName() = %q", got)
	}
	if got := bookName("书籍标题", "legacy content"); got != "书籍标题" {
		t.Fatalf("legacy bookName() = %q", got)
	}
}

func TestSplitMessage(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("1 张三 【新】视频 ", 900)
	for _, chunk := range splitMessage(text) {
		if len(chunk) > 3500 || !utf8.ValidString(chunk) {
			t.Fatalf("invalid chunk length=%d", len(chunk))
		}
	}
	got := strings.Join(splitMessage(text), " ")
	if strings.Join(strings.Fields(got), " ") != strings.TrimSpace(text) {
		t.Fatal("split lost message content")
	}
	unbroken := strings.Repeat("灵", 4000)
	if strings.Join(splitMessage(unbroken), "") != unbroken {
		t.Fatal("split lost unbroken Unicode content")
	}
}
