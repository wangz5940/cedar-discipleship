package notification

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type initialSource struct {
	events []Event
}

func (*initialSource) Enabled(context.Context, Event) (bool, error) { return true, nil }

func (s *initialSource) Snapshot(_ context.Context, event Event) (Snapshot, error) {
	s.events = append(s.events, event)
	return Snapshot{
		Text:      event.Initial,
		ExpiresAt: event.OccurredAt.Add(time.Hour),
		Topic:     event.Initial,
		Version:   event.Initial + ":2026-09-09",
	}, nil
}

type topicSource struct {
	snapshots map[string]Snapshot
}

func (*topicSource) Enabled(context.Context, Event) (bool, error) { return true, nil }

func (s *topicSource) Snapshot(_ context.Context, event Event) (Snapshot, error) {
	return s.snapshots[event.Initial], nil
}

func TestInitialQueueSendsTwoOnceAcrossRestarts(t *testing.T) {
	t.Parallel()
	source, sender := &initialSource{}, &fakeSender{}
	targets := map[uint64][]Target{1: {{ChatID: 99, ChatType: 3}}}
	dir, now := t.TempDir(), time.Now()
	q, err := NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 5 {
		workers.Go(func() {
			if err := q.EnqueueInitial(now); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if got := stateFiles(t, q, "pending"); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	q.processNext(t.Context(), now)
	restarted, err := NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.EnqueueInitial(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.Add(time.Second))
	if !reflect.DeepEqual(sender.messages, []string{"daily", "weekly"}) {
		t.Fatalf("messages = %#v", sender.messages)
	}
	if err := restarted.EnqueueInitial(now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.AddDate(0, 0, 1))
	if len(sender.messages) != 2 || stateFiles(t, q, "completed") != 2 {
		t.Fatal("restart resent completed initial summaries")
	}
	for index, event := range source.events {
		wantOccurredAt := now.Add(time.Duration(index) * time.Second)
		if event.GroupID != 1 || event.RecordID != 0 || !event.OccurredAt.Equal(wantOccurredAt) {
			t.Fatalf("wrong persisted event: %#v", event)
		}
	}
	newTarget := Target{ChatID: 100, ChatType: 3}
	switched, err := NewQueue(dir, map[uint64][]Target{1: {newTarget}}, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := switched.EnqueueInitial(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	switched.processNext(t.Context(), now.Add(time.Minute))
	switched.processNext(t.Context(), now.Add(time.Minute+time.Second))
	if len(sender.messages) != 4 || sender.targets[2] != newTarget || sender.targets[3] != newTarget {
		t.Fatal("new chat did not receive its own initial summaries")
	}
}

func TestInitialQueueRetriesOnlyFailedSummary(t *testing.T) {
	t.Parallel()
	source, sender := &initialSource{}, &fakeSender{}
	targets := map[uint64][]Target{1: {{ChatID: 99, ChatType: 3}}}
	dir, now := t.TempDir(), time.Now()
	q, err := NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.EnqueueInitial(now); err != nil {
		t.Fatal(err)
	}
	q.processNext(t.Context(), now)
	sender.err = &deliveryError{code: "http_503", retry: true}
	q.processNext(t.Context(), now.Add(time.Second))
	restarted, err := NewQueue(dir, targets, source, sender)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.EnqueueInitial(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	restarted.processNext(t.Context(), now.Add(2*time.Second))
	if len(sender.messages) != 2 {
		t.Fatal("backoff or initial deduplication was ignored")
	}
	sender.err = nil
	restarted.processNext(t.Context(), now.Add(11*time.Second))
	if !reflect.DeepEqual(sender.messages, []string{"daily", "weekly", "weekly"}) {
		t.Fatalf("messages = %#v", sender.messages)
	}
	if len(source.events) != 2 {
		t.Fatal("retry recomputed the initial snapshot")
	}
}

func TestQueueWakeInitialContentDiff(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	target := Target{ChatID: 99, ChatType: 3}
	daily := Snapshot{
		Text:      "每日灵修\n1 张三\n2 李四",
		ExpiresAt: now.Add(12 * time.Hour),
		Topic:     "daily",
		Version:   "daily:2026-09-10",
	}
	weekly := Snapshot{
		Text:      "本周任务\n1 张三 基督 视频\n2 李四 史剧",
		ExpiresAt: now.Add(72 * time.Hour),
		Topic:     "weekly",
		Version:   "weekly:2026-09-13",
	}
	tests := []struct {
		name          string
		previousDaily *sentState
		previousWeek  *sentState
		currentDaily  Snapshot
		currentWeek   Snapshot
		want          []string
	}{
		{
			name:         "never sent",
			currentDaily: daily,
			currentWeek:  weekly,
			want:         []string{daily.Text, weekly.Text},
		},
		{
			name:          "unchanged",
			previousDaily: stateForSnapshot(target, daily, now),
			previousWeek:  stateForSnapshot(target, weekly, now),
			currentDaily:  daily,
			currentWeek:   weekly,
		},
		{
			name:          "daily changed sends full current day",
			previousDaily: stateForSnapshot(target, snapshotWithText(daily, "每日灵修\n1 张三"), now),
			previousWeek:  stateForSnapshot(target, weekly, now),
			currentDaily:  daily,
			currentWeek:   weekly,
			want:          []string{daily.Text},
		},
		{
			name:          "weekly changed sends full current week",
			previousDaily: stateForSnapshot(target, daily, now),
			previousWeek:  stateForSnapshot(target, snapshotWithText(weekly, "本周任务\n1 张三 基督"), now),
			currentDaily:  daily,
			currentWeek:   weekly,
			want:          []string{weekly.Text},
		},
		{
			name:          "both changed send both full snapshots",
			previousDaily: stateForSnapshot(target, snapshotWithText(daily, "每日灵修\n1 张三"), now),
			previousWeek:  stateForSnapshot(target, snapshotWithText(weekly, "本周任务\n1 张三 基督"), now),
			currentDaily:  daily,
			currentWeek:   weekly,
			want:          []string{daily.Text, weekly.Text},
		},
		{
			name:          "new marker is ignored",
			previousDaily: stateForSnapshot(target, daily, now),
			previousWeek:  stateForSnapshot(target, weekly, now),
			currentDaily:  snapshotWithText(daily, "每日灵修\n1 张三\n2 【新】李四"),
			currentWeek:   weekly,
		},
		{
			name: "new period with same content",
			previousDaily: stateForSnapshot(target, Snapshot{
				Text: daily.Text, Topic: "daily", Version: "daily:2026-09-09",
			}, now),
			previousWeek: stateForSnapshot(target, Snapshot{
				Text: weekly.Text, Topic: "weekly", Version: "weekly:2026-09-06",
			}, now),
			currentDaily: daily,
			currentWeek:  weekly,
			want:         []string{daily.Text, weekly.Text},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := &topicSource{snapshots: map[string]Snapshot{
				"daily":  tt.currentDaily,
				"weekly": tt.currentWeek,
			}}
			sender := &fakeSender{}
			queue, err := NewQueue(t.TempDir(), map[uint64][]Target{1: {target}}, source, sender)
			if err != nil {
				t.Fatal(err)
			}
			for _, previous := range []*sentState{tt.previousDaily, tt.previousWeek} {
				if previous != nil {
					if err := queue.sent.Record(*previous); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := queue.EnqueueInitialBinding(1, target, now); err != nil {
				t.Fatal(err)
			}
			queue.processNext(t.Context(), now)
			queue.processNext(t.Context(), now.Add(time.Second))
			if !reflect.DeepEqual(sender.messages, tt.want) {
				t.Fatalf("messages = %#v, want %#v", sender.messages, tt.want)
			}
		})
	}
}

func snapshotWithText(snapshot Snapshot, text string) Snapshot {
	snapshot.Text = text
	return snapshot
}

func stateForSnapshot(target Target, snapshot Snapshot, sentAt time.Time) *sentState {
	content := canonicalNotificationContent(snapshot.Text)
	return &sentState{
		Target:  target,
		Topic:   snapshot.Topic,
		Version: snapshot.Version,
		Hash:    contentHash(content),
		Content: content,
		SentAt:  sentAt,
	}
}

func TestInitialSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 8, 16, 5, 0, 0, time.UTC)
	start := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 6)
	tests := []struct {
		name, kind  string
		rows        [][]driver.Value
		noWeek      bool
		dbError     bool
		want        string
		wantVersion string
	}{
		{
			name: "daily includes existing people", kind: "daily",
			rows: [][]driver.Value{
				{int64(1), int64(1), "张三", "daily_devotion", "", ""},
				{int64(2), int64(2), "李四", "daily_devotion", "", ""},
			},
			want: "每日灵修\n1 张三\n2 李四", wantVersion: "daily:2026-09-09",
		},
		{
			name: "weekly groups books and video", kind: "weekly",
			rows: [][]driver.Value{
				{int64(1), int64(1), "张三", "weekly_book", "基督是一切", ""},
				{int64(2), int64(1), "张三", "weekly_video", "", ""},
				{int64(3), int64(2), "李四", "weekly_book", "史剧", ""},
			},
			want: "本周任务\n1 张三 基督 视频\n2 李四 史剧", wantVersion: "weekly:2026-09-13",
		},
		{
			name: "carried video and repeated completion merge", kind: "weekly",
			rows: [][]driver.Value{
				{int64(1), int64(1), "张三", "weekly_video", "", ""},
				{int64(2), int64(1), "张三", "weekly_video", "", ""},
			},
			want: "本周任务\n1 张三 视频", wantVersion: "weekly:2026-09-13",
		},
		{name: "empty daily", kind: "daily", want: "每日灵修\n暂无打卡记录", wantVersion: "daily:2026-09-09"},
		{name: "empty weekly", kind: "weekly", want: "本周任务\n暂无打卡记录", wantVersion: "weekly:2026-09-13"},
		{name: "no current week", kind: "weekly", noWeek: true, want: "本周任务\n暂无打卡记录", wantVersion: "weekly:none:2026-09-09"},
		{name: "database failure is not empty progress", kind: "daily", dbError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var steps []queryStep
			from, to := "2026-09-09", "2026-09-09"
			if tt.kind == "weekly" {
				weekRows := [][]driver.Value{{int64(7), start, end}}
				if tt.noWeek {
					weekRows = nil
				}
				steps = append(steps, queryStep{
					contains: []string{"FROM study_weeks", "group_id=?", "start_date<=? AND end_date>=?"},
					args:     []any{int64(1), "2026-09-09", "2026-09-09"}, columns: 3, rows: weekRows,
				})
				from, to = "2026-09-07", "2026-09-13"
			}
			if !tt.noWeek {
				args := []any{int64(1), from, to, now.UTC().Format("2006-01-02 15:04:05.000")}
				fragments := []string{
					"c.group_id=?", "c.logical_date BETWEEN ? AND ?", "c.checkin_time<=?",
					"m.group_id=c.group_id", "c.deleted_at IS NULL", "c.status='done'",
				}
				if tt.kind == "weekly" {
					args = append(args[:3], int64(7), int64(7), now.UTC().Format("2006-01-02 15:04:05.000"))
					fragments = append(fragments, "c.week_id=?", "current_task.week_id=?",
						"current_ta.asset_id=checked_ta.asset_id", "checked_ta.group_id=c.group_id",
						"current_task.enabled=1")
				} else {
					fragments = append(fragments, "c.task_type='daily_devotion'")
				}
				step := queryStep{contains: fragments, args: args, columns: 6, rows: tt.rows}
				if tt.dbError {
					step.err = errors.New("database unavailable")
				}
				steps = append(steps, step)
			}
			connector := &sourceConnector{t: t, steps: steps}
			db := sql.OpenDB(connector)
			defer db.Close()
			s := NewCheckinSource(db, time.FixedZone("CST", 8*3600))
			snapshot, err := s.Snapshot(t.Context(), Event{GroupID: 1, Initial: tt.kind, OccurredAt: now})
			if (err != nil) != tt.dbError || snapshot.Text != tt.want {
				t.Fatalf("Snapshot = %q err=%v, want %q", snapshot.Text, err, tt.want)
			}
			if strings.Contains(snapshot.Text, "【新】") {
				t.Fatal("initial progress must not mark historical checkins as new")
			}
			if !tt.dbError && (snapshot.Topic != tt.kind || snapshot.Version != tt.wantVersion) {
				t.Fatalf("topic/version = %q/%q, want %q/%q",
					snapshot.Topic, snapshot.Version, tt.kind, tt.wantVersion)
			}
			if !tt.dbError && !snapshot.ExpiresAt.After(now) {
				t.Fatal("initial snapshot already expired")
			}
			if len(connector.steps) != 0 {
				t.Fatal("expected query was not executed")
			}
		})
	}
}
