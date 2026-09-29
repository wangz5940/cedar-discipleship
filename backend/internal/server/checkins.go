package server

import (
	"context"
	"database/sql"
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
	if u.IsTenantAdmin && !u.IsSuperAdmin {
		var member bool
		if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=? AND user_id=? AND status=1)`, groupID, u.ID).Scan(&member); err != nil || !member {
			writeError(w, http.StatusForbidden, "group_membership_required")
			return
		}
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
		markAuditHandled(r)
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
	before := a.checkinAuditSnapshot(r.Context(), groupID, id)
	if err := a.checkins.DeleteOwn(r.Context(), groupID, u.ID, id); err != nil {
		writeCheckinDeleteError(w, err)
		return
	}
	a.audit(groupID, u.ID, "delete_own_checkin", "checkin_records", id, before, map[string]any{"deleted": true}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleAdminDeleteCheckin(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	before := a.checkinAuditSnapshot(r.Context(), groupID, id)
	if err := a.checkins.DeleteAny(r.Context(), groupID, id); err != nil {
		writeCheckinDeleteError(w, err)
		return
	}
	a.audit(groupID, u.ID, "delete_checkin", "checkin_records", id, before, map[string]any{"deleted": true}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) checkinAuditSnapshot(ctx context.Context, groupID, recordID uint64) map[string]any {
	if a.db == nil || recordID == 0 {
		return nil
	}
	var (
		userID      uint64
		taskID      sql.NullInt64
		weekID      sql.NullInt64
		logicalDate time.Time
		taskType    string
		part        string
	)
	err := a.db.QueryRowContext(ctx, `SELECT user_id,task_id,week_id,logical_date,task_type,part
		FROM checkin_records WHERE id=? AND group_id=? AND deleted_at IS NULL`,
		recordID, groupID,
	).Scan(&userID, &taskID, &weekID, &logicalDate, &taskType, &part)
	if err != nil {
		return nil
	}
	return map[string]any{
		"user_id":      userID,
		"task_id":      nullableAuditID(taskID),
		"week_id":      nullableAuditID(weekID),
		"logical_date": logicalDate.Format("2006-01-02"),
		"task_type":    taskType,
		"part":         part,
		"deleted":      false,
	}
}

func nullableAuditID(value sql.NullInt64) any {
	if !value.Valid || value.Int64 <= 0 {
		return nil
	}
	return uint64(value.Int64)
}

func writeCheckinDeleteError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "checkin_not_found")
		return
	}
	writeError(w, http.StatusInternalServerError, "delete_failed")
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
