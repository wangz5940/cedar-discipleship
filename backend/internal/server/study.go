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
	week, err := a.currentWeek(r.Context(), groupID)
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
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_week_dates")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil || endDate.Before(startDate) {
		writeError(w, http.StatusBadRequest, "invalid_week_dates")
		return
	}
	savedID, err := a.learning.SaveWeek(r.Context(), groupID, id, req, time.Now().In(a.location))
	if errors.Is(err, learningdomain.ErrWeekNotFound) {
		writeError(w, http.StatusNotFound, "week_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "week_task_save_failed")
		return
	}
	id = savedID
	a.audit(groupID, u.ID, "save_study_week", "study_weeks", id, nil, map[string]any{"title": req.Title}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (a *app) currentWeek(ctx context.Context, groupID uint64) (map[string]any, error) {
	today := time.Now().In(a.location).Format("2006-01-02")
	return a.currentWeekAt(ctx, groupID, today)
}

func (a *app) currentWeekAt(ctx context.Context, groupID uint64, date string) (map[string]any, error) {
	return a.learning.CurrentWeek(ctx, groupID, date)
}

func (a *app) weekTasks(ctx context.Context, groupID, weekID uint64) ([]map[string]any, error) {
	return a.learning.WeekTasks(ctx, groupID, weekID)
}

func weeklyVerseTaskTitle(req studyWeekInput, existingTitle string) string {
	return learningdomain.WeeklyVerseTaskTitle(req, existingTitle)
}
