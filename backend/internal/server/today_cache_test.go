package server

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	learningdomain "agp/backend/internal/learning"
)

func TestTodayContentCacheUsesValidatedSnapshot(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	var loads atomic.Int32
	load := func(context.Context) (learningdomain.TodayContent, error) {
		loads.Add(1)
		return testTodayContent("2026-08-28", "每日灵修"), nil
	}

	first, hit, err := cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, load)
	if err != nil || hit {
		t.Fatalf("first load = (%v, %v), want miss without error", hit, err)
	}
	first.Settings["label"] = "mutated"

	second, hit, err := cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, load)
	if err != nil || !hit {
		t.Fatalf("second load = (%v, %v), want hit without error", hit, err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", loads.Load())
	}
	if second.Settings["label"] != "每日灵修" {
		t.Fatalf("cached label = %v, want immutable snapshot", second.Settings["label"])
	}
	metrics := cache.Metrics()
	if metrics.Hits != 1 || metrics.Misses != 1 || metrics.HitRate != 0.5 {
		t.Fatalf("metrics = %+v, want one hit and one miss", metrics)
	}
}

func TestTodayContentCacheExpiresAtMidnight(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	beforeMidnight := time.Date(2026, 8, 28, 23, 59, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	var loads atomic.Int32
	load := func(context.Context) (learningdomain.TodayContent, error) {
		loads.Add(1)
		return testTodayContent("2026-08-28", "每日灵修"), nil
	}

	_, _, _ = cache.GetOrLoad(context.Background(), 1, "2026-08-28", beforeMidnight, load)
	_, hit, err := cache.GetOrLoad(
		context.Background(),
		1,
		"2026-08-28",
		beforeMidnight.Add(2*time.Minute),
		load,
	)
	if err != nil || hit {
		t.Fatalf("post-midnight load = (%v, %v), want expired miss", hit, err)
	}
	if loads.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", loads.Load())
	}
}

func TestTodayContentCacheBoundsMemoryAndRecoversFromCorruption(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	cache := newTodayContentCache(1, 1<<20, location)
	load := func(label string) func(context.Context) (learningdomain.TodayContent, error) {
		return func(context.Context) (learningdomain.TodayContent, error) {
			return testTodayContent("2026-08-28", label), nil
		}
	}

	_, _, _ = cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, load("group 1"))
	_, _, _ = cache.GetOrLoad(context.Background(), 2, "2026-08-28", now, load("group 2"))
	if metrics := cache.Metrics(); metrics.Entries != 1 || metrics.Evictions != 1 {
		t.Fatalf("bounded cache metrics = %+v, want one entry and one eviction", metrics)
	}

	key := todayCacheKey{groupID: 2, date: "2026-08-28"}
	cache.mu.Lock()
	cache.entries[key].Value.(*todayCacheEntry).payload[0] ^= 0xff
	cache.mu.Unlock()

	_, hit, err := cache.GetOrLoad(context.Background(), 2, "2026-08-28", now, load("reloaded"))
	if err != nil || hit {
		t.Fatalf("corrupted load = (%v, %v), want miss without error", hit, err)
	}
	if metrics := cache.Metrics(); metrics.Corruptions != 1 {
		t.Fatalf("corruptions = %d, want 1", metrics.Corruptions)
	}

	smallCache := newTodayContentCache(4, 128, location)
	large := testTodayContent("2026-08-28", strings.Repeat("x", 256))
	_, _, _ = smallCache.GetOrLoad(context.Background(), 1, "2026-08-28", now, func(context.Context) (learningdomain.TodayContent, error) {
		return large, nil
	})
	if metrics := smallCache.Metrics(); metrics.Entries != 0 || metrics.Rejected != 1 {
		t.Fatalf("oversized cache metrics = %+v, want rejected entry", metrics)
	}
}

func TestTodayContentCacheDegradesWhenEntryCannotBeStored(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	content := testTodayContent("2026-08-28", "每日灵修")
	content.Settings["unsupported"] = func() {}

	got, hit, err := cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, func(context.Context) (learningdomain.TodayContent, error) {
		return content, nil
	})
	if err != nil || hit || got.Title != content.Title {
		t.Fatalf("uncacheable load = (%+v, %v, %v), want direct result", got, hit, err)
	}
	if metrics := cache.Metrics(); metrics.Entries != 0 || metrics.Rejected != 1 {
		t.Fatalf("metrics = %+v, want rejected entry and direct fallback", metrics)
	}
}

func TestTodayContentCacheRecordsLoaderFailure(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	wantErr := errors.New("load failed")

	_, hit, err := cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, func(context.Context) (learningdomain.TodayContent, error) {
		return learningdomain.TodayContent{}, wantErr
	})
	if !errors.Is(err, wantErr) || hit {
		t.Fatalf("failed load = (%v, %v), want loader error", hit, err)
	}
	if metrics := cache.Metrics(); metrics.LoadFailures != 1 || metrics.Entries != 0 {
		t.Fatalf("metrics = %+v, want one load failure", metrics)
	}
}

func TestTodayContentCacheInvalidationRejectsStaleLoad(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		_, _, _ = cache.GetOrLoad(context.Background(), 1, "2026-08-28", now, func(context.Context) (learningdomain.TodayContent, error) {
			close(started)
			<-release
			return testTodayContent("2026-08-28", "stale"), nil
		})
	}()
	<-started
	cache.InvalidateGroup(1)
	close(release)
	<-done

	metrics := cache.Metrics()
	if metrics.Entries != 0 || metrics.StaleLoads != 1 {
		t.Fatalf("metrics = %+v, want stale load rejected", metrics)
	}
}

func TestRefreshTodayContentInvalidatesAndQueuesPrewarm(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, location)
	a := &app{
		todayCache:    newTodayContentCache(4, 1<<20, location),
		pdfRangeCache: newPDFRangeCache(4, 1<<20),
		cacheRefresh:  make(chan uint64, 1),
	}
	_, _, _ = a.todayCache.GetOrLoad(context.Background(), 7, "2026-08-28", now, func(context.Context) (learningdomain.TodayContent, error) {
		return testTodayContent("2026-08-28", "cached"), nil
	})
	a.pdfRangeCache.Put(pdfRangeCacheKey{groupID: 7, assetID: 10, pages: "1-2"}, []byte("%PDF"))

	a.refreshTodayContent(7)

	if metrics := a.todayCache.Metrics(); metrics.Entries != 0 {
		t.Fatalf("today cache entries = %d, want 0", metrics.Entries)
	}
	if metrics := a.pdfRangeCache.Metrics(); metrics.Entries != 0 {
		t.Fatalf("PDF cache entries = %d, want 0", metrics.Entries)
	}
	select {
	case groupID := <-a.cacheRefresh:
		if groupID != 7 {
			t.Fatalf("queued group = %d, want 7", groupID)
		}
	default:
		t.Fatal("cache refresh was not queued")
	}
}

func testTodayContent(date, label string) learningdomain.TodayContent {
	return learningdomain.TodayContent{
		Date:       date,
		Title:      "今日学习",
		Settings:   map[string]any{"label": label},
		RecordFrom: date,
		RecordTo:   date,
	}
}
