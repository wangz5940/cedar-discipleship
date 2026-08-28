package server

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	learningdomain "agp/backend/internal/learning"
)

const (
	defaultTodayCacheMaxEntries = 256
	defaultTodayCacheMaxBytes   = 32 << 20
)

type todayCacheKey struct {
	groupID uint64
	date    string
}

type todayCacheEntry struct {
	key       todayCacheKey
	payload   []byte
	checksum  [sha256.Size]byte
	expiresAt time.Time
}

type todayContentCache struct {
	mu          sync.Mutex
	entries     map[todayCacheKey]*list.Element
	lru         *list.List
	sizeBytes   int64
	maxEntries  int
	maxBytes    int64
	location    *time.Location
	epoch       uint64
	generations map[uint64]uint64

	hits              uint64
	misses            uint64
	loads             uint64
	loadFailures      uint64
	evictions         uint64
	invalidations     uint64
	corruptions       uint64
	rejected          uint64
	staleLoads        uint64
	hitDurationTotal  time.Duration
	loadDurationTotal time.Duration
}

type todayCacheMetrics struct {
	Entries                          int     `json:"entries"`
	SizeBytes                        int64   `json:"size_bytes"`
	MaxEntries                       int     `json:"max_entries"`
	MaxBytes                         int64   `json:"max_bytes"`
	Hits                             uint64  `json:"hits"`
	Misses                           uint64  `json:"misses"`
	Loads                            uint64  `json:"loads"`
	LoadFailures                     uint64  `json:"load_failures"`
	Evictions                        uint64  `json:"evictions"`
	Invalidations                    uint64  `json:"invalidations"`
	Corruptions                      uint64  `json:"corruptions"`
	Rejected                         uint64  `json:"rejected"`
	StaleLoads                       uint64  `json:"stale_loads"`
	HitRate                          float64 `json:"hit_rate"`
	AverageHitMicroseconds           float64 `json:"average_hit_microseconds"`
	AverageLoadMilliseconds          float64 `json:"average_load_milliseconds"`
	EstimatedContentLoadReductionPct float64 `json:"estimated_content_load_reduction_percent"`
}

func newTodayContentCache(maxEntries int, maxBytes int64, location *time.Location) *todayContentCache {
	if maxEntries <= 0 {
		maxEntries = defaultTodayCacheMaxEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultTodayCacheMaxBytes
	}
	if location == nil {
		location = time.Local
	}
	return &todayContentCache{
		entries:     make(map[todayCacheKey]*list.Element, maxEntries),
		lru:         list.New(),
		maxEntries:  maxEntries,
		maxBytes:    maxBytes,
		location:    location,
		generations: make(map[uint64]uint64),
	}
}

func (c *todayContentCache) GetOrLoad(
	ctx context.Context,
	groupID uint64,
	date string,
	now time.Time,
	load func(context.Context) (learningdomain.TodayContent, error),
) (learningdomain.TodayContent, bool, error) {
	key := todayCacheKey{groupID: groupID, date: date}
	hitStarted := time.Now()
	if content, ok := c.get(key, now); ok {
		c.mu.Lock()
		c.hits++
		c.hitDurationTotal += time.Since(hitStarted)
		c.mu.Unlock()
		return content, true, nil
	}

	c.mu.Lock()
	c.misses++
	epoch := c.epoch
	generation := c.generations[groupID]
	c.mu.Unlock()

	loadStarted := time.Now()
	content, err := load(ctx)
	loadDuration := time.Since(loadStarted)
	c.mu.Lock()
	if err != nil {
		c.loadFailures++
		c.mu.Unlock()
		return learningdomain.TodayContent{}, false, err
	}
	c.loads++
	c.loadDurationTotal += loadDuration
	c.mu.Unlock()

	c.store(key, content, nextMidnight(now.In(c.location)), epoch, generation)
	return content, false, nil
}

