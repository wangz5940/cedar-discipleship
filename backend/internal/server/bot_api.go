package server

import (
	"crypto/subtle"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	checkindomain "agp/backend/internal/checkin"
	notificationdomain "agp/backend/internal/notification"
	userdomain "agp/backend/internal/user"
)

const botHistoryStart = "2026-04-06"

func (a *app) botAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(a.botAPIKey) == 0 || len(provided) != len(a.botAPIKey) || subtle.ConstantTimeCompare([]byte(provided), a.botAPIKey) != 1 {
			writeError(w, http.StatusUnauthorized, "bot_unauthorized")
			return
		}
		next(w, r)
	}
}

func (a *app) botGroup(r *http.Request) (userdomain.Group, error) {
	code := strings.TrimSpace(r.PathValue("code"))
	groups, err := a.users.AllGroups(r.Context())
	if err != nil {
		return userdomain.Group{}, err
	}
	for _, item := range groups {
		if item.Code == code {
			return item, nil
		}
	}
	return userdomain.Group{}, sql.ErrNoRows
}

func (a *app) handleBotGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.users.AllGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_groups_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (a *app) botGroupData(r *http.Request) (userdomain.Group, []userdomain.MemberVO, []map[string]any, error) {
	group, err := a.botGroup(r)
	if err != nil {
		return group, nil, nil, err
	}
	members, err := a.users.Members(r.Context(), group.ID)
	if err != nil {
		return group, nil, nil, err
	}
	weeks, err := a.learning.ListWeeks(r.Context(), group.ID)
	if err != nil {
		return group, nil, nil, err
	}
	schedule := make([]map[string]any, 0, len(weeks))
	for _, week := range weeks {
		schedule = append(schedule, map[string]any{
			"id": week.ID, "start": week.Start, "end": week.End, "title": week.Title,
			"verse": week.VerseRef, "verse_ref": week.VerseRef, "reciteText": week.ReciteText,
			"recite_text": week.ReciteText, "readings": week.Readings, "videos": week.Videos,
			"book_enabled": week.BookEnabled, "video_enabled": week.VideoEnabled,
			"verse_enabled": week.VerseEnabled, "outline_enabled": week.OutlineEnabled,
		})
	}
	return group, members, schedule, nil
}

func botMemberNames(members []userdomain.MemberVO) []string {
	out := make([]string, 0, len(members))
	for _, member := range members {
		name := strings.TrimSpace(member.MemberName)
		if name == "" {
			name = strings.TrimSpace(member.DisplayName)
		}
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (a *app) handleBotConfig(w http.ResponseWriter, r *http.Request) {
	group, members, schedule, err := a.botGroupData(r)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bot_group_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_config_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"site_info": map[string]any{"title": group.Name, "group_code": group.Code},
		"members":   botMemberNames(members), "weekly_schedule": schedule,
	})
}

