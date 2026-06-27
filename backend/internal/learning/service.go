package learning

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"agp/backend/internal/checkin"
	"agp/backend/internal/progress"
)

type Service struct {
	repo     Repository
	progress *progress.Service
	checkins *checkin.Service
}

func NewService(repo Repository, progressSvc *progress.Service, checkinSvc *checkin.Service) *Service {
	return &Service{repo: repo, progress: progressSvc, checkins: checkinSvc}
}

func (s *Service) ListWeeks(ctx context.Context, groupID uint64) ([]WeekVO, error) {
	weeks, err := s.repo.ListWeeks(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]WeekVO, 0, len(weeks))
	for _, week := range weeks {
		tasks, err := s.repo.ListTasks(ctx, groupID, week.ID)
		if err != nil {
			return nil, err
		}
		readings, videos, outline := SplitWeekTaskBindings(TaskMaps(tasks))
		out = append(out, weekVO(week, readings, videos, outline))
	}
	return out, nil
}

func (s *Service) ListWeekInputs(ctx context.Context, groupID uint64) ([]WeekInput, error) {
	weeks, err := s.ListWeeks(ctx, groupID)
	if err != nil {
		return nil, err
	}
	inputs := make([]WeekInput, 0, len(weeks))
	for _, week := range weeks {
		inputs = append(inputs, WeekInput{
			StartDate:      week.Start,
			EndDate:        week.End,
			Title:          week.Title,
			VerseRef:       week.VerseRef,
			ReciteText:     week.ReciteText,
			BookEnabled:    week.BookEnabled,
			VideoEnabled:   week.VideoEnabled,
			VerseEnabled:   week.VerseEnabled,
			OutlineEnabled: week.OutlineEnabled,
			Readings:       week.Readings,
			Videos:         week.Videos,
			Outline:        week.Outline,
		})
	}
	return inputs, nil
}

func (s *Service) CurrentWeek(ctx context.Context, groupID uint64, date string) (map[string]any, error) {
	week, err := s.repo.CurrentWeek(ctx, groupID, date)
	if err != nil {
		return nil, err
	}
	return WeekMap(*week), nil
}

func (s *Service) WeekTasks(ctx context.Context, groupID, weekID uint64) ([]map[string]any, error) {
	tasks, err := s.repo.ListTasks(ctx, groupID, weekID)
	if err != nil {
		return nil, err
	}
	return TaskMaps(tasks), nil
}

func (s *Service) SaveWeek(ctx context.Context, groupID, weekID uint64, input WeekInput, now time.Time) (uint64, error) {
	existingVerseTitle := ""
	if weekID > 0 {
		title, err := s.repo.ExistingTaskTitle(ctx, groupID, weekID, "weekly_verse")
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		existingVerseTitle = title
	}
	tasks := BuildTaskDrafts(input, existingVerseTitle)
	return s.repo.SaveWeek(ctx, groupID, weekID, input, tasks, now)
}

func (s *Service) DeleteWeek(ctx context.Context, groupID, weekID uint64) error {
	return s.repo.DeleteWeek(ctx, groupID, weekID)
}

func (s *Service) TodayHub(ctx context.Context, groupID, userID uint64, date string, settings map[string]any, now time.Time) (TodayVO, error) {
	var week map[string]any
	weekModel, err := s.repo.CurrentWeek(ctx, groupID, date)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return TodayVO{}, err
		}
		week = nil
	} else {
		week = WeekMap(*weekModel)
	}

	var weekTasks []map[string]any
	if weekID := mapUint64(week, "id"); weekID > 0 {
		tasks, err := s.repo.ListTasks(ctx, groupID, weekID)
		if err != nil {
			return TodayVO{}, err
		}
		weekTasks = TaskMaps(tasks)
	}

	from, to := date, date
	if start, end := asString(week["start"]), asString(week["end"]); start != "" && end != "" {
		from, to = start, end
	}
	records, err := s.repo.ListTodayRecords(ctx, groupID, userID, from, to)
	if err != nil {
		return TodayVO{}, err
	}

	tasks := buildTodayTasks(date, week, weekTasks, settings, records)
	completed := 0
	for _, task := range tasks {
		if task.Completed {
			completed++
		}
	}
	total := len(tasks)
	percent := 0
	if total > 0 {
		percent = int(math.Round(float64(completed) / float64(total) * 100))
	}

	title := "今日学习"
	if date != now.Format("2006-01-02") {
		title = "学习回顾"
	}
	subtitle := "打开内容完成学习，系统会记录到你的今日进度。"
	if week != nil {
		subtitle = fmt.Sprintf("%s - %s · %s", asString(week["start"]), asString(week["end"]), firstNonEmpty(asString(week["title"]), "本周学习计划"))
	}

	return TodayVO{
		Date:        date,
		Title:       title,
		Subtitle:    subtitle,
		CurrentWeek: week,
		Progress:    TodayProgress{Completed: completed, Total: total, Percent: percent},
		Tasks:       tasks,
		Records:     records,
	}, nil
}

