package server

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type reciteTarget struct {
	WeekID   uint64
	VerseRef string
}

type reciteAttempt struct {
	ID        uint64 `json:"id"`
	At        string `json:"at"`
	Rate      int    `json:"rate"`
	Correct   int    `json:"correct"`
	Total     int    `json:"total"`
	Score     int    `json:"score"`
	AttemptNo int    `json:"attempt_no"`
}

type reciteAdminAttempt struct {
	reciteAttempt
	GroupID  uint64  `json:"group_id"`
	UserID   uint64  `json:"user_id"`
	UserName string  `json:"user_name"`
	WeekID   *uint64 `json:"week_id"`
	VerseRef string  `json:"verse_ref"`
}

type reciteLeaderboardEntry struct {
	Name             string `json:"name"`
	Ref              string `json:"ref"`
	RankScore        int    `json:"rankScore"`
	Attempts         int    `json:"attempts"`
	BestScore        int    `json:"bestScore"`
	BestBlankPercent int    `json:"bestBlankPercent"`
	BestAccuracy     int    `json:"bestAccuracy"`
	AverageAccuracy  int    `json:"averageAccuracy"`
	LatestAt         string `json:"latestAt"`
	accuracyTotal    int
}

type reciteLeaderboardAttempt struct {
	UserID       uint64
	Name         string
	Ref          string
	Score        int
	BlankPercent int
	Accuracy     int
	At           time.Time
}

func buildReciteLeaderboard(attempts []reciteLeaderboardAttempt, location *time.Location) []reciteLeaderboardEntry {
	entries := make([]reciteLeaderboardEntry, 0)
	byUser := make(map[uint64]int)
	for _, attempt := range attempts {
		index, exists := byUser[attempt.UserID]
		if !exists {
			index = len(entries)
			byUser[attempt.UserID] = index
			entries = append(entries, reciteLeaderboardEntry{Name: attempt.Name, Ref: attempt.Ref})
		}
		entry := &entries[index]
		entry.Attempts++
		entry.accuracyTotal += attempt.Accuracy
		if attempt.Score > entry.BestScore || (attempt.Score == entry.BestScore && attempt.BlankPercent > entry.BestBlankPercent) {
			entry.RankScore = attempt.Score
			entry.BestScore = attempt.Score
			entry.BestBlankPercent = attempt.BlankPercent
			entry.BestAccuracy = attempt.Accuracy
		}
		at := attempt.At.In(location).Format(time.RFC3339)
		if at > entry.LatestAt {
			entry.LatestAt = at
		}
	}
	for index := range entries {
		entry := &entries[index]
		entry.AverageAccuracy = (entry.accuracyTotal + entry.Attempts/2) / entry.Attempts
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if left.BestAccuracy != right.BestAccuracy {
			return left.BestAccuracy > right.BestAccuracy
		}
		if left.BestBlankPercent != right.BestBlankPercent {
			return left.BestBlankPercent > right.BestBlankPercent
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Ref < right.Ref
	})
	return entries
}

