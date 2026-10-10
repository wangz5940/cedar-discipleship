package server

import (
	"testing"
	"time"
)

func TestDevotionReminderDue(t *testing.T) {
	for _, tc := range []struct {
		at  string
		due bool
	}{{"2026-10-09T23:29:59Z", false}, {"2026-10-09T23:30:00Z", true}, {"2026-10-09T23:30:59Z", true}, {"2026-10-09T23:31:00Z", false}, {"2026-10-10T07:30:00Z", false}, {"2026-10-10T10:29:59Z", false}, {"2026-10-10T10:30:00Z", true}, {"2026-10-10T10:30:59Z", true}, {"2026-10-10T10:31:00Z", false}} {
		at, _ := time.Parse(time.RFC3339, tc.at)
		if got := devotionReminderDue(at); got != tc.due {
			t.Fatalf("%s: due=%v", tc.at, got)
		}
	}
}
