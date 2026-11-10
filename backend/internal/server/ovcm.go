package server

import "net/http"

// Catalogue metadata was already public in ovcm-courses.json. Playback access
// and account progress continue through their existing authorization paths.
func (a *app) handleOvcmCatalogue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if a.ovcmCatalogue == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ovcm_catalogue_unavailable"})
		return
	}
	snapshot, ok := a.ovcmCatalogue.Snapshot()
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ovcm_catalogue_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
