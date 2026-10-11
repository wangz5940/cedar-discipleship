package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type personalBookPlan struct {
	Date      string `json:"date"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	Section   uint32 `json:"section"`
	PageStart uint32 `json:"page_start"`
	PageEnd   uint32 `json:"page_end"`
	Completed bool   `json:"completed"`
}

func (a *app) personalBookMember(w http.ResponseWriter, r *http.Request) bool {
	u := mustUser(r)
	if requireGroupID(w, u) == 0 {
		return false
	}
	member, err := a.learningReminderMember(r, u.CurrentGroupID, u.ID)
	if err != nil {
		writeError(w, 500, "personal_books_failed")
		return false
	}
	if !member {
		writeError(w, 403, "group_membership_required")
		return false
	}
	return true
}

func (a *app) handlePersonalBooks(w http.ResponseWriter, r *http.Request) {
	if !a.personalBookMember(w, r) {
		return
	}
	u := mustUser(r)
	rows, err := a.db.QueryContext(r.Context(), `SELECT p.logical_date,p.title,p.path,p.content_type,p.section_number,p.page_start,p.page_end,c.completed_at IS NOT NULL
 FROM personal_daily_books p LEFT JOIN personal_book_completions c ON c.group_id=p.group_id AND c.user_id=p.user_id AND c.logical_date=p.logical_date
 WHERE p.group_id=? AND p.user_id=? ORDER BY p.logical_date`, u.CurrentGroupID, u.ID)
	if err != nil {
		writeError(w, 500, "personal_books_failed")
		return
	}
	defer rows.Close()
	plans := []personalBookPlan{}
	for rows.Next() {
		var p personalBookPlan
		var date time.Time
		if err := rows.Scan(&date, &p.Title, &p.Path, &p.Type, &p.Section, &p.PageStart, &p.PageEnd, &p.Completed); err != nil {
			writeError(w, 500, "personal_books_failed")
			return
		}
		p.Date = date.Format("2006-01-02")
		plans = append(plans, p)
	}
	if rows.Err() != nil {
		writeError(w, 500, "personal_books_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"plans": plans})
}

func (a *app) handleSavePersonalBook(w http.ResponseWriter, r *http.Request) {
	if !a.personalBookMember(w, r) {
		return
	}
	u := mustUser(r)
	var p personalBookPlan
	if !readJSON(w, r, &p) {
		return
	}
	p.Title = strings.TrimSpace(p.Title)
	_, dateErr := time.Parse("2006-01-02", p.Date)
	assetPart := strings.TrimSuffix(strings.TrimPrefix(p.Path, "/api/assets/"), "/download")
	assetID, assetErr := strconv.ParseUint(assetPart, 10, 64)
	if dateErr != nil || p.Title == "" || utf8.RuneCountInString(p.Title) > 255 ||
		assetErr != nil || assetID == 0 ||
		p.Path != "/api/assets/"+strconv.FormatUint(assetID, 10)+"/download" ||
		(p.Type != "pdf" && p.Type != "markdown") ||
		(p.Type == "pdf" && (p.PageStart == 0 || p.PageEnd < p.PageStart)) ||
		(p.Type == "markdown" && p.Section == 0) {
		writeError(w, 400, "invalid_personal_book")
		return
	}
	// Use the same resource-access check as the download entry point.
	if _, err := a.assets.DownloadFile(r.Context(), u.CurrentGroupID, assetID); err != nil {
		writeError(w, 404, "asset_not_found")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO personal_daily_books(group_id,user_id,logical_date,title,path,content_type,section_number,page_start,page_end) VALUES(?,?,?,?,?,?,?,?,?)
 ON DUPLICATE KEY UPDATE title=VALUES(title),path=VALUES(path),content_type=VALUES(content_type),section_number=VALUES(section_number),page_start=VALUES(page_start),page_end=VALUES(page_end)`, u.CurrentGroupID, u.ID, p.Date, p.Title, p.Path, p.Type, p.Section, p.PageStart, p.PageEnd)
	if err != nil {
		writeError(w, 500, "personal_books_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handleDeletePersonalBook(w http.ResponseWriter, r *http.Request) {
	if !a.personalBookMember(w, r) {
		return
	}
	u := mustUser(r)
	if _, err := time.Parse("2006-01-02", r.PathValue("date")); err != nil {
		writeError(w, 400, "invalid_date")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `DELETE FROM personal_daily_books WHERE group_id=? AND user_id=? AND logical_date=?`, u.CurrentGroupID, u.ID, r.PathValue("date"))
	if err != nil {
		writeError(w, 500, "personal_books_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handlePersonalBookCompletion(w http.ResponseWriter, r *http.Request) {
	if !a.personalBookMember(w, r) {
		return
	}
	u := mustUser(r)
	date := r.PathValue("date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeError(w, 400, "invalid_date")
		return
	}
	if date > time.Now().In(a.location).Format("2006-01-02") {
		writeError(w, 400, "future_date_forbidden")
		return
	}
	var err error
	if r.Method == http.MethodDelete {
		_, err = a.db.ExecContext(r.Context(), `DELETE FROM personal_book_completions WHERE group_id=? AND user_id=? AND logical_date=?`, u.CurrentGroupID, u.ID, date)
	} else {
		result, execErr := a.db.ExecContext(r.Context(), `INSERT INTO personal_book_completions(group_id,user_id,logical_date,completed_at) SELECT group_id,user_id,logical_date,? FROM personal_daily_books WHERE group_id=? AND user_id=? AND logical_date=? ON DUPLICATE KEY UPDATE completed_at=completed_at`, time.Now().UTC(), u.CurrentGroupID, u.ID, date)
		err = execErr
		if err == nil {
			var count int64
			count, err = result.RowsAffected()
			if err == nil && count == 0 {
				var exists bool
				err = a.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM personal_daily_books WHERE group_id=? AND user_id=? AND logical_date=?)`, u.CurrentGroupID, u.ID, date).Scan(&exists)
				if err == nil && !exists {
					writeError(w, 404, "personal_book_not_found")
					return
				}
			}
		}
	}
	if err != nil {
		writeError(w, 500, "personal_books_failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
