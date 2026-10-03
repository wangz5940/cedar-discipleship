package learning

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidDailyVerse = errors.New("invalid_daily_verse")

// DailyVersePlan resolves only an explicitly configured date.
func DailyVersePlan(settings map[string]any, date string) (map[string]any, bool) {
	config, ok := nestedMap(settings, "task_sections", "daily", "verse")
	if !ok || !mapBool(config, "enabled", false) || date == "" {
		return nil, false
	}
	return customDailyDevotionPlan(config, date)
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
	dates := make(map[string]bool)
	for _, item := range plans {
		plan, ok := item.(map[string]any)
		date := asString(plan["date"])
		_, err := time.Parse("2006-01-02", date)
		ref := strings.TrimSpace(asString(plan["verse_ref"]))
		text := strings.TrimSpace(asString(plan["recite_text"]))
		if !ok || err != nil || dates[date] || ref == "" || text == "" ||
			utf8.RuneCountInString(ref) > 255 || utf8.RuneCountInString(text) > 10000 {
			return ErrInvalidDailyVerse
		}
		dates[date] = true
	}
	return nil
}
