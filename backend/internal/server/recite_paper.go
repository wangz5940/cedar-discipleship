package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"unicode/utf8"
)

// recitePaper preserves the submitted text and answers, independently of later plan edits.
// Version 1 uses the frontend's Chinese-whitespace tokenizer and continuous-blank grading.
// Version 2 additionally keeps book names and numeric characters outside blanks.
// Existing character counts remain the source of the saved score.
type recitePaper struct {
	Version      int      `json:"version"`
	Text         string   `json:"text"`
	BlankIndexes []int    `json:"blank_indexes"`
	Answers      []string `json:"answers"`
}

func validRecitePaper(paper *recitePaper) bool {
	if paper == nil {
		return true // Older clients save only counts.
	}
	length := utf8.RuneCountInString(paper.Text)
	if (paper.Version != 1 && paper.Version != 2) || length == 0 || length > 30000 ||
		len(paper.BlankIndexes) == 0 || len(paper.BlankIndexes) > 10000 ||
		len(paper.BlankIndexes) != len(paper.Answers) {
		return false
	}
	previous, answerLength := -1, 0
	for index, tokenIndex := range paper.BlankIndexes {
		if tokenIndex <= previous || tokenIndex >= length {
			return false
		}
		previous = tokenIndex
		answerLength += utf8.RuneCountInString(paper.Answers[index])
		if answerLength > 30000 {
			return false
		}
	}
	return true
}

func (a *app) handleGetRecitePaper(w http.ResponseWriter, r *http.Request) {
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
	var owner uint64
	var paper sql.NullString
	err = a.db.QueryRowContext(r.Context(), `SELECT user_id,CAST(paper AS CHAR)
		FROM recite_attempts WHERE id=? AND group_id=?`, id, groupID).Scan(&owner, &paper)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "recite_attempt_not_found")
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "recite paper lookup failed", "error", err)
		writeError(w, http.StatusInternalServerError, "recite_paper_failed")
		return
	}
	if owner != u.ID && !u.IsSuperAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if !a.requireReciteGroupMember(w, r, groupID, owner, u.ID) {
		return
	}
	var value json.RawMessage
	if paper.Valid {
		value = json.RawMessage(paper.String)
	}
	writeJSON(w, http.StatusOK, map[string]any{"paper": value})
}
