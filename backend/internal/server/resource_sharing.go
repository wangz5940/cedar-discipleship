package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	assetdomain "agp/backend/internal/asset"
)

func (a *app) handleResourceGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.users.AllGroups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "groups_failed")
		return
	}
	currentGroupID := mustUser(r).CurrentGroupID
	filtered := make([]group, 0, len(groups))
	for _, item := range groups {
		if item.ID != currentGroupID {
			filtered = append(filtered, item)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"study_groups": filtered})
}

func (a *app) handleSharedResources(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	ownerGroupID, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("owner_group_id")), 10, 64)
	items, err := a.assets.SharedResources(r.Context(), groupID, assetdomain.SharedFilter{
		OwnerGroupID: ownerGroupID,
		Category:     strings.TrimSpace(r.URL.Query().Get("type")),
		Status:       strings.TrimSpace(r.URL.Query().Get("status")),
	})
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": items})
}

func (a *app) handleAssetSharing(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	assetID := pathUint64(r, "id")
	settings, err := a.assets.ShareSettings(r.Context(), groupID, assetID)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sharing": settings})
}

func (a *app) handleUpdateAssetSharing(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	assetID := pathUint64(r, "id")
	var input assetdomain.ShareInput
	if !readJSON(w, r, &input) {
		return
	}
	if err := a.assets.SaveShareSettings(r.Context(), groupID, assetID, u.ID, input); err != nil {
		a.writeAssetError(w, err)
		return
	}
	action := "share_asset"
	if input.Scope == "" || input.Scope == assetdomain.ShareScopePrivate {
		action = "revoke_asset_share"
	}
	a.audit(groupID, u.ID, action, "assets", assetID, nil, input, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleResourceImportPreview(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input assetdomain.ImportInput
	if !readJSON(w, r, &input) {
		return
	}
	preview, err := a.assets.ImportPreview(r.Context(), groupID, input)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"preview": preview})
}

func (a *app) handleResourceImport(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input assetdomain.ImportInput
	if !readJSON(w, r, &input) {
		return
	}
	item, err := a.assets.Import(r.Context(), groupID, u.ID, input)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	a.audit(groupID, u.ID, "import_asset", "assets", item.ID, nil, map[string]any{
		"source_asset_id": input.SourceAssetID,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"asset": item})
}

func (a *app) handleBatchAssetSharing(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input assetdomain.BatchShareInput
	if !readJSON(w, r, &input) {
		return
	}
	result, err := a.assets.BatchSaveShareSettings(r.Context(), groupID, u.ID, input)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	a.audit(groupID, u.ID, "batch_share_assets", "assets", 0, nil, map[string]any{
		"asset_ids": result.AssetIDs,
		"scope":     result.Scope,
		"count":     result.Count,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *app) handleBatchDeleteAssets(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input assetdomain.BatchDeleteInput
	if !readJSON(w, r, &input) {
		return
	}
	result, err := a.assets.BatchDelete(r.Context(), groupID, u.ID, input)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	a.audit(groupID, u.ID, "batch_delete_assets", "assets", 0, nil, map[string]any{
		"asset_ids":      result.DeletedIDs,
		"count":          result.Count,
		"owned_count":    result.OwnedCount,
		"imported_count": result.ImportedCount,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (a *app) handleBatchResourceImport(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var input assetdomain.BatchImportInput
	if !readJSON(w, r, &input) {
		return
	}
	result, err := a.assets.BatchImport(r.Context(), groupID, u.ID, input)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	a.audit(groupID, u.ID, "batch_import_assets", "assets", 0, nil, map[string]any{
		"source_asset_ids":   result.SourceAssetIDs,
		"imported_asset_ids": result.ImportedAssetIDs,
		"count":              result.Count,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"result": result})
}

func (a *app) handleResourceImportHistory(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	events, err := a.assets.ImportHistory(r.Context(), groupID, limit)
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (a *app) handleRemoveResourceImport(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	importedAssetID := pathUint64(r, "id")
	if err := a.assets.RemoveImport(r.Context(), groupID, importedAssetID, u.ID); err != nil {
		a.writeAssetError(w, err)
		return
	}
	a.audit(groupID, u.ID, "remove_imported_asset", "assets", importedAssetID, nil, nil, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleResourceDependencyGraph(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	filterGroupID, _ := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("group_id")), 10, 64)
	depth, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("depth")))
	graph, err := a.assets.DependencyGraph(r.Context(), groupID, u.IsSuperAdmin, assetdomain.DependencyFilter{
		GroupID:  filterGroupID,
		Category: strings.TrimSpace(r.URL.Query().Get("type")),
		Strength: strings.TrimSpace(r.URL.Query().Get("strength")),
		Depth:    depth,
	})
	if err != nil {
		a.writeAssetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (a *app) writeAssetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, assetdomain.ErrInvalidBatchInput):
		writeError(w, http.StatusBadRequest, "invalid_batch_input")
	case errors.Is(err, assetdomain.ErrInvalidShareScope):
		writeError(w, http.StatusBadRequest, "invalid_share_scope")
	case errors.Is(err, assetdomain.ErrInvalidGroupCode):
		writeError(w, http.StatusBadRequest, "invalid_group_code")
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "asset_not_found")
	case errors.Is(err, assetdomain.ErrSharingUnsupported):
		writeError(w, http.StatusInternalServerError, "asset_sharing_unsupported")
	default:
		writeError(w, http.StatusInternalServerError, "asset_operation_failed")
	}
}
