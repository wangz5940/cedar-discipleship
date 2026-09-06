package learning

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const scheduleHistoryKey = "schedule_history"

func preserveDailyScheduleHistory(existing, next map[string]any) error {
	existingDaily, existingOK := nestedMap(existing, "task_sections", "daily")
	nextDaily, nextOK := nestedMap(next, "task_sections", "daily")
	if !existingOK || !nextOK {
		return nil
	}
	configs := []struct {
		name     string
		dateKeys []string
	}{
		{name: "devotion", dateKeys: []string{"numbered_start_date", "start_date"}},
		{name: "scripture", dateKeys: []string{"start_date"}},
	}
	for _, item := range configs {
		existingConfig, existingOK := nestedMap(existingDaily, item.name)
		nextConfig, nextOK := nestedMap(nextDaily, item.name)
		if !existingOK || !nextOK {
			continue
		}
		history, err := clonedScheduleHistory(existingConfig)
		if err != nil {
			return err
		}
		existingStart := scheduleStartDate(existingConfig, item.dateKeys)
		nextStart := scheduleStartDate(nextConfig, item.dateKeys)
		if scheduleMovedForward(existingStart, nextStart) {
			previous, err := cloneScheduleConfig(existingConfig)
			if err != nil {
				return err
			}
			history = upsertScheduleVersion(history, previous, item.dateKeys)
		}
		if len(history) == 0 {
			delete(nextConfig, scheduleHistoryKey)
			continue
		}
		sort.SliceStable(history, func(i, j int) bool {
			return scheduleStartDate(history[i], item.dateKeys) < scheduleStartDate(history[j], item.dateKeys)
		})
		items := make([]any, 0, len(history))
		for _, version := range history {
			items = append(items, version)
		}
		nextConfig[scheduleHistoryKey] = items
	}
	return nil
}

func clonedScheduleHistory(config map[string]any) ([]map[string]any, error) {
	raw, ok := config[scheduleHistoryKey]
	if !ok {
		return nil, nil
	}
	var source []any
	switch items := raw.(type) {
	case []any:
		source = items
	case []map[string]any:
		source = make([]any, 0, len(items))
		for _, item := range items {
			source = append(source, item)
		}
	default:
		return nil, nil
	}
	history := make([]map[string]any, 0, len(source))
	for _, item := range source {
		version, ok := item.(map[string]any)
		if !ok {
			continue
		}
		cloned, err := cloneScheduleConfig(version)
		if err != nil {
			return nil, err
		}
		history = append(history, cloned)
	}
	return history, nil
}

func cloneScheduleConfig(config map[string]any) (map[string]any, error) {
	payload, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return nil, err
	}
	delete(cloned, scheduleHistoryKey)
	return cloned, nil
}

func upsertScheduleVersion(history []map[string]any, version map[string]any, dateKeys []string) []map[string]any {
	start := scheduleStartDate(version, dateKeys)
	for index := range history {
		if scheduleStartDate(history[index], dateKeys) == start {
			history[index] = version
			return history
		}
	}
	return append(history, version)
}

func scheduleStartDate(config map[string]any, keys []string) string {
	for _, key := range keys {
		value := strings.TrimSpace(asString(config[key]))
		if _, err := time.Parse("2006-01-02", value); err == nil {
			return value
		}
	}
	return ""
}

func scheduleMovedForward(existingStart, nextStart string) bool {
	return existingStart != "" && nextStart != "" && nextStart > existingStart
}
