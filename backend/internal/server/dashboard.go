package server

import (
	"errors"
	"net/http"
	"strconv"
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
	ranking, err := a.statistics.MonthlyRanking(r.Context(), groupID, r.URL.Query().Get("month"), a.location)
	if errors.Is(err, statisticsdomain.ErrInvalidMonth) {
		writeError(w, http.StatusBadRequest, "invalid_month")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "monthly_ranking_failed")
		return
	}
	writeJSON(w, http.StatusOK, ranking)
}

func (a *app) handleMembers(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	members, err := a.listMembers(groupID)
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
