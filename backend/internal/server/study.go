package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	learningdomain "agp/backend/internal/learning"
)

type weekTaskBinding = learningdomain.TaskBinding
type studyWeekInput = learningdomain.WeekInput

type studyWeekSaveRequest struct {
	studyWeekInput
	Force bool `json:"force"`
}

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
	before := a.studyWeekAuditSnapshot(r.Context(), groupID, weekID)
	if err := a.learning.DeleteWeek(r.Context(), groupID, weekID); err != nil {
		writeError(w, http.StatusInternalServerError, "week_delete_failed")
		return
	}
	a.refreshTodayContent(groupID)
	a.audit(groupID, u.ID, "delete_study_week", "study_weeks", weekID, before, map[string]any{"deleted": true}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) saveStudyWeek(w http.ResponseWriter, r *http.Request, id uint64) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req studyWeekSaveRequest
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
	before := a.studyWeekAuditSnapshot(r.Context(), groupID, id)
	savedID, err := a.learning.SaveWeek(
		r.Context(),
		groupID,
		id,
		req.studyWeekInput,
		req.Force,
		time.Now().In(a.location),
	)
	if errors.Is(err, learningdomain.ErrInvalidVerseMode) {
		writeError(w, http.StatusBadRequest, "invalid_verse_mode")
		return
	}
	if errors.Is(err, learningdomain.ErrWeekNotFound) {
		writeError(w, http.StatusNotFound, "week_not_found")
		return
	}
	if errors.Is(err, learningdomain.ErrWeekHasCheckins) {
		writeError(w, http.StatusConflict, "week_has_checkins")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "study week save failed",
			"group_id", groupID, "week_id", id, "error", err)
		writeError(w, http.StatusInternalServerError, "week_task_save_failed")
		return
	}
	id = savedID
	a.refreshTodayContent(groupID)
	after := a.studyWeekAuditSnapshot(r.Context(), groupID, id)
	if after == nil {
		after = studyWeekAuditValue(req.studyWeekInput)
	}
	if before == nil {
		a.audit(groupID, u.ID, "save_study_week", "study_weeks", id, nil, after, r)
	} else {
		a.auditChanges(groupID, u.ID, "save_study_week", "study_weeks", id, before, after, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func (a *app) studyWeekAuditSnapshot(ctx context.Context, groupID, weekID uint64) map[string]any {
	if weekID == 0 {
		return nil
	}
	weeks, err := a.learning.ListWeekInputs(ctx, groupID)
	if err != nil {
		slog.WarnContext(ctx, "study week audit snapshot failed",
			"group_id", groupID, "week_id", weekID, "error", err)
		return nil
	}
	for _, week := range weeks {
		if week.ID == weekID {
			return studyWeekAuditValue(week)
		}
	}
	return nil
}

func studyWeekAuditValue(week learningdomain.WeekInput) map[string]any {
	return map[string]any{
		"start_date":         week.StartDate,
		"end_date":           week.EndDate,
		"title":              week.Title,
		"verse_ref":          week.VerseRef,
		"recite_text_length": len([]rune(week.ReciteText)),
		"recite_text_sha256": textAuditDigest(week.ReciteText),
		"book_enabled":       week.BookEnabled,
		"weekly_checkin":     week.WeeklyCheckin,
		"video_enabled":      week.VideoEnabled,
		"verse_enabled":      week.VerseEnabled,
		"verse_mode":         week.VerseMode,
		"outline_enabled":    week.OutlineEnabled,
		"readings":           studyWeekBindingsAuditValue(week.Readings),
		"videos":             studyWeekBindingsAuditValue(week.Videos),
		"outline":            studyWeekBindingAuditValue(week.Outline),
	}
}

func studyWeekBindingsAuditValue(bindings []learningdomain.TaskBinding) []map[string]any {
	items := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, studyWeekBindingAuditValue(binding))
	}
	return items
}

func studyWeekBindingAuditValue(binding learningdomain.TaskBinding) map[string]any {
	return map[string]any{
		"title":      binding.Title,
		"url":        binding.URL,
		"type":       binding.Type,
		"asset_id":   binding.AssetID,
		"page_start": binding.PageStart,
		"page_end":   binding.PageEnd,
	}
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
