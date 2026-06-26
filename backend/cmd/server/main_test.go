package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWeeklyVerseTaskTitle(t *testing.T) {
	tests := []struct {
		name          string
		req           studyWeekInput
		existingTitle string
		want          string
	}{
		{
			name: "disabled returns empty",
			req: studyWeekInput{
				VerseEnabled: false,
			},
			existingTitle: "背经任务1",
			want:          "",
		},
		{
			name: "prefers verse ref",
			req: studyWeekInput{
				VerseEnabled: true,
				VerseRef:     "约翰福音 3:16",
			},
			existingTitle: "背经任务1",
			want:          "约翰福音 3:16",
		},
		{
			name: "falls back to existing title",
			req: studyWeekInput{
				VerseEnabled: true,
			},
			existingTitle: "背经任务1",
			want:          "背经任务1",
		},
		{
			name: "uses default title when empty",
			req: studyWeekInput{
				VerseEnabled: true,
			},
			existingTitle: "",
			want:          "背经",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weeklyVerseTaskTitle(tt.req, tt.existingTitle)
			if got != tt.want {
				t.Fatalf("weeklyVerseTaskTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveExistingFileInRootsFallsBackToContentRoot(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")
	if err := os.MkdirAll(filepath.Join(contentRoot, "Book"), 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	want := filepath.Join(contentRoot, "Book", "sample.pdf")
	if err := os.WriteFile(want, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	got, original, err := resolveExistingFileInRoots("/Book/sample.pdf", assetsRoot, contentRoot)
	if err != nil {
		t.Fatalf("resolveExistingFileInRoots returned error: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if original != "sample.pdf" {
		t.Fatalf("expected original sample.pdf, got %q", original)
	}
}

func TestResolveExistingFileInRootsPrefersAssetsRoot(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")
	if err := os.MkdirAll(filepath.Join(assetsRoot, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir assets root: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(contentRoot, "shared"), 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	want := filepath.Join(assetsRoot, "shared", "sample.pdf")
	if err := os.WriteFile(want, []byte("asset"), 0o644); err != nil {
		t.Fatalf("write assets file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "shared", "sample.pdf"), []byte("content"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}

	got, _, err := resolveExistingFileInRoots("shared/sample.pdf", assetsRoot, contentRoot)
	if err != nil {
		t.Fatalf("resolveExistingFileInRoots returned error: %v", err)
	}
	if got != want {
		t.Fatalf("expected assets root path %q, got %q", want, got)
	}
}

func TestResolveExistingFileInRootsReturnsErrorWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	assetsRoot := filepath.Join(tempDir, "assets")
	contentRoot := filepath.Join(tempDir, "content")

	if _, _, err := resolveExistingFileInRoots("/Book/missing.pdf", assetsRoot, contentRoot); err == nil {
		t.Fatal("expected missing file error")
	}
}
