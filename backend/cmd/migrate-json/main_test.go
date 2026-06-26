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