func (c *todayContentCache) get(key todayCacheKey, now time.Time) (learningdomain.TodayContent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element := c.entries[key]
	if element == nil {
		return learningdomain.TodayContent{}, false
	}
	entry := element.Value.(*todayCacheEntry)
	if !now.Before(entry.expiresAt) {
		c.removeLocked(element)
		c.invalidations++
		return learningdomain.TodayContent{}, false
	}
	if sha256.Sum256(entry.payload) != entry.checksum {
		c.removeLocked(element)
		c.corruptions++
		return learningdomain.TodayContent{}, false
	}

	var content learningdomain.TodayContent
	if err := json.Unmarshal(entry.payload, &content); err != nil || !validTodayContent(key, content) {
		c.removeLocked(element)
		c.corruptions++
		return learningdomain.TodayContent{}, false
	}
	c.lru.MoveToFront(element)
	return content, true
}

func (c *todayContentCache) store(
	key todayCacheKey,
	content learningdomain.TodayContent,
	expiresAt time.Time,
	epoch uint64,
	generation uint64,
) {
	if !validTodayContent(key, content) {
		c.mu.Lock()
		c.rejected++
		c.mu.Unlock()
		return
	}
	payload, err := json.Marshal(content)
	if err != nil || int64(len(payload)) > c.maxBytes {
		c.mu.Lock()
		c.rejected++
		c.mu.Unlock()
		return
	}

	entry := &todayCacheEntry{
		key:       key,
		payload:   payload,
		checksum:  sha256.Sum256(payload),
		expiresAt: expiresAt,
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch != epoch || c.generations[key.groupID] != generation {
		c.staleLoads++
		return
	}
	if existing := c.entries[key]; existing != nil {
		c.removeLocked(existing)
	}
	c.entries[key] = c.lru.PushFront(entry)
	c.sizeBytes += int64(len(payload))
	for len(c.entries) > c.maxEntries || c.sizeBytes > c.maxBytes {
		c.removeLocked(c.lru.Back())
		c.evictions++
	}
}

func (c *todayContentCache) InvalidateGroup(groupID uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.generations[groupID]++
	removed := 0
	for key, element := range c.entries {
		if key.groupID != groupID {
			continue
		}
		c.removeLocked(element)
		removed++
	}
	c.invalidations += uint64(removed)
	return removed
}

func (c *todayContentCache) Clear() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := len(c.entries)
	c.clearLocked()
	c.epoch++
	c.invalidations += uint64(removed)
	return removed
}

func (c *todayContentCache) Metrics() todayCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics := todayCacheMetrics{
		Entries:       len(c.entries),
		SizeBytes:     c.sizeBytes,
		MaxEntries:    c.maxEntries,
		MaxBytes:      c.maxBytes,
		Hits:          c.hits,
		Misses:        c.misses,
		Loads:         c.loads,
		LoadFailures:  c.loadFailures,
		Evictions:     c.evictions,
		Invalidations: c.invalidations,
		Corruptions:   c.corruptions,
		Rejected:      c.rejected,
		StaleLoads:    c.staleLoads,
	}
	requests := c.hits + c.misses
	if requests > 0 {
		metrics.HitRate = float64(c.hits) / float64(requests)
	}
	if c.hits > 0 {
		metrics.AverageHitMicroseconds = float64(c.hitDurationTotal.Microseconds()) / float64(c.hits)
	}
	if c.loads > 0 {
		metrics.AverageLoadMilliseconds = float64(c.loadDurationTotal.Microseconds()) / 1000 / float64(c.loads)
	}
	if metrics.AverageLoadMilliseconds > 0 {
		hitMilliseconds := metrics.AverageHitMicroseconds / 1000
		reduction := (metrics.AverageLoadMilliseconds - hitMilliseconds) / metrics.AverageLoadMilliseconds * 100
		if reduction > 0 {
			metrics.EstimatedContentLoadReductionPct = reduction
		}
	}
	return metrics
}

func (c *todayContentCache) removeLocked(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*todayCacheEntry)
	delete(c.entries, entry.key)
	c.sizeBytes -= int64(len(entry.payload))
	c.lru.Remove(element)
}

func (c *todayContentCache) clearLocked() {
	clear(c.entries)
	c.lru.Init()
	c.sizeBytes = 0
}

func validTodayContent(key todayCacheKey, content learningdomain.TodayContent) bool {
	return content.Date == key.date &&
		content.Title != "" &&
		content.Settings != nil &&
		content.RecordFrom != "" &&
		content.RecordTo != ""
}

func nextMidnight(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}