func (s *Service) LearningConfig(ctx context.Context, groupID uint64) (map[string]any, error) {
	return s.repo.LearningConfig(ctx, groupID)
}

func (s *Service) SaveLearningConfig(ctx context.Context, groupID uint64, settings map[string]any) error {
	return s.repo.SaveLearningConfig(ctx, groupID, settings)
}

func BuildTaskDrafts(input WeekInput, existingVerseTitle string) []TaskDraft {
	var tasks []TaskDraft
	if input.BookEnabled {
		order := 1
		for _, reading := range input.Readings {
			if strings.TrimSpace(reading.Title) == "" && reading.AssetID == 0 && strings.TrimSpace(reading.URL) == "" {
				continue
			}
			tasks = append(tasks, TaskDraft{
				TaskType:  "weekly_book",
				Title:     firstNonEmpty(strings.TrimSpace(reading.Title), "周读物"),
				Content:   strings.TrimSpace(reading.URL),
				SortOrder: order,
				AssetID:   reading.AssetID,
				UsageType: "reading",
			})
			order++
		}
	}
	if input.VideoEnabled {
		order := 1
		for _, video := range input.Videos {
			if strings.TrimSpace(video.Title) == "" && video.AssetID == 0 && strings.TrimSpace(video.URL) == "" {
				continue
			}
			tasks = append(tasks, TaskDraft{
				TaskType:  "weekly_video",
				Title:     firstNonEmpty(strings.TrimSpace(video.Title), "本周视频"),
				Content:   strings.TrimSpace(video.URL),
				SortOrder: order,
				AssetID:   video.AssetID,
				UsageType: "video",
			})
			break
		}
	}
	if verseTitle := WeeklyVerseTaskTitle(input, existingVerseTitle); verseTitle != "" {
		tasks = append(tasks, TaskDraft{
			TaskType:  "weekly_verse",
			Title:     verseTitle,
			Content:   strings.TrimSpace(input.ReciteText),
			SortOrder: 1,
		})
	}
	if input.OutlineEnabled && (strings.TrimSpace(input.Outline.Title) != "" || input.Outline.AssetID > 0 || strings.TrimSpace(input.Outline.URL) != "") {
		tasks = append(tasks, TaskDraft{
			TaskType:  "weekly_outline",
			Title:     firstNonEmpty(strings.TrimSpace(input.Outline.Title), "提纲背诵"),
			Content:   strings.TrimSpace(input.Outline.URL),
			SortOrder: 1,
			AssetID:   input.Outline.AssetID,
			UsageType: "outline",
		})
	}
	return tasks
}

func WeeklyVerseTaskTitle(input WeekInput, existingTitle string) string {
	if !input.VerseEnabled {
		return ""
	}
	return firstNonEmpty(strings.TrimSpace(input.VerseRef), strings.TrimSpace(existingTitle), "背经")
}

func SplitWeekTaskBindings(tasks []map[string]any) ([]TaskBinding, []TaskBinding, TaskBinding) {
	var readings []TaskBinding
	var videos []TaskBinding
	var outline TaskBinding
	for _, task := range tasks {
		binding := taskBindingFromMap(task)
		switch asString(task["task_type"]) {
		case "weekly_book":
			if binding.Title != "" || binding.URL != "" || binding.AssetID > 0 {
				readings = append(readings, binding)
			}
		case "weekly_video":
			if binding.Title != "" || binding.URL != "" || binding.AssetID > 0 {
				videos = append(videos, binding)
			}
		case "weekly_outline":
			outline = binding
		}
	}
	return readings, videos, outline
}

func WeekMap(week Week) map[string]any {
	return map[string]any{
		"id":              week.ID,
		"start":           week.StartDate,
		"end":             week.EndDate,
		"title":           week.Title,
		"verse_ref":       week.VerseRef,
		"recite_text":     week.ReciteText,
		"book_enabled":    week.BookEnabled,
		"video_enabled":   week.VideoEnabled,
		"verse_enabled":   week.VerseEnabled,
		"outline_enabled": week.OutlineEnabled,
	}
}

