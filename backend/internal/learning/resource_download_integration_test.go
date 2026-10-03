//go:build integration

package learning

import (
	"testing"

	"agp/backend/internal/testdb"
)

func TestResourceDownloadSettingsPersistWithoutChangingOtherGroups(t *testing.T) {
	db := testdb.Open(t)
	service := NewService(NewMySQLRepository(db))
	if err := service.SaveLearningConfig(t.Context(), 1, map[string]any{"custom_title": "原配置"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveResourceDownloadEnabled(t.Context(), 1, false); err != nil {
		t.Fatal(err)
	}
	settings, err := service.LearningConfig(t.Context(), 1)
	if err != nil || ResourceDownloadsAllowed(settings) || settings["custom_title"] != "原配置" {
		t.Fatalf("saved settings=%v err=%v", settings, err)
	}
	if err := service.SaveLearningConfig(t.Context(), 1, map[string]any{"custom_title": "更新配置", "resource_download_enabled": true}); err != nil {
		t.Fatal(err)
	}
	settings, err = service.LearningConfig(t.Context(), 1)
	if err != nil || ResourceDownloadsAllowed(settings) || settings["custom_title"] != "更新配置" {
		t.Fatalf("stale learning configuration changed download policy: settings=%v err=%v", settings, err)
	}
	other, err := service.LearningConfig(t.Context(), 2)
	if err != nil || !ResourceDownloadsAllowed(other) {
		t.Fatalf("other group settings=%v err=%v", other, err)
	}
	if err := service.SaveResourceDownloadEnabled(t.Context(), 1, true); err != nil {
		t.Fatal(err)
	}
	settings, err = service.LearningConfig(t.Context(), 1)
	if err != nil || !ResourceDownloadsAllowed(settings) {
		t.Fatalf("downloads could not be re-enabled: settings=%v err=%v", settings, err)
	}
}
