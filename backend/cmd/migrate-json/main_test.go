package main

import (
	"encoding/json"
	"testing"
)

func TestTasksForWeekSplitsMultipleReadingsIntoMultipleWeeklyBookTasks(t *testing.T) {
	titleJSON, err := json.Marshal([]string{
		"《基督是一切》48-52页",
		"《救赎史剧》109-118页",
	})
	if err != nil {
		t.Fatalf("marshal title json: %v", err)
	}
	week := oldWeek{
		Title: titleJSON,
		Readings: []oldAssetRef{
			{Title: "《基督是一切》48-52页", URL: "/Book/book-a.pdf", Type: "pdf"},
			{Title: "《救赎史剧》109-118页", URL: "/Book/book-b.pdf", Type: "pdf"},
		},
		Video: "本周视频",
		Verse: "背经",
	}

	tasks := tasksForWeek(week)
	var bookTasks []plannedTask
	for _, task := range tasks {
		if task.Type == "weekly_book" {
			bookTasks = append(bookTasks, task)
		}
	}

	if len(bookTasks) != 2 {
		t.Fatalf("expected 2 weekly_book tasks, got %d", len(bookTasks))
	}
	if bookTasks[0].Title != "《基督是一切》48-52页" {
		t.Fatalf("unexpected first title: %q", bookTasks[0].Title)
	}
	if bookTasks[1].Title != "《救赎史剧》109-118页" {
		t.Fatalf("unexpected second title: %q", bookTasks[1].Title)
	}
	if len(bookTasks[0].Assets) != 1 || bookTasks[0].Assets[0].Ref.URL != "/Book/book-a.pdf" {
		t.Fatalf("unexpected first assets: %+v", bookTasks[0].Assets)
	}
	if len(bookTasks[1].Assets) != 1 || bookTasks[1].Assets[0].Ref.URL != "/Book/book-b.pdf" {
		t.Fatalf("unexpected second assets: %+v", bookTasks[1].Assets)
	}
}

func TestReadingTasksForWeekFallsBackWhenTitleCountDiffersFromReadingCount(t *testing.T) {
	titleJSON, err := json.Marshal([]string{"《基督是一切》48-52页"})
	if err != nil {
		t.Fatalf("marshal title json: %v", err)
	}
	week := oldWeek{
		Title: titleJSON,
		Readings: []oldAssetRef{
			{Title: "", URL: "/Book/book-a.pdf", Type: "pdf"},
			{Title: "《救赎史剧》109-118页", URL: "/Book/book-b.pdf", Type: "pdf"},
		},
	}

	tasks := readingTasksForWeek(week)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 reading tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "《基督是一切》48-52页" {
		t.Fatalf("unexpected first task title: %q", tasks[0].Title)
	}
	if tasks[1].Title != "《救赎史剧》109-118页" {
		t.Fatalf("unexpected second task title: %q", tasks[1].Title)
	}
}

func TestParseReadingMetadata(t *testing.T) {
	got := parseReadingMetadata("《基督是一切》基督是神的仆人--马可福音（22-25页，读到事奉的性质为止）")
	if got.BookName != "基督是一切" {
		t.Fatalf("BookName = %q, want %q", got.BookName, "基督是一切")
	}
	if got.PageStart != 22 || got.PageEnd != 25 {
		t.Fatalf("page range = %d-%d, want 22-25", got.PageStart, got.PageEnd)
	}
	if got.ReadingNote != "读到事奉的性质为止" {
		t.Fatalf("ReadingNote = %q, want %q", got.ReadingNote, "读到事奉的性质为止")
	}
}

func TestMigratedReadingContentIsStructuredJSON(t *testing.T) {
	content := migratedReadingContent("《救赎史剧》纵览 73-79页")
	var got migratedReadingMetadata
	if err := json.Unmarshal([]byte(content), &got); err != nil {
		t.Fatalf("unmarshal migrated content: %v", err)
	}
	if got.BookName != "救赎史剧" || got.PageStart != 73 || got.PageEnd != 79 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
}

func TestNormalizeTaskSectionsBuildsCurrentScriptureAndDevotionShape(t *testing.T) {
	raw := json.RawMessage(`{
		"daily": {
			"path": "/newtestament.md",
			"devotion": {"start_date": "2026-05-27", "start_section": 43},
			"scripture": {
				"book": "路加福音",
				"book_id": "42",
				"max_chapters": 24,
				"start_date": "2026-05-27"
			}
		}
	}`)
	normalized, err := normalizeTaskSections(raw)
	if err != nil {
		t.Fatalf("normalize task sections: %v", err)
	}
	var sections map[string]any
	if err := json.Unmarshal(normalized, &sections); err != nil {
		t.Fatalf("unmarshal normalized sections: %v", err)
	}
	daily := sections["daily"].(map[string]any)
	devotion := daily["devotion"].(map[string]any)
	if devotion["path"] != "/newtestament.md" || devotion["numbered_start_date"] != "2026-05-27" || devotion["numbered_start"].(float64) != 43 {
		t.Fatalf("unexpected devotion config: %+v", devotion)
	}
	scripture := daily["scripture"].(map[string]any)
	sequence := scripture["sequence"].([]any)
	if len(sequence) != 25 {
		t.Fatalf("unexpected scripture sequence: %+v", sequence)
	}
	first := sequence[0].(map[string]any)
	last := sequence[len(sequence)-1].(map[string]any)
	if first["book"] != "路加福音" || first["book_id"] != "42" || first["chapters"].(float64) != 24 {
		t.Fatalf("unexpected first scripture book: %+v", first)
	}
	if last["book"] != "启示录" || last["book_id"] != "66" || last["chapters"].(float64) != 22 {
		t.Fatalf("unexpected last scripture book: %+v", last)
	}
	if scripture["book"] != "路加福音" || scripture["book_id"] != "42" || scripture["max_chapters"].(float64) != 24 {
		t.Fatalf("unexpected scripture start book: %+v", scripture)
	}
}