func TaskMaps(tasks []Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, map[string]any{
			"id":        task.ID,
			"task_type": task.TaskType,
			"title":     task.Title,
			"content":   task.Content,
			"enabled":   task.Enabled,
			"assets":    taskAssetMaps(task.Assets),
		})
	}
	return out
}

func buildTodayTasks(date string, week map[string]any, rawTasks []map[string]any, settings map[string]any, records []TodayRecord) []TodayTaskVO {
	weekID := mapUint64(week, "id")
	tasks := []TodayTaskVO{
		{
			ID:       "daily_devotion",
			Type:     "daily_devotion",
			Kind:     "devotion",
			Title:    nestedString(settings, []string{"task_sections", "daily", "label"}, "每日灵修"),
			Summary:  "今天的灵修与读经",
			Detail:   nestedString(settings, []string{"task_sections", "daily", "label"}, "每日灵修"),
			Required: true,
			Status:   "pending",
		},
	}

	if week != nil {
		for _, raw := range rawTasks {
			taskType := asString(raw["task_type"])
			switch taskType {
			case "weekly_book":
				if !mapBool(week, "book_enabled", true) {
					continue
				}
			case "weekly_video":
				if !mapBool(week, "video_enabled", true) {
					continue
				}
			case "weekly_verse":
				if !mapBool(week, "verse_enabled", true) {
					continue
				}
			default:
				continue
			}
			title := firstNonEmpty(asString(raw["title"]), todayTaskFallbackTitle(taskType))
			tasks = append(tasks, TodayTaskVO{
				ID:       todayTaskID(taskType, mapUint64(raw, "id"), title),
				Type:     taskType,
				Kind:     todayTaskKind(taskType),
				Title:    title,
				Summary:  todayTaskSummary(taskType),
				TaskID:   mapUint64(raw, "id"),
				WeekID:   weekID,
				Part:     todayTaskPart(taskType, title),
				Detail:   title,
				Content:  asString(raw["content"]),
				Required: true,
				Status:   "pending",
				Assets:   todayTaskAssets(raw["assets"]),
			})
		}

		if mapBool(week, "verse_enabled", true) && !hasTodayTaskType(tasks, "weekly_verse") {
			if verse := firstNonEmpty(asString(week["verse_ref"]), asString(week["recite_text"])); verse != "" {
				tasks = append(tasks, TodayTaskVO{
					ID:       "weekly_verse",
					Type:     "weekly_verse",
					Kind:     "verse",
					Title:    verse,
					Summary:  todayTaskSummary("weekly_verse"),
					WeekID:   weekID,
					Detail:   verse,
					Required: true,
					Status:   "pending",
				})
			}
		}
	}

	for index := range tasks {
		if record := matchingTodayRecord(tasks[index], records, date); record != nil {
			tasks[index].Completed = true
			tasks[index].Status = "done"
			tasks[index].Record = record
		}
	}
	return tasks
}

func todayTaskID(taskType string, taskID uint64, title string) string {
	if taskID > 0 {
		return fmt.Sprintf("%s:%d", taskType, taskID)
	}
	return fmt.Sprintf("%s:%s", taskType, strings.TrimSpace(title))
}

func todayTaskKind(taskType string) string {
	switch taskType {
	case "daily_devotion":
		return "devotion"
	case "weekly_book":
		return "book"
	case "weekly_video":
		return "video"
	case "weekly_verse":
		return "verse"
	default:
		return "activity"
	}
}

func todayTaskSummary(taskType string) string {
	switch taskType {
	case "weekly_book":
		return "本周阅读"
	case "weekly_video":
		return "本周视频"
	case "weekly_verse":
		return "背经与默想"
	default:
		return "今日学习"
	}
}

func todayTaskFallbackTitle(taskType string) string {
	switch taskType {
	case "weekly_book":
		return "周读物"
	case "weekly_video":
		return "本周视频"
	case "weekly_verse":
		return "本周背经"
	default:
		return "学习任务"
	}
}

func todayTaskPart(taskType, title string) string {
	if taskType == "weekly_book" {
		return strings.TrimSpace(title)
	}
	return ""
}

