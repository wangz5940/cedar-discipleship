package backup

import (
	"errors"
	"testing"

	"agp/backend/internal/learning"
)

func TestMapBackupTaskIDsHandlesDuplicateTitles(t *testing.T) {
	week := learning.WeekInput{
		Readings: []learning.TaskBinding{
			{TaskID: 11, Title: "同名读物"},
			{TaskID: 12, Title: "同名读物"},
		},
	}
	drafts := []learning.TaskDraft{
		{TaskType: "weekly_book", Title: "同名读物"},
		{TaskType: "weekly_book", Title: "同名读物"},
	}
	taskIDs := map[uint64]uint64{}

	mapBackupTaskIDs(week, drafts, []uint64{101, 102}, taskIDs)

	if taskIDs[11] != 101 || taskIDs[12] != 102 {
		t.Fatalf("task ID map = %#v, want 11->101 and 12->102", taskIDs)
	}
}

func TestNormalizeBackupSettingsRemapsLegacyDailyPaths(t *testing.T) {
	t.Parallel()

	settings := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"path": "/newtestament.md",
				"devotion": map[string]any{
					"path": "https://mouss.synology.me:7399/newtestament.md",
					"type": "markdown",
				},
			},
		},
	}
	resolve := func(value, preferredCategory string) (uint64, error) {
		if preferredCategory != "markdown" {
			t.Fatalf("preferred category = %q, want markdown", preferredCategory)
		}
		if backupResourceFileName(value) == "newtestament.md" {
			return 193, nil
		}
		return 0, nil
	}

	got, err := normalizeBackupSettings(settings, resolve)
	if err != nil {
		t.Fatalf("normalizeBackupSettings() error = %v", err)
	}
	daily, ok := nestedBackupSettingsMap(got, "task_sections", "daily")
	if !ok {
		t.Fatal("daily settings missing")
	}
	devotion, ok := nestedBackupSettingsMap(daily, "devotion")
	if !ok {
		t.Fatal("devotion settings missing")
	}
	if daily["path"] != "/api/assets/193/download" {
		t.Fatalf("daily path = %q, want asset download path", daily["path"])
	}
	if devotion["path"] != "/api/assets/193/download" {
		t.Fatalf("devotion path = %q, want asset download path", devotion["path"])
	}
	if original := settings["task_sections"].(map[string]any)["daily"].(map[string]any)["path"]; original != "/newtestament.md" {
		t.Fatalf("original settings mutated: %q", original)
	}
}

func TestNormalizeBackupSettingsRemapsAssetDownloadID(t *testing.T) {
	t.Parallel()

	settings := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"devotion": map[string]any{
					"path": "/api/assets/12/download",
				},
			},
		},
	}
	resolve := func(value, _ string) (uint64, error) {
		if backupAssetIDFromDownloadURL(value) == 12 {
			return 193, nil
		}
		return 0, nil
	}

	got, err := normalizeBackupSettings(settings, resolve)
	if err != nil {
		t.Fatalf("normalizeBackupSettings() error = %v", err)
	}
	devotion, ok := nestedBackupSettingsMap(got, "task_sections", "daily", "devotion")
	if !ok {
		t.Fatal("devotion settings missing")
	}
	if devotion["path"] != "/api/assets/193/download" {
		t.Fatalf("devotion path = %q, want remapped asset download path", devotion["path"])
	}
}

func TestNormalizeBackupSettingsDropsUnresolvedLocalPath(t *testing.T) {
	t.Parallel()

	settings := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"path": "/newtestament.md",
				"devotion": map[string]any{
					"path": "/newtestament.md",
				},
			},
		},
	}
	resolve := func(_, _ string) (uint64, error) {
		return 0, nil
	}

	got, err := normalizeBackupSettings(settings, resolve)
	if err != nil {
		t.Fatalf("normalizeBackupSettings() error = %v", err)
	}
	daily, ok := nestedBackupSettingsMap(got, "task_sections", "daily")
	if !ok {
		t.Fatal("daily settings missing")
	}
	devotion, ok := nestedBackupSettingsMap(daily, "devotion")
	if !ok {
		t.Fatal("devotion settings missing")
	}
	if _, ok := daily["path"]; ok {
		t.Fatalf("daily path was not removed: %q", daily["path"])
	}
	if _, ok := devotion["path"]; ok {
		t.Fatalf("devotion path was not removed: %q", devotion["path"])
	}
}

func TestNormalizeBackupSettingsReturnsResolverError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("resolver failed")
	settings := map[string]any{
		"task_sections": map[string]any{
			"daily": map[string]any{
				"path": "/newtestament.md",
			},
		},
	}
	resolve := func(_, _ string) (uint64, error) {
		return 0, wantErr
	}

	if _, err := normalizeBackupSettings(settings, resolve); !errors.Is(err, wantErr) {
		t.Fatalf("normalizeBackupSettings() error = %v, want %v", err, wantErr)
	}
}
