package learning

func SeparateDailyCheckins(settings map[string]any) bool {
	return nestedString(settings, []string{"task_sections", "daily", "checkin_mode"}, "combined") == "separate"
}

func DailyTaskTypeEnabled(settings map[string]any, taskType string) bool {
	if !SeparateDailyCheckins(settings) {
		return taskType == "daily_devotion" && DailyTaskEnabled(settings)
	}
	return dailyComponentEnabled(settings, taskType)
}

func DailyTaskTypeEnabledOnDate(settings map[string]any, taskType, date string) bool {
	if !SeparateDailyCheckins(settings) {
		if taskType != "daily_devotion" {
			return false
		}
		return dailyDevotionEnabledOnDate(settings, date) || dailyComponentEnabled(settings, "daily_scripture")
	}
	if taskType == "daily_devotion" {
		return dailyDevotionEnabledOnDate(settings, date)
	}
	return dailyComponentEnabled(settings, taskType)
}

func dailyComponentEnabled(settings map[string]any, taskType string) bool {
	component := ""
	switch taskType {
	case "daily_devotion":
		component = "devotion"
	case "daily_scripture":
		component = "scripture"
	default:
		return false
	}
	config, exists := nestedMap(settings, "task_sections", "daily", component)
	return !exists || mapBool(config, "enabled", true)
}

func dailyDevotionEnabledOnDate(settings map[string]any, date string) bool {
	config, exists := nestedMap(settings, "task_sections", "daily", "devotion")
	if exists && !mapBool(config, "enabled", true) {
		return false
	}
	if !exists || asString(config["plan_mode"]) != "custom" || date == "" {
		return true
	}
	_, found := customDailyDevotionPlan(config, date)
	return found
}

func customDailyDevotionPlan(config map[string]any, date string) (map[string]any, bool) {
	var source []any
	switch plans := config["plans"].(type) {
	case []any:
		source = plans
	case []map[string]any:
		source = make([]any, 0, len(plans))
		for _, plan := range plans {
			source = append(source, plan)
		}
	}
	var matched map[string]any
	for _, item := range source {
		plan, ok := item.(map[string]any)
		if ok && asString(plan["date"]) == date {
			matched = plan
		}
	}
	return matched, matched != nil
}

func dailyTasks(date string, settings map[string]any) []TodayTaskVO {
	var tasks []TodayTaskVO
	for _, taskType := range []string{"daily_devotion", "daily_scripture"} {
		if !DailyTaskTypeEnabledOnDate(settings, taskType, date) {
			continue
		}
		title := nestedString(settings, []string{"task_sections", "daily", "label"}, "每日灵修")
		summary := "今天的灵修与读经"
		if SeparateDailyCheckins(settings) {
			title = nestedString(settings, []string{"task_sections", "daily", "devotion", "title"}, "每日灵修")
			if taskType == "daily_scripture" {
				title = nestedString(settings, []string{"task_sections", "daily", "scripture", "label"}, "每日读经")
			}
			summary = title
		}
		if taskType == "daily_devotion" {
			if config, ok := nestedMap(settings, "task_sections", "daily", "devotion"); ok {
				if plan, found := customDailyDevotionPlan(config, date); found {
					title = firstNonEmpty(asString(plan["title"]), title)
					summary = title
				}
			}
		}
		tasks = append(tasks, TodayTaskVO{
			ID: taskType, Type: taskType, Kind: todayTaskKind(taskType),
			Title: title, Detail: title, Summary: summary,
			Required: true, Status: "pending",
		})
	}
	return tasks
}

func hasAggregateWeeklyTask(tasks []map[string]any) bool {
	for _, task := range tasks {
		if asString(task["task_type"]) == "weekly_checkin" && mapBool(task, "enabled", true) {
			return true
		}
	}
	return false
}
