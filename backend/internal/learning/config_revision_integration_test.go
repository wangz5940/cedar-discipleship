//go:build integration

package learning

import (
	"agp/backend/internal/testdb"
	"errors"
	"testing"
)

func TestLearningConfigRejectsStaleSnapshotWithoutLosingNewerData(t *testing.T) {
	db := testdb.Open(t)
	repo := NewMySQLRepository(db)
	first := map[string]any{"_revision": 0, "title": "第一位管理员"}
	if err := repo.SaveLearningConfig(t.Context(), 1, first); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveLearningConfig(t.Context(), 1, map[string]any{"_revision": 0, "title": "旧页面覆盖"}); !errors.Is(err, ErrLearningConfigConflict) {
		t.Fatalf("stale save err=%v", err)
	}
	current, err := repo.LearningConfig(t.Context(), 1)
	if err != nil || current["title"] != "第一位管理员" {
		t.Fatalf("data=%v err=%v", current, err)
	}
	if err := repo.SaveLearningConfig(t.Context(), 1, map[string]any{"_revision": 1, "title": "新页面"}); err != nil {
		t.Fatal(err)
	}
	// Legacy clients without a revision retain the original API contract.
	if err := repo.SaveLearningConfig(t.Context(), 1, map[string]any{"title": "旧客户端"}); err != nil {
		t.Fatal(err)
	}
}
