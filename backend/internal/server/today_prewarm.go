package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	learningdomain "agp/backend/internal/learning"
)

var studyPageRangePattern = regexp.MustCompile(`([0-9]{1,4})[[:space:]]*(?:[-~—–至到][[:space:]]*([0-9]{1,4}))?[[:space:]]*页`)

func (a *app) runTodayCacheMaintenance(ctx context.Context) {
	a.prewarmTodayCaches(ctx, time.Now().In(a.location))
	timer := time.NewTimer(time.Until(nextMidnight(time.Now().In(a.location))))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case tick := <-timer.C:
			a.todayCache.Clear()
			a.pdfRangeCache.Clear()
			a.prewarmTodayCaches(ctx, tick.In(a.location))
			timer.Reset(time.Until(nextMidnight(time.Now().In(a.location))))
		case groupID := <-a.cacheRefresh:
			a.prewarmTodayGroup(ctx, groupID, time.Now().In(a.location))
		}
	}
}

func (a *app) prewarmTodayCaches(ctx context.Context, now time.Time) {
	groupIDs, err := a.activeStudyGroupIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "today cache prewarm failed", "error", err)
		return
	}
	date := now.Format("2006-01-02")
	groupsLoaded := 0
	pdfRangesLoaded := 0
	for _, groupID := range groupIDs {
		if err := ctx.Err(); err != nil {
			return
		}
		pdfRanges, ok := a.prewarmTodayGroup(ctx, groupID, now)
		if ok {
			groupsLoaded++
			pdfRangesLoaded += pdfRanges
		}
	}
	slog.InfoContext(
		ctx,
		"today cache prewarm completed",
		"date", date,
		"groups", groupsLoaded,
		"pdf_ranges", pdfRangesLoaded,
	)
}

func (a *app) prewarmTodayGroup(ctx context.Context, groupID uint64, now time.Time) (int, bool) {
	content, _, err := a.todayContent(ctx, groupID, now.Format("2006-01-02"), now)
	if err != nil {
		slog.WarnContext(ctx, "today group cache prewarm failed", "group_id", groupID, "error", err)
		return 0, false
	}
	return a.prewarmTodayPDFRanges(ctx, groupID, content), true
}

func (a *app) activeStudyGroupIDs(ctx context.Context) ([]uint64, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM study_groups WHERE status=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groupIDs []uint64
	for rows.Next() {
		var groupID uint64
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs, rows.Err()
}

func (a *app) prewarmTodayPDFRanges(ctx context.Context, groupID uint64, content learningdomain.TodayContent) int {
	loaded := 0
	var fallbackAssets map[string]uint64
	for _, task := range content.WeekTasks {
		if strings.TrimSpace(anyString(task["task_type"])) != "weekly_book" {
			continue
		}
		pages := studyTaskPageRange(task)
		assetID := firstStudyTaskAssetID(task["assets"])
		if pages != "" && assetID == 0 {
			if fallbackAssets == nil {
				fallbackAssets = a.studyPDFAssetIndex(ctx, groupID)
			}
			assetID = fallbackAssets[studyTitleKey(anyString(task["title"]))]
		}
		if pages == "" || assetID == 0 {
			continue
		}
		file, err := a.assets.DownloadFile(ctx, groupID, assetID)
		if err != nil || strings.ToLower(filepath.Ext(file.OriginalName)) != ".pdf" {
			continue
		}
		info, err := os.Stat(file.AbsolutePath)
		if err != nil {
			continue
		}
		key := pdfRangeCacheKey{
			groupID:       groupID,
			assetID:       assetID,
			pages:         pages,
			sourceSize:    info.Size(),
			sourceModUnix: info.ModTime().UnixNano(),
		}
		if a.pdfRangeCache.Has(key) {
			continue
		}
		payload, err := trimPDFRange(file.AbsolutePath, pages)
		if err != nil {
			slog.WarnContext(
				ctx,
				"today PDF range prewarm failed",
				"group_id", groupID,
				"asset_id", assetID,
				"pages", pages,
				"error", err,
			)
			continue
		}
		if a.pdfRangeCache.Put(key, payload) {
			loaded++
		}
	}
	return loaded
}

func (a *app) studyPDFAssetIndex(ctx context.Context, groupID uint64) map[string]uint64 {
	sections, err := a.assets.ResourceLibrary(ctx, groupID)
	if err != nil {
		return nil
	}
	index := make(map[string]uint64)
	for _, section := range sections {
		for _, item := range section.Items {
			if strings.ToLower(filepath.Ext(item.OriginalName)) != ".pdf" {
				continue
			}
			addStudyPDFAssetIndex(index, studyTitleKey(item.Title), item.ID)
			baseName := strings.TrimSuffix(filepath.Base(item.OriginalName), filepath.Ext(item.OriginalName))
			addStudyPDFAssetIndex(index, studyTitleKey(baseName), item.ID)
		}
	}
	return index
}

func addStudyPDFAssetIndex(index map[string]uint64, key string, assetID uint64) {
	if key == "" || assetID == 0 {
		return
	}
	if existing, ok := index[key]; ok && existing != assetID {
		index[key] = 0
		return
	}
	index[key] = assetID
}

func studyTitleKey(value string) string {
	value = studyPageRangePattern.ReplaceAllString(value, "")
	var out strings.Builder
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			out.WriteRune(char)
		}
	}
	return out.String()
}

func studyTaskPageRange(task map[string]any) string {
	if match := studyPageRangePattern.FindStringSubmatch(anyString(task["title"])); len(match) > 0 {
		end := match[2]
		if end == "" {
			end = match[1]
		}
		pages, _ := normalizePageRange(match[1] + "-" + end)
		return pages
	}
	var metadata struct {
		PageStart int `json:"page_start"`
		PageEnd   int `json:"page_end"`
	}
	if json.Unmarshal([]byte(anyString(task["content"])), &metadata) != nil || metadata.PageStart < 1 {
		return ""
	}
	if metadata.PageEnd < metadata.PageStart {
		metadata.PageEnd = metadata.PageStart
	}
	pages, _ := normalizePageRange(strconv.Itoa(metadata.PageStart) + "-" + strconv.Itoa(metadata.PageEnd))
	return pages
}

func firstStudyTaskAssetID(raw any) uint64 {
	var asset map[string]any
	switch items := raw.(type) {
	case []map[string]any:
		if len(items) > 0 {
			asset = items[0]
		}
	case []any:
		if len(items) > 0 {
			asset, _ = items[0].(map[string]any)
		}
	}
	if asset == nil {
		return 0
	}
	switch value := asset["id"].(type) {
	case uint64:
		return value
	case int:
		if value > 0 {
			return uint64(value)
		}
	case float64:
		if value > 0 {
			return uint64(value)
		}
	}
	return 0
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}
