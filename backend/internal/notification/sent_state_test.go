package notification

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSentStateStoreBootstrapsCompletedNotifications(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	completed := filepath.Join(dir, "completed")
	if err := os.MkdirAll(completed, 0o700); err != nil {
		t.Fatal(err)
	}
	target := Target{ChatID: 99, ChatType: 3}
	expiresAt := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	item := job{
		Event: Event{
			RecordID:    10,
			GroupID:     1,
			LogicalDate: "2026-09-10",
			OccurredAt:  time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC),
		},
		Target:    target,
		Messages:  []string{"每日灵修\n1 【新】张三"},
		ExpiresAt: expiresAt,
		NextPart:  1,
		Status:    "sent",
	}
	if err := writeJob(filepath.Join(completed, "legacy.json"), item); err != nil {
		t.Fatal(err)
	}
	store, err := newSentStateStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	content := canonicalNotificationContent("每日灵修\n1 张三")
	needsSend, err := store.NeedsSend(target, "daily", "daily:2026-09-10", contentHash(content))
	if err != nil {
		t.Fatal(err)
	}
	if needsSend {
		t.Fatal("completed notification was not migrated")
	}
}
