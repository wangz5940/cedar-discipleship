package server

import (
	"context"
	"net/http"
	"time"

	learningdomain "agp/backend/internal/learning"
)

func (a *app) handleToday(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	date := queryDate(r, "date", time.Now().In(a.location))
	now := time.Now().In(a.location)
	content, cacheStatus, err := a.todayContent(r.Context(), groupID, date, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "today_failed")
		return
	}
	hub, err := a.learning.TodayHubFromContent(r.Context(), groupID, u.ID, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "today_failed")
		return
	}
	w.Header().Set("X-AGP-Today-Cache", cacheStatus)
	writeJSON(w, http.StatusOK, hub)
}

func (a *app) todayContent(
	ctx context.Context,
	groupID uint64,
	date string,
	now time.Time,
) (learningdomain.TodayContent, string, error) {
	load := func(ctx context.Context) (learningdomain.TodayContent, error) {
		settings, err := a.groupLearningConfig(ctx, groupID)
		if err != nil {
			return learningdomain.TodayContent{}, err
		}
		return a.learning.TodayContent(ctx, groupID, date, settings, now)
	}
	if a.todayCache == nil || date != now.Format("2006-01-02") {
		content, err := load(ctx)
		return content, "BYPASS", err
	}
	content, hit, err := a.todayCache.GetOrLoad(ctx, groupID, date, now, load)
	if hit {
		return content, "HIT", err
	}
	return content, "MISS", err
}

func (a *app) handleTodayCacheMetrics(w http.ResponseWriter, r *http.Request) {
	contentMetrics := todayCacheMetrics{}
	if a.todayCache != nil {
		contentMetrics = a.todayCache.Metrics()
	}
	pdfMetrics := pdfRangeCacheMetrics{}
	if a.pdfRangeCache != nil {
		pdfMetrics = a.pdfRangeCache.Metrics()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"content":    contentMetrics,
		"pdf_ranges": pdfMetrics,
	})
}

func (a *app) handleClearTodayCache(w http.ResponseWriter, r *http.Request) {
	groupID := requireGroupID(w, mustUser(r))
	if groupID == 0 {
		return
	}
	removed := a.invalidateTodayContent(groupID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"removed_entries": removed,
	})
}

func (a *app) invalidateTodayContent(groupID uint64) int {
	removed := 0
	if a.todayCache != nil {
		removed += a.todayCache.InvalidateGroup(groupID)
	}
	if a.pdfRangeCache != nil {
		removed += a.pdfRangeCache.InvalidateGroup(groupID)
	}
	return removed
}

func (a *app) refreshTodayContent(groupID uint64) {
	a.invalidateTodayContent(groupID)
	if a.cacheRefresh == nil {
		return
	}
	select {
	case a.cacheRefresh <- groupID:
	default:
	}
}
