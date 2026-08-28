package server

import (
	"container/list"
	"crypto/sha256"
	"sync"
)

const (
	defaultPDFRangeCacheMaxEntries = 64
	defaultPDFRangeCacheMaxBytes   = 64 << 20
)

type pdfRangeCacheKey struct {
	groupID       uint64
	assetID       uint64
	pages         string
	sourceSize    int64
	sourceModUnix int64
}

type pdfRangeCacheEntry struct {
	key      pdfRangeCacheKey
	payload  []byte
	checksum [sha256.Size]byte
}

type pdfRangeCache struct {
	mu          sync.Mutex
	entries     map[pdfRangeCacheKey]*list.Element
	lru         *list.List
	sizeBytes   int64
	maxEntries  int
	maxBytes    int64
	hits        uint64
	misses      uint64
	evictions   uint64
	rejected    uint64
	corruptions uint64
}

type pdfRangeCacheMetrics struct {
	Entries     int     `json:"entries"`
	SizeBytes   int64   `json:"size_bytes"`
	MaxEntries  int     `json:"max_entries"`
	MaxBytes    int64   `json:"max_bytes"`
	Hits        uint64  `json:"hits"`
	Misses      uint64  `json:"misses"`
	Evictions   uint64  `json:"evictions"`
	Rejected    uint64  `json:"rejected"`
	Corruptions uint64  `json:"corruptions"`
	HitRate     float64 `json:"hit_rate"`
}

func newPDFRangeCache(maxEntries int, maxBytes int64) *pdfRangeCache {
	if maxEntries <= 0 {
		maxEntries = defaultPDFRangeCacheMaxEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultPDFRangeCacheMaxBytes
	}
	return &pdfRangeCache{
		entries:    make(map[pdfRangeCacheKey]*list.Element, maxEntries),
		lru:        list.New(),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
	}
}

func (c *pdfRangeCache) Get(key pdfRangeCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element := c.entries[key]
	if element == nil {
		c.misses++
		return nil, false
	}
	entry := element.Value.(*pdfRangeCacheEntry)
	if sha256.Sum256(entry.payload) != entry.checksum {
		c.removeLocked(element)
		c.corruptions++
		c.misses++
		return nil, false
	}
	c.lru.MoveToFront(element)
	c.hits++
	return entry.payload, true
}

func (c *pdfRangeCache) Put(key pdfRangeCacheKey, payload []byte) bool {
	if len(payload) == 0 || int64(len(payload)) > c.maxBytes {
		c.mu.Lock()
		c.rejected++
		c.mu.Unlock()
		return false
	}
	snapshot := append([]byte(nil), payload...)
	entry := &pdfRangeCacheEntry{
		key:      key,
		payload:  snapshot,
		checksum: sha256.Sum256(snapshot),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.entries[key]; existing != nil {
		c.removeLocked(existing)
	}
	c.entries[key] = c.lru.PushFront(entry)
	c.sizeBytes += int64(len(snapshot))
	for len(c.entries) > c.maxEntries || c.sizeBytes > c.maxBytes {
		c.removeLocked(c.lru.Back())
		c.evictions++
	}
	return c.entries[key] != nil
}

func (c *pdfRangeCache) Has(key pdfRangeCacheKey) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	element := c.entries[key]
	if element == nil {
		return false
	}
	c.lru.MoveToFront(element)
	return true
}

func (c *pdfRangeCache) InvalidateGroup(groupID uint64) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	for key, element := range c.entries {
		if key.groupID != groupID {
			continue
		}
		c.removeLocked(element)
		removed++
	}
	return removed
}

func (c *pdfRangeCache) Clear() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := len(c.entries)
	clear(c.entries)
	c.lru.Init()
	c.sizeBytes = 0
	return removed
}

func (c *pdfRangeCache) Metrics() pdfRangeCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics := pdfRangeCacheMetrics{
		Entries:     len(c.entries),
		SizeBytes:   c.sizeBytes,
		MaxEntries:  c.maxEntries,
		MaxBytes:    c.maxBytes,
		Hits:        c.hits,
		Misses:      c.misses,
		Evictions:   c.evictions,
		Rejected:    c.rejected,
		Corruptions: c.corruptions,
	}
	requests := c.hits + c.misses
	if requests > 0 {
		metrics.HitRate = float64(c.hits) / float64(requests)
	}
	return metrics
}

func (c *pdfRangeCache) removeLocked(element *list.Element) {
	if element == nil {
		return
	}
	entry := element.Value.(*pdfRangeCacheEntry)
	delete(c.entries, entry.key)
	c.sizeBytes -= int64(len(entry.payload))
	c.lru.Remove(element)
}
