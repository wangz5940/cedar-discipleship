package server

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	assetdomain "agp/backend/internal/asset"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

func (a *app) handleListAssets(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	items, err := a.assets.List(r.Context(), groupID, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "assets_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": items})
}

func (a *app) handleDownloadAsset(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	file, err := a.assets.DownloadFile(r.Context(), groupID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "asset_not_found")
		return
	}
	mt := file.MimeType
	if mt == "" {
		mt = mime.TypeByExtension(filepath.Ext(file.OriginalName))
	}
	w.Header().Set("Content-Type", mt)
	http.ServeFile(w, r, file.AbsolutePath)
}

func (a *app) handleDownloadAssetRange(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	file, err := a.assets.DownloadFile(r.Context(), groupID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "asset_not_found")
		return
	}
	pages, err := normalizePageRange(r.URL.Query().Get("pages"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pages")
		return
	}
	if strings.ToLower(filepath.Ext(file.OriginalName)) != ".pdf" {
		writeError(w, http.StatusBadRequest, "asset_not_pdf")
		return
	}
	if err := servePDFRange(w, file.AbsolutePath, file.OriginalName, pages); err != nil {
		writeError(w, http.StatusInternalServerError, "pdf_range_failed")
	}
}

func (a *app) handleStaticPDFRange(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	pages, err := normalizePageRange(r.URL.Query().Get("pages"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pages")
		return
	}
	src := strings.TrimSpace(r.URL.Query().Get("path"))
	abs, original, err := assetdomain.ResolveFileInRoot(a.contentRoot, src)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_pdf_path")
		return
	}
	if strings.ToLower(filepath.Ext(original)) != ".pdf" {
		writeError(w, http.StatusBadRequest, "asset_not_pdf")
		return
	}
	if err := servePDFRange(w, abs, original, pages); err != nil {
		writeError(w, http.StatusInternalServerError, "pdf_range_failed")
	}
}

func normalizePageRange(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", errors.New("empty_pages")
	}
	parts := strings.SplitN(raw, "-", 2)
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || start < 1 {
		return "", errors.New("invalid_start")
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || end < start {
			return "", errors.New("invalid_end")
		}
	}
	return fmt.Sprintf("%d-%d", start, end), nil
}

func servePDFRange(w http.ResponseWriter, srcPath, original, pages string) error {
	tmp, err := os.CreateTemp("", "agp-pdf-range-*.pdf")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := pdfapi.TrimFile(srcPath, tmp.Name(), []string{pages}, nil); err != nil {
		return err
	}
	file, err := os.Open(tmp.Name())
	if err != nil {
		return err
	}
	defer file.Close()
	if info, statErr := file.Stat(); statErr == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(original)))
	_, err = io.Copy(w, file)
	return err
}

func (a *app) handleAdminUploadAsset(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload_form")
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		category = "uploaded"
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file_required")
		return
	}
	defer file.Close()
	item, err := a.assets.Upload(r.Context(), assetdomain.UploadRequest{
		GroupID:  groupID,
		ActorID:  u.ID,
		Category: category,
		FileName: header.Filename,
		Reader:   file,
	})
	if err != nil {
		if errors.Is(err, assetdomain.ErrInvalidFilename) {
			writeError(w, http.StatusBadRequest, "invalid_filename")
			return
		}
		if errors.Is(err, assetdomain.ErrStorageDirectory) {
			writeError(w, http.StatusInternalServerError, "asset_dir_failed")
			return
		}
		if errors.Is(err, assetdomain.ErrStorageWrite) {
			writeError(w, http.StatusInternalServerError, "asset_write_failed")
			return
		}
		writeError(w, http.StatusInternalServerError, "asset_save_failed")
		return
	}
	a.audit(groupID, u.ID, "upload_asset", "assets", item.ID, nil, map[string]any{"title": item.Title, "category": item.Category}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"asset": item})
}

func (a *app) handleAdminResourceLibrary(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	sections, err := a.assets.ResourceLibrary(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resource_library_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": sections})
}

func (a *app) handleResourceLibrary(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	sections, err := a.assets.ResourceLibrary(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resource_library_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": sections})
}

func (a *app) handleAdminCreateAsset(w http.ResponseWriter, r *http.Request) {
	u := mustUser(r)
	groupID := requireGroupID(w, u)
	if groupID == 0 {
		return
	}
	var req struct {
		Category    string `json:"category"`
		Title       string `json:"title"`
		StoragePath string `json:"storage_path"`
		MimeType    string `json:"mime_type"`
		FileSize    uint64 `json:"file_size"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	id, err := a.assets.CreateMetadata(r.Context(), assetdomain.CreateRequest{
		GroupID:     groupID,
		ActorID:     u.ID,
		Category:    req.Category,
		Title:       req.Title,
		StoragePath: req.StoragePath,
		MimeType:    req.MimeType,
		FileSize:    req.FileSize,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "asset_save_failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}
