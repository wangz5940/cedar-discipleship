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

func TestBackupResourceKeyFromStoragePath(t *testing.T) {
	t.Parallel()

	got := backupResourceKeyFromStoragePath("team-agape-a-resources/objects/A2DB2A2E5D31A9EDC6215E79F9B499DE/newtestament.md")
	if got != "a2db2a2e5d31a9edc6215e79f9b499de" {
		t.Fatalf("backupResourceKeyFromStoragePath() = %q, want normalized resource key", got)
	}
	if invalid := backupResourceKeyFromStoragePath("legacy/newtestament.md"); invalid != "" {
		t.Fatalf("backupResourceKeyFromStoragePath() = %q, want empty for legacy path", invalid)
	}
}

func TestBackupTaskAssetRefsUsesReadingMetadata(t *testing.T) {
	t.Parallel()

	got := backupTaskAssetRefs(
		"《救赎史剧》纵览 88-96页",
		`{"book_name":"救赎史剧","page_start":88,"page_end":96,"source_title":"《救赎史剧》纵览 88-96页"}`,
	)
	if len(got) != 2 || got[0] != "《救赎史剧》纵览 88-96页" || got[1] != "救赎史剧" {
		t.Fatalf("backupTaskAssetRefs() = %v, want source title and book name", got)
	}
}

func TestBackupTaskAssetMatchScorePrefersSpecificTitle(t *testing.T) {
	t.Parallel()

	sourceTitle := normalizeBackupTaskAssetText("《救赎史剧》纵览 79-84页（读到基督的赎罪工作为止）")
	specificAsset := normalizeBackupTaskAssetText("《救赎史剧》纵览 88-96页")
	broadAsset := normalizeBackupTaskAssetText("圣经救赎史剧综览-3.pdf")
	specificScore := backupTaskAssetMatchScore(sourceTitle, specificAsset)
	broadScore := backupTaskAssetMatchScore(normalizeBackupTaskAssetText("救赎史剧"), broadAsset)
	if specificScore <= broadScore {
		t.Fatalf("specific score = %d, broad score = %d, want specific higher", specificScore, broadScore)
	}
}

func TestCanonicalBackupAssetTitleRepairsPageScopedBookTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		title        string
		originalName string
		want         string
	}{
		{
			name:         "christ is all",
			title:        "《基督是一切》36-40页",
			originalName: "基督是一切-江守道.pdf",
			want:         "基督是一切-江守道",
		},
		{
			name:         "redemption drama overview",
			title:        "《救赎史剧》纵览 88-96页",
			originalName: "圣经救赎史剧综览-2.pdf",
			want:         "圣经救赎史剧综览-2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalBackupAssetTitle("book", tt.title, tt.originalName); got != tt.want {
				t.Fatalf("canonicalBackupAssetTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeBackupLogicalDateAcceptsExportedRFC3339Date(t *testing.T) {
	t.Parallel()

	got, err := normalizeBackupLogicalDate("2026-05-12T00:00:00Z")
	if err != nil {
		t.Fatalf("normalizeBackupLogicalDate() error = %v", err)
	}
	if got != "2026-05-12" {
		t.Fatalf("normalizeBackupLogicalDate() = %q, want 2026-05-12", got)
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
