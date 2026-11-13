package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	userdomain "agp/backend/internal/user"
)

func (a *app) handleAccountLinkReviews(w http.ResponseWriter, r *http.Request) {
	items, err := a.users.AccountLinkReviews(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "account_links_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviews": items})
}

func accountLinkError(w http.ResponseWriter, err error) {
	if errors.Is(err, userdomain.ErrAccountLink) || errors.Is(err, userdomain.ErrAccountLinkChanged) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "account_link_not_found")
		return
	}
	writeError(w, http.StatusInternalServerError, "account_links_failed")
}

func (a *app) handleCreateAccountLinkReview(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	var req struct {
		FirstUsername  string `json:"first_username"`
		SecondUsername string `json:"second_username"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	item, err := a.users.CreateAccountLinkReview(r.Context(), strings.ToLower(strings.TrimSpace(req.FirstUsername)), strings.ToLower(strings.TrimSpace(req.SecondUsername)), u.ID)
	if err != nil {
		accountLinkError(w, err)
		return
	}
	a.audit(0, u.ID, "request_account_link", "account_link_reviews", item.ID, nil, item, r)
	writeJSON(w, http.StatusCreated, item)
}

func (a *app) handleReviewAccountLink(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "invalid_decision")
		return
	}
	if err := a.users.ReviewAccountLink(r.Context(), id, u.ID, req.Decision == "approve"); err != nil {
		accountLinkError(w, err)
		return
	}
	a.audit(0, u.ID, "review_account_link", "account_link_reviews", id, nil, map[string]any{"decision": req.Decision}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
