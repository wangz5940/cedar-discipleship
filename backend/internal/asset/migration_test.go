package asset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResourceSharingMigration(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "migrations", "007_resource_sharing.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)
	required := []string{
		"asset_bindings",
		"asset_share_grants",
		"asset_import_events",
		"asset_dependencies",
		"uk_asset_import",
		"INSERT IGNORE INTO asset_bindings",
	}
	for _, item := range required {
		if !strings.Contains(sql, item) {
			t.Fatalf("migration does not contain %q", item)
		}
	}
	if strings.Contains(sql, "ALTER TABLE assets") {
		t.Fatal("migration must not alter assets directly because startup replays migrations")
	}
	if strings.Contains(sql, "CREATE TABLE IF NOT EXISTS asset_versions") {
		t.Fatal("resources are immutable and must not have a version table")
	}
	for _, item := range []string{"source_version_id", "selected_version_id"} {
		if strings.Contains(sql, item) {
			t.Fatalf("resource sharing migration must not contain version column %q", item)
		}
	}
}

func TestEnforceResourceObjectPathsMigration(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "migrations", "008_enforce_resource_object_paths.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)
	required := []string{
		"team-%-resources/objects/%",
		"DROP COLUMN source_version_id",
		"DROP COLUMN selected_version_id",
		"UPDATE asset_bindings",
		"UPDATE asset_share_grants",
		"UPDATE asset_dependencies",
	}
	for _, item := range required {
		if !strings.Contains(sql, item) {
			t.Fatalf("migration does not contain %q", item)
		}
	}
}

func TestClassifyMentorResourcesMigration(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "migrations", "009_classify_mentor_resources.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)
	required := []string{
		"category = 'mentor'",
		"storage_path LIKE 'team-%-resources/objects/%'",
		"导读",
		"内容概要",
	}
	for _, item := range required {
		if !strings.Contains(sql, item) {
			t.Fatalf("migration does not contain %q", item)
		}
	}
}
