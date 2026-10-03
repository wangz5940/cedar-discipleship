package server

import (
	"net/http"

	"agp/backend/internal/learning"
)

func (a *app) handleResourceDownloadSettings(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "resource_download_enabled_required")
		return
	}
	before, err := a.groupLearningConfig(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "learning_config_failed")
		return
	}
	if err := a.learning.SaveResourceDownloadEnabled(r.Context(), groupID, *req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "learning_config_save_failed")
		return
	}
	a.refreshTodayContent(groupID)
	a.auditChanges(groupID, u.ID, "set_resource_download", "group_settings", groupID,
		map[string]any{"resource_download_enabled": learning.ResourceDownloadsAllowed(before)},
		map[string]any{"resource_download_enabled": *req.Enabled}, r)
	writeJSON(w, http.StatusOK, map[string]any{"enabled": *req.Enabled})
}
