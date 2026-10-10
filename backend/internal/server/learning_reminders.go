package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

func (a *app) learningReminderMember(r *http.Request, groupID, userID uint64) (bool, error) {
	var member bool
	err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=? AND user_id=? AND status=1)`, groupID, userID).Scan(&member)
	return member, err
}

func (a *app) handleCreateLearningReminder(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		UserID uint64 `json:"user_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.UserID == 0 || req.UserID == u.ID {
		writeError(w, 400, "invalid_reminder_recipient")
		return
	}
	for _, id := range []uint64{u.ID, req.UserID} {
		if id == u.ID && u.IsSuperAdmin {
			continue
		}
		member, err := a.learningReminderMember(r, groupID, id)
		if err != nil {
			writeError(w, 500, "reminder_failed")
			return
		}
		if !member {
			writeError(w, 403, "group_membership_required")
			return
		}
	}
	now := time.Now().In(a.location)
	date := now.Format("2006-01-02")
	content, _, err := a.todayContent(r.Context(), groupID, date, now)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	hub, err := a.learning.TodayHubFromContent(r.Context(), groupID, req.UserID, content)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	if !u.IsSuperAdmin && (hub.Progress.Total == 0 || hub.Progress.Completed >= hub.Progress.Total) {
		writeError(w, 409, "member_tasks_completed")
		return
	}
	var dailyLimitSlot any = 1
	if u.IsSuperAdmin {
		dailyLimitSlot = nil
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO learning_reminders(group_id,sender_id,recipient_id,logical_date,created_at,daily_limit_slot) VALUES(?,?,?,?,?,?)`, groupID, u.ID, req.UserID, date, now.UTC(), dailyLimitSlot)
	var duplicate *mysql.MySQLError
	if errors.As(err, &duplicate) && duplicate.Number == 1062 {
		writeError(w, 409, "already_reminded_today")
		return
	}
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (a *app) handleLearningReminders(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	member, err := a.learningReminderMember(r, groupID, u.ID)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	if !member && !u.IsSuperAdmin {
		writeError(w, 403, "group_membership_required")
		return
	}
	muted := false
	var applePushAfterID uint64
	err = a.db.QueryRowContext(r.Context(), `SELECT muted,apple_push_after_id FROM learning_reminder_preferences WHERE group_id=? AND user_id=?`, groupID, u.ID).Scan(&muted, &applePushAfterID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeError(w, 500, "reminder_failed")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT r.id,r.logical_date,CASE WHEN r.sender_id=r.recipient_id THEN '每日灵修' ELSE COALESCE(NULLIF(gm.member_name,''),u.display_name) END,r.read_at IS NOT NULL FROM learning_reminders r JOIN users u ON u.id=r.sender_id LEFT JOIN group_members gm ON gm.group_id=r.group_id AND gm.user_id=r.sender_id WHERE r.group_id=? AND r.recipient_id=? AND (r.read_at IS NULL OR r.logical_date=?) ORDER BY (r.read_at IS NULL) DESC,r.id DESC LIMIT 200`, groupID, u.ID, time.Now().In(a.location).Format("2006-01-02"))
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	deliveries := make([]map[string]any, 0)
	for rows.Next() {
		var id uint64
		var date time.Time
		var sender string
		var read bool
		if rows.Scan(&id, &date, &sender, &read) != nil {
			writeError(w, 500, "reminder_failed")
			return
		}
		item := map[string]any{"id": id, "date": date.Format("2006-01-02"), "sender": sender, "apple_push_eligible": id > applePushAfterID}
		deliveries = append(deliveries, item)
		if !read && len(items) < 100 {
			items = append(items, item)
		}
	}
	if rows.Err() != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	var sentRows *sql.Rows
	sentRows, err = a.db.QueryContext(r.Context(), `SELECT DISTINCT recipient_id FROM learning_reminders WHERE group_id=? AND sender_id=? AND logical_date=?`, groupID, u.ID, time.Now().In(a.location).Format("2006-01-02"))
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	defer sentRows.Close()
	sent := make([]uint64, 0)
	for sentRows.Next() {
		var id uint64
		if sentRows.Scan(&id) != nil {
			writeError(w, 500, "reminder_failed")
			return
		}
		sent = append(sent, id)
	}
	if sentRows.Err() != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "deliveries": deliveries, "muted": muted, "sent_ids": sent})
}

func (a *app) handleReadLearningReminder(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, 400, "invalid_reminder")
		return
	}
	result, err := a.db.ExecContext(r.Context(), `UPDATE learning_reminders SET read_at=COALESCE(read_at,?) WHERE id=? AND group_id=? AND recipient_id=?`, time.Now().UTC(), id, groupID, u.ID)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	if n == 0 {
		var exists bool
		err = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM learning_reminders WHERE id=? AND group_id=? AND recipient_id=?)`, id, groupID, u.ID).Scan(&exists)
		if err != nil {
			writeError(w, 500, "reminder_failed")
			return
		}
		if !exists {
			writeError(w, 404, "reminder_not_found")
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handleReadAllLearningReminders(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		ThroughID uint64 `json:"through_id"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.ThroughID == 0 {
		writeError(w, 400, "invalid_reminder")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `UPDATE learning_reminders SET read_at=? WHERE group_id=? AND recipient_id=? AND id<=? AND read_at IS NULL`, time.Now().UTC(), groupID, u.ID, req.ThroughID)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handleLearningReminderPreference(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		Muted *bool `json:"muted"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Muted == nil {
		writeError(w, 400, "invalid_reminder_preference")
		return
	}
	member, err := a.learningReminderMember(r, groupID, u.ID)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	if !member {
		writeError(w, 403, "group_membership_required")
		return
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO learning_reminder_preferences(group_id,user_id,muted) VALUES(?,?,?) ON DUPLICATE KEY UPDATE apple_push_after_id=IF(muted=TRUE AND VALUES(muted)=FALSE,(SELECT COALESCE(MAX(id),0) FROM learning_reminders WHERE group_id=? AND recipient_id=?),apple_push_after_id),muted=VALUES(muted)`, groupID, u.ID, *req.Muted, groupID, u.ID)
	if err != nil {
		writeError(w, 500, "reminder_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
