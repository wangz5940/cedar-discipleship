package server

import (
	"agp/backend/internal/studymemory"
	"net/http"
	"time"
)

func (a *app) handleStudyMemory(w http.ResponseWriter, r *http.Request) {
	result, err := a.studyMemory.Load(r.Context(), mustUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "study_memory_failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func (a *app) handleStudyProgress(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
		studymemory.Progress
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !studymemory.ValidKey(req.Key) || !studymemory.ValidProgress(req.Progress) {
		writeError(w, http.StatusBadRequest, "invalid_study_progress")
		return
	}
	req.UpdatedAt = time.Now().UTC().UnixMilli()
	if err := a.studyMemory.SaveProgress(r.Context(), mustUser(r).ID, req.Key, req.Progress); err != nil {
		writeError(w, http.StatusInternalServerError, "study_memory_failed")
		return
	}
	writeJSON(w, http.StatusOK, req.Progress)
}
func (a *app) handleStudyFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key      string                `json:"key"`
		Favorite bool                  `json:"favorite"`
		Item     *studymemory.Favorite `json:"item"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !studymemory.ValidKey(req.Key) || req.Favorite && (req.Item == nil || req.Item.Key != req.Key || !studymemory.ValidFavorite(*req.Item)) {
		writeError(w, http.StatusBadRequest, "invalid_study_favorite")
		return
	}
	if req.Favorite {
		req.Item.SavedAt = time.Now().UTC().UnixMilli()
	} else {
		req.Item = nil
	}
	if err := a.studyMemory.SetFavorite(r.Context(), mustUser(r).ID, req.Key, req.Item); err != nil {
		writeError(w, http.StatusInternalServerError, "study_memory_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
