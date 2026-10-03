package server

import (
	assetdomain "agp/backend/internal/asset"
	"agp/backend/internal/studymemory"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
)

func (a *app) handleStudyAccess(w http.ResponseWriter, r *http.Request) {
	unlocked, err := a.studyLearning.Access(r.Context(), mustUser(r).ID, a.ovcmKeyHash)
	if err != nil {
		writeError(w, 500, "study_access_failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"unlocked": unlocked})
}
func (a *app) handleToggleStudyAccess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if a.ovcmKeyHash == "" {
		writeError(w, 503, "study_key_not_configured")
		return
	}
	hash := sha256.Sum256([]byte(req.Key))
	supplied := hex.EncodeToString(hash[:])
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(a.ovcmKeyHash)) != 1 {
		writeError(w, 403, "invalid_study_key")
		return
	}
	id := mustUser(r).ID
	unlocked, err := a.studyLearning.Access(r.Context(), id, a.ovcmKeyHash)
	if err != nil {
		writeError(w, 500, "study_access_failed")
		return
	}
	if unlocked {
		err = a.studyLearning.Revoke(r.Context(), id)
	} else {
		err = a.studyLearning.Unlock(r.Context(), id, a.ovcmKeyHash)
	}
	if err != nil {
		writeError(w, 500, "study_access_failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"unlocked": !unlocked})
}
func (a *app) handleMediaHandout(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	group := requireGroupID(w, u)
	if group == 0 {
		return
	}
	media := pathUint64(r, "id")
	asset, err := a.assets.Find(r.Context(), group, media)
	if err != nil {
		writeError(w, 404, "asset_not_found")
		return
	}
	if !isPlaybackMediaAsset(&assetdomain.DownloadFile{MimeType: asset.MimeType, OriginalName: asset.OriginalName}) {
		writeError(w, 400, "asset_not_video")
		return
	}
	if r.Method == http.MethodPut {
		var req struct {
			Handout *studymemory.Handout `json:"handout"`
			GroupID uint64               `json:"groupId"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if req.GroupID != group {
			writeError(w, 409, "study_group_changed")
			return
		}
		if req.Handout != nil {
			if !studymemory.ValidHandout(*req.Handout) {
				writeError(w, 400, "invalid_handout")
				return
			}
			pdf, err := a.assets.Find(r.Context(), group, req.Handout.AssetID)
			if err != nil {
				writeError(w, 404, "handout_not_found")
				return
			}
			if pdf.MimeType != "application/pdf" && !strings.EqualFold(filepath.Ext(pdf.OriginalName), ".pdf") {
				writeError(w, 400, "handout_not_pdf")
				return
			}
		}
		if err := a.studyLearning.SaveHandout(r.Context(), group, media, req.Handout); err != nil {
			writeError(w, 500, "handout_save_failed")
			return
		}
	}
	h, err := a.studyLearning.Handout(r.Context(), group, media)
	if err != nil {
		writeError(w, 500, "handout_load_failed")
		return
	}
	// Check the PDF still belongs to the active group's accessible resource bindings.
	if h != nil {
		if _, err := a.assets.Find(r.Context(), group, h.AssetID); err != nil {
			h = nil
		}
	}
	writeJSON(w, 200, map[string]any{"handout": h})
}
