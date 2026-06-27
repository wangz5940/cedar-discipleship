package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	learningdomain "agp/backend/internal/learning"
)

type weekTaskBinding = learningdomain.TaskBinding
type studyWeekInput = learningdomain.WeekInput

func (a *app) handleStudyWeeks(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	weeks, err := a.learning.ListWeeks(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "weeks_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"weeks": weeks})
}

func (a *app) handleCurrentStudyWeek(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	week, err := a.currentWeek(groupID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "week_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"week": week})
}

func (a *app) handleAdminCreateStudyWeek(w http.ResponseWriter, r *http.Request) {
	a.saveStudyWeek(w, r, 0)
}

func (a *app) handleAdminUpdateStudyWeek(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	a.saveStudyWeek(w, r, id)
}

func (a *app) handleAdminDeleteStudyWeek(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	weekID, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if weekID == 0 {
		writeError(w, http.StatusBadRequest, "week_id_required")
		return
	}
	if err := a.learning.DeleteWeek(r.Context(), groupID, weekID); err != nil {
		writeError(w, http.StatusInternalServerError, "week_delete_failed")
		return
	}
	a.audit(groupID, u.ID, "delete_study_week", "study_weeks", weekID, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) saveStudyWeek(w http.ResponseWriter, r *http.Request, id uint64) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req studyWeekInput
	if !readJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.StartDate) == "" || strings.TrimSpace(req.EndDate) == "" {
		writeError(w, http.StatusBadRequest, "week_dates_required")
		return
	}
	savedID, err := a.learning.SaveWeek(r.Context(), groupID, id, req, time.Now().In(a.location))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "week_task_save_failed")
		return
	}
	id = savedID
	a.audit(groupID, u.ID, "save_study_week", "study_weeks", id, nil, map[string]any{"title": req.Title}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (a *app) currentWeek(groupID uint64) (map[string]any, error) {
	today := time.Now().In(a.location).Format("2006-01-02")
	return a.currentWeekAt(groupID, today)
}

func splitWeekTaskBindings(tasks []map[string]any) ([]weekTaskBinding, []weekTaskBinding, weekTaskBinding) {
	return learningdomain.SplitWeekTaskBindings(tasks)
}

func inferTaskBindingType(taskType, urlValue, fileName string) string {
	return learningdomain.InferTaskBindingType(taskType, urlValue, fileName)
}

func (a *app) currentWeekAt(groupID uint64, date string) (map[string]any, error) {
	return a.learning.CurrentWeek(context.Background(), groupID, date)
}

func (a *app) weekTasks(groupID, weekID uint64) ([]map[string]any, error) {
	return a.learning.WeekTasks(context.Background(), groupID, weekID)
}

func weeklyVerseTaskTitle(req studyWeekInput, existingTitle string) string {
	return learningdomain.WeeklyVerseTaskTitle(req, existingTitle)
}