func (a *app) handleReciteLeaderboard(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	taskID, err := strconv.ParseUint(r.URL.Query().Get("task_id"), 10, 64)
	if err != nil || taskID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_task_id")
		return
	}
	target, err := a.reciteTarget(r, groupID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "recite_task_not_found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "recite leaderboard target lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_leaderboard_failed")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT ra.user_id,
		COALESCE(NULLIF(gm.member_name,''),u.display_name), ra.verse_ref,
		ra.score,ra.blank_percent,ra.accuracy,ra.created_at
		FROM recite_attempts ra
		JOIN users u ON u.id=ra.user_id
		LEFT JOIN group_members gm ON gm.group_id=ra.group_id AND gm.user_id=ra.user_id
		WHERE ra.group_id=? AND ra.week_id=? AND ra.verse_ref=?`, groupID, target.WeekID, target.VerseRef)
	if err != nil {
		slog.ErrorContext(r.Context(), "recite leaderboard query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_leaderboard_failed")
		return
	}
	defer rows.Close()
	attempts := make([]reciteLeaderboardAttempt, 0)
	for rows.Next() {
		var item reciteLeaderboardAttempt
		if err := rows.Scan(&item.UserID, &item.Name, &item.Ref, &item.Score, &item.BlankPercent, &item.Accuracy, &item.At); err != nil {
			writeError(w, http.StatusInternalServerError, "recite_leaderboard_failed")
			return
		}
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "recite_leaderboard_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": buildReciteLeaderboard(attempts, a.location)})
}

func (a *app) reciteTarget(r *http.Request, groupID, taskID uint64) (reciteTarget, error) {
	var target reciteTarget
	err := a.db.QueryRowContext(r.Context(), `SELECT t.week_id,
		COALESCE(NULLIF(w.verse_ref,''),t.title)
		FROM study_tasks t JOIN study_weeks w ON w.id=t.week_id AND w.group_id=t.group_id
		WHERE t.id=? AND t.group_id=? AND t.task_type='weekly_verse' AND t.enabled=1`, taskID, groupID).
		Scan(&target.WeekID, &target.VerseRef)
	if err != nil {
		return target, err
	}
	return target, nil
}

// An omitted user_id preserves the existing personal-history and personal-save behavior.
func requestedReciteUserID(w http.ResponseWriter, u currentUser, raw string) (uint64, bool) {
	if raw == "" || raw == "0" {
		return u.ID, true
	}
	userID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || userID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_user_id")
		return 0, false
	}
	if userID != u.ID && !u.IsSuperAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return 0, false
	}
	return userID, true
}

func (a *app) requireReciteGroupMember(w http.ResponseWriter, r *http.Request, groupID, userID, actorID uint64) bool {
	if userID == actorID {
		return true
	}
	var exists int
	err := a.db.QueryRowContext(r.Context(), `SELECT 1 FROM group_members WHERE group_id=? AND user_id=? AND status=1`, groupID, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "member_not_found")
		return false
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "recite member lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_member_lookup_failed")
		return false
	}
	return true
}

func (a *app) handleListReciteAttempts(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	userID, ok := requestedReciteUserID(w, u, r.URL.Query().Get("user_id"))
	if !ok || !a.requireReciteGroupMember(w, r, groupID, userID, u.ID) {
		return
	}
	taskID, err := strconv.ParseUint(r.URL.Query().Get("task_id"), 10, 64)
	if err != nil || taskID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_task_id")
		return
	}
	target, err := a.reciteTarget(r, groupID, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "recite_task_not_found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "recite target lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_history_failed")
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id,blank_percent,blank_count,correct_count,score,attempt_no,created_at
		FROM recite_attempts WHERE group_id=? AND user_id=? AND week_id=? AND verse_ref=?
		ORDER BY attempt_no DESC LIMIT 30`, groupID, userID, target.WeekID, target.VerseRef)
	if err != nil {
		slog.ErrorContext(r.Context(), "recite history query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_history_failed")
		return
	}
	defer rows.Close()
	attempts := make([]reciteAttempt, 0)
	for rows.Next() {
		var item reciteAttempt
		var at time.Time
		if err := rows.Scan(&item.ID, &item.Rate, &item.Total, &item.Correct, &item.Score, &item.AttemptNo, &at); err != nil {
			writeError(w, http.StatusInternalServerError, "recite_history_failed")
			return
		}
		item.At = at.In(a.location).Format(time.RFC3339)
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "recite_history_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": attempts})
}

