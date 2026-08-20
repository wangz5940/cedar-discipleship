package server

import (
	"errors"
	"log"
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
		log.Printf(
			"dashboard active rule load failed method=%s path=%s user_id=%d group_id=%d err=%v",
			r.Method,
			r.URL.Path,
			u.ID,
			groupID,
			err,
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
	settings, err := a.groupLearningConfig(r.Context(), groupID)
	if err != nil {
		log.Printf(
			"dashboard active rule load failed method=%s path=%s user_id=%d group_id=%d err=%v",
			r.Method,
			r.URL.Path,
			u.ID,
			groupID,
			err,
		)
		writeError(w, http.StatusInternalServerError, "active_member_rule_failed")
		return
	}
	settings["active_member_rule"] = map[string]any{
		"mode":       rule.Mode,
		"task_types": rule.TaskTypes,
	}
	if err := a.upsertGroupLearningConfig(r.Context(), groupID, settings); err != nil {
		log.Printf(
			"dashboard active rule save failed method=%s path=%s user_id=%d group_id=%d err=%v",
			r.Method,
			r.URL.Path,
			u.ID,
			groupID,
			err,
		)
		writeError(w, http.StatusInternalServerError, "active_member_rule_failed")
		return
	}
	a.audit(
		groupID,
		u.ID,
		"update_active_member_rule",
		"group_settings",
		groupID,
		nil,
		rule,
		r,
	)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active_rule": rule})
}

var activeMemberTaskTypes = []string{
	"daily_devotion",
	"weekly_book",
	"weekly_video",
	"weekly_outline",
}

func defaultActiveMemberRule() statisticsdomain.ActiveMemberRuleVO {
	return statisticsdomain.ActiveMemberRuleVO{
		Mode:      "any",
		TaskTypes: []string{"weekly_outline"},
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
