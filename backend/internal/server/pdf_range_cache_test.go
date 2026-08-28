package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPDFRangeCacheBoundsAndProtectsPayload(t *testing.T) {
	t.Parallel()

	cache := newPDFRangeCache(1, 16)
	firstKey := pdfRangeCacheKey{groupID: 1, assetID: 10, pages: "2-3", sourceSize: 100, sourceModUnix: 1}
	payload := []byte("%PDF-first")
	if !cache.Put(firstKey, payload) {
		t.Fatal("Put rejected valid payload")
	}
	payload[0] = 'X'
	got, ok := cache.Get(firstKey)
	if !ok || string(got) != "%PDF-first" {
		t.Fatalf("Get = %q, %v; want immutable cached payload", got, ok)
	}

	secondKey := pdfRangeCacheKey{groupID: 1, assetID: 11, pages: "4-5", sourceSize: 100, sourceModUnix: 1}
	if !cache.Put(secondKey, []byte("%PDF-second")) {
		t.Fatal("Put rejected second valid payload")
	}
	if _, ok := cache.Get(firstKey); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if metrics := cache.Metrics(); metrics.Entries != 1 || metrics.Evictions != 1 {
		t.Fatalf("metrics = %+v, want one entry and one eviction", metrics)
	}
}

func TestPDFRangeCacheRejectsOversizedAndCorruptEntries(t *testing.T) {
	t.Parallel()

	cache := newPDFRangeCache(2, 8)
	key := pdfRangeCacheKey{groupID: 1, assetID: 10, pages: "2-3", sourceSize: 100, sourceModUnix: 1}
	if cache.Put(key, []byte("too-large")) {
		t.Fatal("Put accepted oversized payload")
	}

	cache = newPDFRangeCache(2, 1024)
	if !cache.Put(key, []byte("%PDF-valid")) {
		t.Fatal("Put rejected valid payload")
	}
	cache.mu.Lock()
	cache.entries[key].Value.(*pdfRangeCacheEntry).payload[0] = 'X'
	cache.mu.Unlock()
	if _, ok := cache.Get(key); ok {
		t.Fatal("Get accepted corrupted payload")
	}
	if metrics := cache.Metrics(); metrics.Corruptions != 1 {
		t.Fatalf("corruptions = %d, want 1", metrics.Corruptions)
	}
}

func TestStudyTaskPageRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		task map[string]any
		want string
	}{
		{
			name: "title range",
			task: map[string]any{"title": "《基督是一切》36-40页"},
			want: "36-40",
		},
		{
			name: "single page",
			task: map[string]any{"title": "第 8 页"},
			want: "8-8",
		},
		{
			name: "legacy JSON metadata",
			task: map[string]any{"content": `{"page_start":88,"page_end":96}`},
			want: "88-96",
		},
		{
			name: "missing range",
			task: map[string]any{"title": "完整读物"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := studyTaskPageRange(tt.task); got != tt.want {
				t.Fatalf("studyTaskPageRange() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStudyTitleKeyMatchesPageScopedTaskToAsset(t *testing.T) {
	t.Parallel()

	taskTitle := studyTitleKey("《圣经救赎史剧综览-2》 10-11页")
	assetTitle := studyTitleKey("圣经救赎史剧综览-2")
	if taskTitle != assetTitle {
		t.Fatalf("task key = %q, asset key = %q", taskTitle, assetTitle)
	}

	index := map[string]uint64{}
	addStudyPDFAssetIndex(index, assetTitle, 10)
	addStudyPDFAssetIndex(index, assetTitle, 11)
	if index[assetTitle] != 0 {
		t.Fatalf("ambiguous asset key = %d, want disabled match", index[assetTitle])
	}
}

func TestServePDFRangeBytesSupportsHTTPRange(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/book.pdf", nil)
	request.Header.Set("Range", "bytes=0-3")
	recorder := httptest.NewRecorder()

	servePDFRangeBytes(
		recorder,
		request,
		"book.pdf",
		"8-9",
		time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC),
		[]byte("%PDF-test"),
	)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPartialContent)
	}
	if got := recorder.Body.String(); got != "%PDF" {
		t.Fatalf("body = %q, want PDF prefix", got)
	}
	if recorder.Header().Get("ETag") == "" {
		t.Fatal("ETag is empty")
	}
}
