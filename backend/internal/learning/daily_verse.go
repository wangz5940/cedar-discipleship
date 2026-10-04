package learning

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidDailyVerse = errors.New("invalid_daily_verse")
var ErrLearningConfigConflict = errors.New("learning_config_conflict")

// DailyVersePlan resolves an explicitly configured date or independent date range.
func DailyVersePlan(settings map[string]any, date string) (map[string]any, bool) {
	config, ok := nestedMap(settings, "task_sections", "daily", "verse")
	if !ok || !mapBool(config, "enabled", false) || date == "" {
		return nil, false
	}
	for _, plan := range versePlans(config) {
		from, to := VersePlanRange(plan)
		if from <= date && date <= to {
			return plan, true
		}
	}
	return nil, false
}

func versePlans(config map[string]any) []map[string]any {
	var plans []map[string]any
	switch value := config["plans"].(type) {
	case []map[string]any:
		return value
	case []any:
		for _, item := range value {
			if plan, ok := item.(map[string]any); ok {
				plans = append(plans, plan)
			}
		}
	}
	return plans
}

func VersePlanRange(plan map[string]any) (string, string) {
	from := asString(plan["date"])
	return from, firstNonEmpty(asString(plan["end_date"]), from)
}

func WeeklyVersePlan(plan map[string]any) bool {
	return asString(plan["completion_mode"]) == "weekly"
}

func verseRecordRange(settings map[string]any, date, from, to string) (string, string) {
	if plan, ok := DailyVersePlan(settings, date); ok && WeeklyVersePlan(plan) {
		start, end := VersePlanRange(plan)
		if start < from {
			from = start
		}
		if end > to {
			to = end
		}
	}
	return from, to
}

func ValidateDailyVerse(settings map[string]any) error {
	config, ok := nestedMap(settings, "task_sections", "daily", "verse")
	if !ok {
		return nil
	}
	var plans []any
	switch value := config["plans"].(type) {
	case nil:
		return nil
	case []any:
		plans = value
	case []map[string]any:
		for _, plan := range value {
			plans = append(plans, plan)
		}
	default:
		return ErrInvalidDailyVerse
	}
	var ranges [][2]string
	for _, item := range plans {
		plan, ok := item.(map[string]any)
		date := asString(plan["date"])
		start, err := time.Parse("2006-01-02", date)
		from, to := VersePlanRange(plan)
		end, endErr := time.Parse("2006-01-02", to)
		mode := asString(plan["completion_mode"])
		ref := strings.TrimSpace(asString(plan["verse_ref"]))
		text := strings.TrimSpace(asString(plan["recite_text"]))
		if !ok || err != nil || endErr != nil || end.Before(start) ||
			(mode != "" && mode != "daily" && mode != "weekly") ||
			(mode == "weekly" && end.Sub(start) > 6*24*time.Hour) || ref == "" || text == "" ||
			utf8.RuneCountInString(ref) > 255 || utf8.RuneCountInString(text) > 10000 {
			return ErrInvalidDailyVerse
		}
		for _, existing := range ranges {
			if from <= existing[1] && to >= existing[0] {
				return ErrInvalidDailyVerse
			}
		}
		ranges = append(ranges, [2]string{from, to})
	}
	return nil
}
