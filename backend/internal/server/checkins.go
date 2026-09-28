package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	checkindomain "agp/backend/internal/checkin"
	learningdomain "agp/backend/internal/learning"
	notificationdomain "agp/backend/internal/notification"
)

var (
	errDailyTaskDisabled = errors.New("daily_task_disabled")
	errCheckinConfig     = errors.New("checkin_config_lookup_failed")
)

func (a *app) createAdmittedCheckin(
	ctx context.Context,
	record *checkindomain.Record,
	actorID uint64,
) (uint64, bool, error) {
	if record.TaskType == "daily_devotion" || record.TaskType == "daily_scripture" {
		settings, err := a.groupLearningConfig(ctx, record.GroupID)
		if err != nil {
			return 0, false, fmt.Errorf("%w: %w", errCheckinConfig, err)
		}
		if !learningdomain.DailyTaskTypeEnabledOnDate(settings, record.TaskType, record.LogicalDate) {
			return 0, false, errDailyTaskDisabled
		}
		record.Part = ""
		record.TaskID = 0
		record.WeekID = 0
	}
	return a.checkins.Create(ctx, record, actorID)
}

func (a *app) handleCreateCheckin(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		TaskType    string `json:"task_type"`
		LogicalDate string `json:"logical_date"`
		Part        string `json:"part"`
		Detail      string `json:"detail"`
		Note        string `json:"note"`
		WeekID      uint64 `json:"week_id"`
		TaskID      uint64 `json:"task_id"`
		IsRetro     bool   `json:"is_retro"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.TaskType == "" {
		writeError(w, http.StatusBadRequest, "task_type_required")
		return
	}
	if !validCheckinTaskType(req.TaskType) {
		writeError(w, http.StatusBadRequest, "invalid_task_type")
		return
	}
	if req.LogicalDate == "" {
		req.LogicalDate = time.Now().In(a.location).Format("2006-01-02")
	}
	logicalDate, err := time.ParseInLocation("2006-01-02", req.LogicalDate, a.location)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_logical_date")
		return
	}
	today := time.Now().In(a.location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, a.location)
	if logicalDate.After(today) {
		writeError(w, http.StatusBadRequest, "future_checkin_not_allowed")
		return
	}
	record := &checkindomain.Record{
		GroupID:     groupID,
		UserID:      u.ID,
		TaskID:      req.TaskID,
		WeekID:      req.WeekID,
		LogicalDate: req.LogicalDate,
		TaskType:    req.TaskType,
		Part:        req.Part,
		Detail:      req.Detail,
		Note:        req.Note,
		IsRetro:     req.IsRetro,
	}
	id, existing, err := a.createAdmittedCheckin(r.Context(), record, u.ID)
	if errors.Is(err, errDailyTaskDisabled) {
		writeError(w, http.StatusBadRequest, "daily_task_disabled")
		return
	}
	if errors.Is(err, errCheckinConfig) {
		slog.ErrorContext(r.Context(), "checkin learning config lookup failed", "group_id", groupID, "error", err)
		writeError(w, http.StatusInternalServerError, "checkin_save_failed")
		return
	}
	if errors.Is(err, checkindomain.ErrInvalidWeeklyTarget) {
		writeError(w, http.StatusBadRequest, "invalid_checkin_target")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "checkin save failed", "group_id", groupID, "user_id", u.ID,
			"task_type", req.TaskType, "logical_date", req.LogicalDate, "task_id", req.TaskID, "week_id", req.WeekID, "error", err)
		writeError(w, http.StatusConflict, "checkin_save_failed")
		return
	}
	if existing {
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
		return
	}
	if a.notifications != nil {
		err := a.notifications.Enqueue(notificationdomain.Event{
			RecordID: id, GroupID: groupID, LogicalDate: req.LogicalDate, OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			slog.ErrorContext(r.Context(), "checkin notification enqueue failed",
				"record_id", id, "group_id", groupID, "error", err)
		}
	}
	a.audit(groupID, u.ID, "create_checkin", "checkin_records", id, nil, map[string]any{
		"logical_date": req.LogicalDate,
		"task_type":    req.TaskType,
		"task_id":      record.TaskID,
		"week_id":      record.WeekID,
		"part":         record.Part,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func validCheckinTaskType(taskType string) bool {
	switch taskType {
	case "daily_devotion", "daily_scripture", "weekly_book", "weekly_video", "weekly_verse", "weekly_outline":
		return true
	default:
		return false
	}
}

func (a *app) handleDeleteOwnCheckin(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err := a.checkins.DeleteOwn(r.Context(), groupID, u.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed")
		return
	}
	a.audit(groupID, u.ID, "delete_own_checkin", "checkin_records", id, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleAdminDeleteCheckin(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err := a.checkins.DeleteAny(r.Context(), groupID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed")
		return
	}
	a.audit(groupID, u.ID, "delete_checkin", "checkin_records", id, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleListCheckins(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	from := queryDate(r, "from", time.Now().In(a.location).AddDate(0, 0, -30))
	to := queryDate(r, "to", time.Now().In(a.location))
	userID, _ := strconv.ParseUint(r.URL.Query().Get("user_id"), 10, 64)
	limit := clampInt(queryInt(r, "page_size", 50), 1, 1000)
	records, err := a.checkins.List(r.Context(), groupID, from, to, userID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "checkins_failed")
		return
	}
	var items []map[string]any
	for _, record := range records {
		items = append(items, map[string]any{
			"id":           record.ID,
			"user_id":      record.UserID,
			"task_id":      nullableUint64Value(record.TaskID),
			"week_id":      nullableUint64Value(record.WeekID),
			"logical_date": record.LogicalDate,
			"checkin_time": record.CheckinTime,
			"task_type":    record.TaskType,
			"part":         record.Part,
			"detail":       record.Detail,
			"note":         record.Note,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
