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
	needsSend, err := store.NeedsSend(sentState{
		GroupID: 1, Target: target, Topic: "daily", Version: "daily:2026-09-10", Hash: contentHash(content),
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if needsSend {
		t.Fatal("completed notification was not migrated")
	}
	// Old last-sent filenames have no reliable group identity. Only completed
	// jobs may reconstruct that association during an upgrade.
	if err := os.WriteFile(filepath.Join(store.dir, "00000000000000000099-3-daily.json"),
		[]byte(`{"target":{"chat_id":99,"chat_type":3},"topic":"daily","version":"daily:2026-09-30","hash":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := newSentStateStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, groupID := range []uint64{1, 2} {
		needsSend, err := restarted.NeedsSend(sentState{
			GroupID: groupID, Target: target, Topic: "daily", Version: "daily:2026-09-10", Hash: contentHash(content),
		}, 0)
		if err != nil || needsSend != (groupID == 2) {
			t.Fatalf("group %d after legacy upgrade: send=%v err=%v", groupID, needsSend, err)
		}
	}
}

func TestSentStateStoreUsesStablePeriodID(t *testing.T) {
	t.Parallel()
	target := Target{ChatID: 99, ChatType: 3}
	tests := []struct {
		name      string
		previous  sentState
		candidate sentState
		wantSend  bool
	}{
		{
			name: "same week allows changed end date",
			previous: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-29", PeriodID: "week:7", Hash: contentHash("old"),
			},
			candidate: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-28", PeriodID: "week:7", Hash: contentHash("new"),
			},
			wantSend: true,
		},
		{
			name: "different older week stays blocked",
			previous: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-29", PeriodID: "week:8", Hash: contentHash("old"),
			},
			candidate: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-28", PeriodID: "week:7", Hash: contentHash("new"),
			},
		},
		{
			name: "legacy state keeps date ordering",
			previous: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-29", Hash: contentHash("old"),
			},
			candidate: sentState{
				GroupID: 1, Target: target, Topic: "weekly",
				Version: "weekly:2026-09-28", PeriodID: "week:7", Hash: contentHash("new"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "completed"), 0o700); err != nil {
				t.Fatal(err)
			}
			store, err := newSentStateStore(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Record(tt.previous); err != nil {
				t.Fatal(err)
			}
			got, err := store.NeedsSend(tt.candidate, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.wantSend {
				t.Fatalf("NeedsSend = %v, want %v", got, tt.wantSend)
			}
		})
	}
}
