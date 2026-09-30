package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	statisticsdomain "agp/backend/internal/statistics"
)

func (a *app) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	from := queryDate(r, "from", time.Now().In(a.location).AddDate(0, 0, -7))
	to := queryDate(r, "to", time.Now().In(a.location))
	summary, err := a.statistics.Summary(r.Context(), groupID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_failed")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (a *app) handleDashboardTaskCompletions(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	date := queryDate(r, "date", time.Now().In(a.location))
	now := time.Now().In(a.location)
	content, cacheStatus, err := a.todayContent(r.Context(), groupID, date, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "task_completions_failed")
		return
	}
	completions, err := a.learning.GroupTaskCompletionsFromContent(r.Context(), groupID, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "task_completions_failed")
		return
	}
	w.Header().Set("X-AGP-Today-Cache", cacheStatus)
	writeJSON(w, http.StatusOK, completions)
}

func (a *app) handleDashboardMonthlyRanking(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	ranking, err := a.statistics.MonthlyRanking(
		r.Context(),
		groupID,
		r.URL.Query().Get("month"),
		r.URL.Query().Get("from"),
		r.URL.Query().Get("to"),
		a.location,
	)
	if errors.Is(err, statisticsdomain.ErrInvalidMonth) {
		writeError(w, http.StatusBadRequest, "invalid_month")
		return
	}
	if errors.Is(err, statisticsdomain.ErrInvalidDateRange) {
		writeError(w, http.StatusBadRequest, "invalid_date_range")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "monthly_ranking_failed")
		return
	}
	settings, err := a.groupLearningConfig(r.Context(), groupID)
	if err != nil {
		slog.ErrorContext(
			r.Context(),
			"dashboard active rule load failed",
			"method", r.Method,
			"path", r.URL.Path,
			"user_id", u.ID,
			"group_id", groupID,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "monthly_ranking_failed")
		return
	}
	ranking.ActiveRule = activeMemberRuleFromSettings(settings)
	ranking.CanManageActiveRule = canManageActiveMemberRule(u)
	writeJSON(w, http.StatusOK, ranking)
}

func (a *app) handleDashboardActiveRule(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input statisticsdomain.ActiveMemberRuleVO
	if !readJSON(w, r, &input) {
		return
	}
	rule, valid := normalizeActiveMemberRule(input)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid_active_member_rule")
		return
	}
	var beforeRule *statisticsdomain.ActiveMemberRuleVO
	if currentSettings, err := a.groupLearningConfig(r.Context(), groupID); err == nil {
		current := activeMemberRuleFromSettings(currentSettings)
		beforeRule = &current
	} else {
		slog.WarnContext(
			r.Context(),
			"dashboard active rule audit snapshot failed",
			"method", r.Method,
			"path", r.URL.Path,
			"user_id", u.ID,
			"group_id", groupID,
			"error", err,
		)
	}
	settings := map[string]any{
		"mode":       rule.Mode,
		"task_types": rule.TaskTypes,
	}
	if err := a.learning.SaveActiveMemberRule(r.Context(), groupID, settings); err != nil {
		slog.ErrorContext(
			r.Context(),
			"dashboard active rule save failed",
			"method", r.Method,
			"path", r.URL.Path,
			"user_id", u.ID,
			"group_id", groupID,
			"error", err,
		)
		writeError(w, http.StatusInternalServerError, "active_member_rule_failed")
		return
	}
	a.refreshTodayContent(groupID)
	if beforeRule == nil {
		a.audit(groupID, u.ID, "update_active_member_rule", "group_settings", groupID, nil, rule, r)
	} else {
		a.auditChanges(groupID, u.ID, "update_active_member_rule", "group_settings", groupID, *beforeRule, rule, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active_rule": rule})
}

var activeMemberTaskTypes = []string{
	"daily_devotion",
	"daily_scripture",
	"weekly_book",
	"weekly_video",
}

func defaultActiveMemberRule() statisticsdomain.ActiveMemberRuleVO {
	return statisticsdomain.ActiveMemberRuleVO{
		Mode:      "any",
		TaskTypes: append([]string(nil), activeMemberTaskTypes...),
	}
}

func activeMemberRuleFromSettings(settings map[string]any) statisticsdomain.ActiveMemberRuleVO {
	raw, ok := settings["active_member_rule"].(map[string]any)
	if !ok {
		return defaultActiveMemberRule()
	}
	mode, _ := raw["mode"].(string)
	input := statisticsdomain.ActiveMemberRuleVO{Mode: mode}
	switch taskTypes := raw["task_types"].(type) {
	case []string:
		input.TaskTypes = taskTypes
	case []any:
		for _, value := range taskTypes {
			if taskType, ok := value.(string); ok {
				input.TaskTypes = append(input.TaskTypes, taskType)
			}
		}
	}
	rule, valid := normalizeActiveMemberRule(input)
	if !valid {
		return defaultActiveMemberRule()
	}
	return rule
}

func normalizeActiveMemberRule(input statisticsdomain.ActiveMemberRuleVO) (statisticsdomain.ActiveMemberRuleVO, bool) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode != "any" && mode != "all" {
		return statisticsdomain.ActiveMemberRuleVO{}, false
	}
	requested := make(map[string]bool, len(input.TaskTypes))
	for _, taskType := range input.TaskTypes {
		taskType = strings.TrimSpace(taskType)
		if !validActiveMemberTaskType(taskType) {
			if retiredActiveMemberTaskType(taskType) {
				continue
			}
			return statisticsdomain.ActiveMemberRuleVO{}, false
		}
		requested[taskType] = true
	}
	taskTypes := make([]string, 0, len(requested))
	for _, taskType := range activeMemberTaskTypes {
		if requested[taskType] {
			taskTypes = append(taskTypes, taskType)
		}
	}
	if len(taskTypes) == 0 {
		return statisticsdomain.ActiveMemberRuleVO{}, false
	}
	return statisticsdomain.ActiveMemberRuleVO{Mode: mode, TaskTypes: taskTypes}, true
}

func validActiveMemberTaskType(value string) bool {
	for _, taskType := range activeMemberTaskTypes {
		if value == taskType {
			return true
		}
	}
	return false
}

func retiredActiveMemberTaskType(value string) bool {
	return value == "weekly_checkin" || value == "weekly_outline"
}

func canManageActiveMemberRule(user currentUser) bool {
	return user.IsSuperAdmin ||
		hasRole(user.Roles, roleGroupAdmin) ||
		hasRole(user.Roles, roleGroupLeader)
}

func (a *app) handleMembers(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	members, err := a.listMembers(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "members_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (a *app) handleMemberCalendar(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	memberID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	calendar, err := a.statistics.MemberCalendar(r.Context(), groupID, memberID, r.URL.Query().Get("month"), a.location)
	if errors.Is(err, statisticsdomain.ErrInvalidMonth) {
		writeError(w, http.StatusBadRequest, "invalid_month")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "calendar_failed")
		return
	}
	writeJSON(w, http.StatusOK, calendar)
}