func todayTaskAssets(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		if len(items) == 0 {
			return nil
		}
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if asset, ok := item.(map[string]any); ok {
				out = append(out, asset)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func hasTodayTaskType(tasks []TodayTaskVO, taskType string) bool {
	for _, task := range tasks {
		if task.Type == taskType {
			return true
		}
	}
	return false
}

func matchingTodayRecord(task TodayTaskVO, records []TodayRecord, date string) *TodayRecord {
	for i := range records {
		record := &records[i]
		if record.TaskType != task.Type {
			continue
		}
		if task.Type == "weekly_book" {
			if task.TaskID > 0 && record.TaskID != nil && *record.TaskID == task.TaskID {
				return record
			}
			title := firstNonEmpty(task.Part, task.Detail, task.Title)
			if title != "" && (record.Part == title || record.Detail == title) {
				return record
			}
			continue
		}
		if task.Type == "weekly_video" || task.Type == "weekly_verse" {
			if task.TaskID > 0 && record.TaskID != nil && *record.TaskID == task.TaskID {
				return record
			}
			if task.WeekID > 0 && record.WeekID != nil && *record.WeekID == task.WeekID {
				return record
			}
			continue
		}
		if record.LogicalDate != date {
			continue
		}
		if task.TaskID > 0 && record.TaskID != nil && *record.TaskID == task.TaskID {
			return record
		}
		if task.Part != "" {
			if record.Part == task.Part || record.Detail == task.Detail {
				return record
			}
			continue
		}
		if record.Part == "" {
			return record
		}
	}
	return nil
}

func weekVO(week Week, readings, videos []TaskBinding, outline TaskBinding) WeekVO {
	return WeekVO{
		ID:             week.ID,
		Start:          week.StartDate,
		End:            week.EndDate,
		Title:          week.Title,
		VerseRef:       week.VerseRef,
		ReciteText:     week.ReciteText,
		BookEnabled:    week.BookEnabled,
		VideoEnabled:   week.VideoEnabled,
		VerseEnabled:   week.VerseEnabled,
		OutlineEnabled: week.OutlineEnabled,
		Readings:       readings,
		Videos:         videos,
		Outline:        outline,
	}
}

func taskBindingFromMap(task map[string]any) TaskBinding {
	binding := TaskBinding{
		Title: asString(task["title"]),
		URL:   asString(task["content"]),
		Type:  InferTaskBindingType(asString(task["task_type"]), asString(task["content"]), ""),
	}
	if asset := firstTaskAsset(task["assets"]); asset != nil {
		if id, ok := asset["id"].(uint64); ok {
			binding.AssetID = id
		}
		if binding.Title == "" {
			binding.Title = firstNonEmpty(asString(asset["title"]), asString(asset["original_name"]))
		}
		binding.Type = InferTaskBindingType(asString(task["task_type"]), binding.URL, firstNonEmpty(asString(asset["original_name"]), asString(asset["title"])))
	}
	return binding
}

func firstTaskAsset(raw any) map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		if len(items) > 0 {
			return items[0]
		}
	case []any:
		if len(items) > 0 {
			if asset, ok := items[0].(map[string]any); ok {
				return asset
			}
		}
	}
	return nil
}

func InferTaskBindingType(taskType, urlValue, fileName string) string {
	switch taskType {
	case "weekly_video":
		return "video"
	case "weekly_outline":
		return "image"
	}
	value := strings.ToLower(firstNonEmpty(fileName, urlValue))
	switch {
	case strings.HasSuffix(value, ".md"):
		return "markdown"
	case strings.HasSuffix(value, ".mp4"), strings.HasSuffix(value, ".webm"), strings.HasSuffix(value, ".mov"), strings.HasSuffix(value, ".m4v"):
		return "video"
	case strings.HasSuffix(value, ".png"), strings.HasSuffix(value, ".jpg"), strings.HasSuffix(value, ".jpeg"), strings.HasSuffix(value, ".webp"):
		return "image"
	default:
		return "pdf"
	}
}

func taskAssetMaps(assets []TaskAsset) []map[string]any {
	if len(assets) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		out = append(out, map[string]any{
			"id":            asset.ID,
			"category":      asset.Category,
			"title":         asset.Title,
			"original_name": asset.OriginalName,
			"usage_type":    asset.UsageType,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func mapUint64(m map[string]any, key string) uint64 {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case uint64:
		return v
	case uint:
		return uint64(v)
	case int:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case float64:
		if v > 0 {
			return uint64(v)
		}
	}
	return 0
}

func mapBool(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	switch v := m[key].(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case uint64:
		return v != 0
	case float64:
		return v != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	return fallback
}

func nestedString(root map[string]any, path []string, fallback string) string {
	var current any = root
	for _, key := range path {
		values, ok := current.(map[string]any)
		if !ok {
			return fallback
		}
		current = values[key]
	}
	if value := strings.TrimSpace(asString(current)); value != "" {
		return value
	}
	return fallback
}

func asString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	default:
		return ""
	}
}
