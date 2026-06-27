package server

import (
	"net/http"
	"strconv"
	"time"

	checkindomain "agp/backend/internal/checkin"
)

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
	if req.LogicalDate == "" {
		req.LogicalDate = time.Now().In(a.location).Format("2006-01-02")
	}
	today := time.Now().In(a.location).Format("2006-01-02")
	if req.LogicalDate > today {
		writeError(w, http.StatusBadRequest, "future_checkin_not_allowed")
		return
	}
	id, existing, err := a.checkins.Create(r.Context(), &checkindomain.Record{
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
	}, u.ID)
	if err != nil {
		writeError(w, http.StatusConflict, "checkin_save_failed")
		return
	}
	if existing {
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
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