func (a *app) handleBotState(w http.ResponseWriter, r *http.Request) {
	_, members, schedule, err := a.botGroupData(r)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bot_group_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_state_failed")
		return
	}
	group, _ := a.botGroup(r)
	to := time.Now().In(a.location).Format("2006-01-02")
	records, err := a.checkins.List(r.Context(), group.ID, botHistoryStart, to, 0, 100000)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_state_failed")
		return
	}
	names := map[uint64]string{}
	for _, member := range members {
		name := strings.TrimSpace(member.MemberName)
		if name == "" {
			name = strings.TrimSpace(member.DisplayName)
		}
		names[member.UserID] = name
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		item := map[string]any{"id": record.ID, "name": names[record.UserID], "logical_date": record.LogicalDate,
			"checkin_time": record.CheckinTime, "is_retro": record.IsRetro, "daily": "", "book": "", "video": "", "verse": ""}
		switch record.TaskType {
		case "daily_devotion":
			item["daily"] = "done"
		case "weekly_book":
			item["book"] = "done"
		case "weekly_video":
			item["video"] = "done"
		case "weekly_verse":
			item["verse"] = "done"
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": botMemberNames(members), "weeklySchedule": schedule, "records": items})
}

// handleBotEvents returns an incremental stream of check-in creates and cancellations.
// The cursor is opaque to clients and orders equal-millisecond updates by record ID.
func (a *app) handleBotEvents(w http.ResponseWriter, r *http.Request) {
	group, err := a.botGroup(r)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bot_group_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_group_failed")
		return
	}
	now := time.Now().UTC()
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor == "" {
		writeJSON(w, http.StatusOK, map[string]any{"cursor": botEventCursor(now, 0), "events": []any{}})
		return
	}
	updatedAt, afterID, err := parseBotEventCursor(cursor)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_event_cursor")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT c.id, COALESCE(NULLIF(m.member_name,''),u.display_name), c.logical_date,
		       c.checkin_time, c.task_type, COALESCE(c.detail,''), COALESCE(c.part,''),
		       COALESCE(st.title,''), c.is_retro, c.updated_at, c.deleted_at
		FROM checkin_records c
		JOIN users u ON u.id=c.user_id
		LEFT JOIN group_members m ON m.group_id=c.group_id AND m.user_id=c.user_id
		LEFT JOIN study_tasks st ON st.id=c.task_id AND st.group_id=c.group_id
		WHERE c.group_id=? AND (c.updated_at>? OR (c.updated_at=? AND c.id>?))
		ORDER BY c.updated_at,c.id LIMIT 500`, group.ID, updatedAt, updatedAt, afterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_events_failed")
		return
	}
	defer rows.Close()
	events := make([]map[string]any, 0)
	lastTime, lastID := updatedAt, afterID
	for rows.Next() {
		var id uint64
		var name, taskType, detail, part, taskTitle string
		var logicalDate, checkinTime, changedAt time.Time
		var isRetro bool
		var deletedAt sql.NullTime
		if err := rows.Scan(&id, &name, &logicalDate, &checkinTime, &taskType, &detail, &part, &taskTitle, &isRetro, &changedAt, &deletedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "bot_events_failed")
			return
		}
		label := botTaskLabel(taskType, taskTitle, detail, part)
		action := "checkin"
		if deletedAt.Valid {
			action = "cancel"
		}
		events = append(events, map[string]any{
			"id": id, "action": action, "name": strings.TrimSpace(name), "type": label, "task_type": taskType,
			"logical_date": logicalDate.Format("2006-01-02"), "checkin_time": checkinTime.Format(time.RFC3339),
			"changed_at": changedAt.UTC().Format(time.RFC3339Nano), "is_retro": isRetro,
		})
		lastTime, lastID = changedAt, id
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "bot_events_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cursor": botEventCursor(lastTime, lastID), "events": events})
}

func botEventCursor(at time.Time, id uint64) string {
	return at.UTC().Format(time.RFC3339Nano) + "," + strconv.FormatUint(id, 10)
}

func parseBotEventCursor(value string) (time.Time, uint64, error) {
	atText, idText, ok := strings.Cut(value, ",")
	if !ok {
		return time.Time{}, 0, errors.New("invalid cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, atText)
	if err != nil {
		return time.Time{}, 0, err
	}
	id, err := strconv.ParseUint(idText, 10, 64)
	return at.UTC(), id, err
}

func botTaskLabel(taskType string, values ...string) string {
	switch taskType {
	case "daily_devotion":
		return "每日灵修"
	case "daily_scripture":
		for _, value := range values {
			if label := strings.TrimSpace(value); label != "" {
				return label
			}
		}
		return "每日读经"
	case "weekly_book":
		return "周读物"
	case "weekly_video":
		return "周视频"
	case "weekly_verse":
		return "周背经"
	case "weekly_outline":
		for _, value := range values {
			if label := strings.TrimSpace(value); label != "" {
				return label
			}
		}
		return "提纲背诵"
	}
	for _, value := range values {
		if label := strings.TrimSpace(value); label != "" {
			return label
		}
	}
	return taskType
}

func botTaskType(value string) string {
	switch strings.TrimSpace(value) {
	case "每日灵修", "灵修", "daily_devotion":
		return "daily_devotion"
	case "周读物", "读物", "weekly_book":
		return "weekly_book"
	case "周视频", "视频", "weekly_video":
		return "weekly_video"
	case "周背经", "背经", "weekly_verse":
		return "weekly_verse"
	case "提纲背诵", "weekly_outline":
		return "weekly_outline"
	default:
		return ""
	}
}

func mapID(value any) uint64 {
	switch number := value.(type) {
	case uint64:
		return number
	case int64:
		return uint64(number)
	case int:
		return uint64(number)
	case float64:
		return uint64(number)
	}
	return 0
}

func mapText(value any) string { text, _ := value.(string); return strings.TrimSpace(text) }

func (a *app) handleBotCreateCheckin(w http.ResponseWriter, r *http.Request) {
	group, err := a.botGroup(r)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bot_group_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_group_failed")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		LogicalDate string `json:"logicalDate"`
		IsRetro     bool   `json:"isRetro"`
		Detail      string `json:"detail"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	members, err := a.users.Members(r.Context(), group.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_members_failed")
		return
	}
	var userID uint64
	for _, member := range members {
		name := strings.TrimSpace(member.MemberName)
		if name == "" {
			name = strings.TrimSpace(member.DisplayName)
		}
		if name == strings.TrimSpace(req.Name) {
			if userID != 0 {
				writeError(w, http.StatusConflict, "bot_member_name_ambiguous")
				return
			}
			userID = member.UserID
		}
	}
	if userID == 0 {
		writeError(w, http.StatusNotFound, "bot_member_not_found")
		return
	}
	taskType := botTaskType(req.Type)
	if taskType == "" {
		writeError(w, http.StatusBadRequest, "invalid_task_type")
		return
	}
	if req.LogicalDate == "" {
		req.LogicalDate = time.Now().In(a.location).Format("2006-01-02")
	}
	date, err := time.ParseInLocation("2006-01-02", req.LogicalDate, a.location)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_logical_date")
		return
	}
	today := time.Now().In(a.location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, a.location)
	if date.After(today) {
		writeError(w, http.StatusBadRequest, "future_checkin_not_allowed")
		return
	}
	record := &checkindomain.Record{GroupID: group.ID, UserID: userID, LogicalDate: req.LogicalDate, TaskType: taskType, Detail: strings.TrimSpace(req.Detail), IsRetro: req.IsRetro}
	if taskType != "daily_devotion" {
		weeks, loadErr := a.learning.ListWeeks(r.Context(), group.ID)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "bot_week_failed")
			return
		}
		for _, week := range weeks {
			if req.LogicalDate >= week.Start && req.LogicalDate <= week.End {
				record.WeekID = week.ID
				break
			}
		}
		if record.WeekID == 0 {
			writeError(w, http.StatusBadRequest, "bot_week_not_found_for_date")
			return
		}
		tasks, loadErr := a.learning.WeekTasks(r.Context(), group.ID, record.WeekID)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "bot_tasks_failed")
			return
		}
		for _, task := range tasks {
			if mapText(task["task_type"]) != taskType {
				continue
			}
			if record.Detail != "" && !strings.EqualFold(record.Detail, mapText(task["title"])) {
				continue
			}
			record.TaskID, record.Detail = mapID(task["id"]), mapText(task["title"])
			break
		}
		if record.TaskID == 0 {
			writeError(w, http.StatusBadRequest, "bot_task_not_found_for_date")
			return
		}
	}
	id, existing, err := a.checkins.Create(r.Context(), record, userID)
	if errors.Is(err, checkindomain.ErrInvalidWeeklyTarget) {
		writeError(w, http.StatusBadRequest, "invalid_checkin_target")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "checkin_save_failed")
		return
	}
	if !existing && a.notifications != nil {
		if notifyErr := a.notifications.Enqueue(notificationdomain.Event{RecordID: id, GroupID: group.ID, LogicalDate: record.LogicalDate, OccurredAt: time.Now().UTC()}); notifyErr != nil {
			slog.ErrorContext(r.Context(), "bot checkin notification enqueue failed", "record_id", id, "group_id", group.ID, "error", notifyErr)
		}
	}
	a.audit(group.ID, userID, "bot_create_checkin", "checkin_records", id, nil, map[string]any{"logical_date": record.LogicalDate, "task_type": record.TaskType, "task_id": record.TaskID, "week_id": record.WeekID}, r)
	status := http.StatusCreated
	if existing {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"id": id, "existing": existing, "task_type": record.TaskType, "task_id": record.TaskID, "week_id": record.WeekID, "detail": record.Detail})
}

func (a *app) handleBotDeleteCheckin(w http.ResponseWriter, r *http.Request) {
	group, err := a.botGroup(r)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bot_group_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "bot_group_failed")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid_checkin_id")
		return
	}
	if err := a.checkins.DeleteAny(r.Context(), group.ID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed")
		return
	}
	a.audit(group.ID, 0, "bot_delete_checkin", "checkin_records", id, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
