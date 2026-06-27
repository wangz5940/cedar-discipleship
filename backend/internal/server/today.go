package server

import (
	"net/http"
	"time"
)

func (a *app) handleToday(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	date := queryDate(r, "date", time.Now().In(a.location))
	settings, err := a.groupLearningConfig(groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "today_failed")
		return
	}
	hub, err := a.learning.TodayHub(r.Context(), groupID, u.ID, date, settings, time.Now().In(a.location))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "today_failed")
		return
	}
	writeJSON(w, http.StatusOK, hub)
}