func (a *app) handleCreateReciteAttempt(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		TaskID  uint64 `json:"task_id"`
		UserID  uint64 `json:"user_id"`
		Rate    int    `json:"blank_percent"`
		Total   int    `json:"blank_count"`
		Correct int    `json:"correct_count"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.TaskID == 0 || req.Rate < 0 || req.Rate > 90 || req.Total < 0 || req.Total > 10000 || req.Correct < 0 || req.Correct > req.Total || (req.Rate == 0 && req.Total != 0) {
		writeError(w, http.StatusBadRequest, "invalid_recite_attempt")
		return
	}
	userID := u.ID
	if req.UserID != 0 {
		userID = req.UserID
	}
	if userID != u.ID && !u.IsSuperAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !a.requireReciteGroupMember(w, r, groupID, userID, u.ID) {
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recite_save_failed")
		return
	}
	defer tx.Rollback()
	var target reciteTarget
	err = tx.QueryRowContext(r.Context(), `SELECT t.week_id,
		COALESCE(NULLIF(w.verse_ref,''),t.title)
		FROM study_tasks t JOIN study_weeks w ON w.id=t.week_id AND w.group_id=t.group_id
		WHERE t.id=? AND t.group_id=? AND t.task_type='weekly_verse' AND t.enabled=1 FOR UPDATE`, req.TaskID, groupID).
		Scan(&target.WeekID, &target.VerseRef)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "recite_task_not_found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "recite target lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_save_failed")
		return
	}
	var attemptNo int
	if err := tx.QueryRowContext(r.Context(), `SELECT COALESCE(MAX(attempt_no),0)+1 FROM recite_attempts
		WHERE group_id=? AND user_id=? AND week_id=? AND verse_ref=?`, groupID, userID, target.WeekID, target.VerseRef).Scan(&attemptNo); err != nil {
		writeError(w, http.StatusInternalServerError, "recite_save_failed")
		return
	}
	accuracy := reciteAccuracy(req.Correct, req.Total)
	score := reciteScore(req.Correct, req.Total, req.Rate)
	now := time.Now().In(a.location)
	result, err := tx.ExecContext(r.Context(), `INSERT INTO recite_attempts
		(group_id,user_id,week_id,checkin_record_id,verse_ref,logical_date,blank_percent,blank_count,correct_count,accuracy,score,attempt_no,created_at)
		VALUES (?,?,?,NULL,?,?,?,?,?,?,?,?,?)`,
		groupID, userID, target.WeekID, target.VerseRef, now.Format("2006-01-02"), req.Rate, req.Total, req.Correct, accuracy, score, attemptNo, now.UTC())
	if err != nil {
		slog.ErrorContext(r.Context(), "recite attempt save failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_save_failed")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "recite_save_failed")
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, reciteAttempt{
		ID: uint64(id), At: now.Format(time.RFC3339), Rate: req.Rate,
		Correct: req.Correct, Total: req.Total, Score: score, AttemptNo: attemptNo,
	})
}

func (a *app) handleSuperListReciteAttempts(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	query := r.URL.Query()
	var userID uint64
	if query.Has("user_id") && query.Get("user_id") != "" {
		var err error
		userID, err = strconv.ParseUint(query.Get("user_id"), 10, 64)
		if err != nil || userID == 0 {
			writeError(w, http.StatusBadRequest, "invalid_user_id")
			return
		}
	}
	var target reciteTarget
	if query.Has("task_id") && query.Get("task_id") != "" {
		taskID, err := strconv.ParseUint(query.Get("task_id"), 10, 64)
		if err != nil || taskID == 0 {
			writeError(w, http.StatusBadRequest, "invalid_task_id")
			return
		}
		target, err = a.reciteTarget(r, groupID, taskID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "recite_task_not_found")
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "admin recite target lookup failed", "error", err)
			writeError(w, http.StatusInternalServerError, "recite_history_failed")
			return
		}
	}
	page := queryInt(r, "page", 1)
	pageSize := queryInt(r, "page_size", 100)
	if page < 1 || pageSize < 1 || pageSize > 500 || page > 1000000 {
		writeError(w, http.StatusBadRequest, "invalid_pagination")
		return
	}
	statement := `SELECT ra.id,ra.group_id,ra.user_id,
		COALESCE(NULLIF(gm.member_name,''),u.display_name),ra.week_id,ra.verse_ref,
		ra.blank_percent,ra.blank_count,ra.correct_count,ra.score,ra.attempt_no,ra.created_at
		FROM recite_attempts ra JOIN users u ON u.id=ra.user_id
		LEFT JOIN group_members gm ON gm.group_id=ra.group_id AND gm.user_id=ra.user_id
		WHERE ra.group_id=?`
	args := []any{groupID}
	if userID != 0 {
		statement += ` AND ra.user_id=?`
		args = append(args, userID)
	}
	if target.WeekID != 0 {
		statement += ` AND ra.week_id=? AND ra.verse_ref=?`
		args = append(args, target.WeekID, target.VerseRef)
	}
	statement += ` ORDER BY ra.created_at DESC,ra.id DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize+1, (page-1)*pageSize)
	rows, err := a.db.QueryContext(r.Context(), statement, args...)
	if err != nil {
		slog.ErrorContext(r.Context(), "admin recite history query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_history_failed")
		return
	}
	defer rows.Close()
	attempts := make([]reciteAdminAttempt, 0)
	for rows.Next() {
		var item reciteAdminAttempt
		var weekID sql.NullInt64
		var at time.Time
		if err := rows.Scan(&item.ID, &item.GroupID, &item.UserID, &item.UserName, &weekID, &item.VerseRef,
			&item.Rate, &item.Total, &item.Correct, &item.Score, &item.AttemptNo, &at); err != nil {
			writeError(w, http.StatusInternalServerError, "recite_history_failed")
			return
		}
		if weekID.Valid {
			id := uint64(weekID.Int64)
			item.WeekID = &id
		}
		item.At = at.In(a.location).Format(time.RFC3339)
		attempts = append(attempts, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "recite_history_failed")
		return
	}
	hasMore := len(attempts) > pageSize
	if hasMore {
		attempts = attempts[:pageSize]
	}
	writeJSON(w, http.StatusOK, map[string]any{"attempts": attempts, "page": page, "page_size": pageSize, "has_more": hasMore})
}

func (a *app) handleSuperDeleteReciteAttempt(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid_recite_attempt_id")
		return
	}
	result, err := a.db.ExecContext(r.Context(), `DELETE FROM recite_attempts WHERE id=? AND group_id=?`, id, groupID)
	if err != nil {
		slog.ErrorContext(r.Context(), "admin recite history delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_delete_failed")
		return
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		writeError(w, http.StatusNotFound, "recite_attempt_not_found")
		return
	}
	a.audit(groupID, u.ID, "delete_recite_attempt", "recite_attempts", id, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func reciteAccuracy(correct, total int) int {
	if total == 0 {
		return 0
	}
	return (correct*100 + total/2) / total
}

func reciteScore(correct, total, rate int) int {
	if total == 0 {
		return 0
	}
	return (correct*rate + total/2) / total
}
