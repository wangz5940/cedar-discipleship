package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	assetdomain "agp/backend/internal/asset"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
)

const (
	assetPlaybackTTL    = 12 * time.Hour
	assetPlaybackSuffix = ".playback.mp4"
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
	serveAssetFile(w, r, file)
}

func (a *app) handleAssetPlayback(w http.ResponseWriter, r *http.Request) {
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
	if !isVideoAsset(file) {
		writeError(w, http.StatusBadRequest, "asset_not_video")
		return
	}
	expiresAt := time.Now().Add(assetPlaybackTTL).Unix()
	signature := a.signAssetPlayback(id, groupID, expiresAt)
	playbackURL := fmt.Sprintf(
		"/api/assets/%d/stream?group_id=%d&expires=%d&signature=%s",
		id,
		groupID,
		expiresAt,
		signature,
	)
	response := map[string]any{
		"url":        playbackURL,
		"expires_at": expiresAt,
	}
	if playbackAssetFile(file).AbsolutePath != file.AbsolutePath {
		fallbackSignature := a.signAssetPlaybackSource(id, groupID, expiresAt, "original")
		response["fallback_url"] = fmt.Sprintf(
			"/api/assets/%d/stream?group_id=%d&expires=%d&source=original&signature=%s",
			id,
			groupID,
			expiresAt,
			fallbackSignature,
		)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func (a *app) handleStreamAsset(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	groupID, _ := strconv.ParseUint(r.URL.Query().Get("group_id"), 10, 64)
	expiresAt, _ := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64)
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	signature := strings.TrimSpace(r.URL.Query().Get("signature"))
	validSignature := source == "" && a.verifyAssetPlayback(id, groupID, expiresAt, signature)
	if source == "original" {
		validSignature = a.verifyAssetPlaybackSource(id, groupID, expiresAt, source, signature)
	}
	if id == 0 || groupID == 0 || expiresAt < time.Now().Unix() || !validSignature {
		writeError(w, http.StatusForbidden, "invalid_playback_url")
		return
	}
	file, err := a.assets.DownloadFile(r.Context(), groupID, id)
	if err != nil || !isVideoAsset(file) {
		writeError(w, http.StatusNotFound, "asset_not_found")
		return
	}
	if source == "original" {
		serveAssetFile(w, r, file)
		return
	}
	serveAssetFile(w, r, playbackAssetFile(file))
}

func serveAssetFile(w http.ResponseWriter, r *http.Request, file *assetdomain.DownloadFile) {
	fh, err := os.Open(file.AbsolutePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "asset_not_found")
		return
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "asset_not_found")
		return
	}
	setAssetDownloadHeaders(w, file, info)
	http.ServeContent(w, r, file.OriginalName, info.ModTime(), fh)
}

func (a *app) signAssetPlayback(assetID, groupID uint64, expiresAt int64) string {
	mac := hmac.New(sha256.New, a.secret)
	_, _ = fmt.Fprintf(mac, "%d:%d:%d", assetID, groupID, expiresAt)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *app) verifyAssetPlayback(assetID, groupID uint64, expiresAt int64, signature string) bool {
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(a.signAssetPlayback(assetID, groupID, expiresAt))
	return err == nil && hmac.Equal(expected, got)
}

func (a *app) signAssetPlaybackSource(assetID, groupID uint64, expiresAt int64, source string) string {
	mac := hmac.New(sha256.New, a.secret)
	_, _ = fmt.Fprintf(mac, "%d:%d:%d:%s", assetID, groupID, expiresAt, source)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *app) verifyAssetPlaybackSource(assetID, groupID uint64, expiresAt int64, source, signature string) bool {
	got, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(a.signAssetPlaybackSource(assetID, groupID, expiresAt, source))
	return err == nil && hmac.Equal(expected, got)
}

func isVideoAsset(file *assetdomain.DownloadFile) bool {
	if file == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(file.MimeType)), "video/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(file.OriginalName)) {
	case ".mp4", ".m4v", ".mov", ".webm":
		return true
	default:
		return false
	}
}

func playbackAssetFile(file *assetdomain.DownloadFile) *assetdomain.DownloadFile {
	if file == nil {
		return nil
	}
	playbackPath := file.AbsolutePath + assetPlaybackSuffix
	info, err := os.Stat(playbackPath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return file
	}
	optimized := *file
	optimized.AbsolutePath = playbackPath
	optimized.MimeType = "video/mp4"
	return &optimized
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
	if err := a.servePDFRange(w, r, groupID, id, file, pages); err != nil {
		writeError(w, http.StatusInternalServerError, "pdf_range_failed")
	}
}

func setAssetDownloadHeaders(w http.ResponseWriter, file *assetdomain.DownloadFile, info os.FileInfo) {
	mt := file.MimeType
	if mt == "" {
		mt = mime.TypeByExtension(filepath.Ext(file.OriginalName))
	}
	if mt == "" {
		mt = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mt)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(file.OriginalName)))
	w.Header().Set("Cache-Control", "private, max-age=604800, immutable")
	w.Header().Set("ETag", fmt.Sprintf(`"agp-%x-%x"`, info.Size(), info.ModTime().UTC().Unix()))
	w.Header().Set("Last-Modified", info.ModTime().UTC().Format(http.TimeFormat))
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

func (a *app) servePDFRange(
	w http.ResponseWriter,
	r *http.Request,
	groupID uint64,
	assetID uint64,
	file *assetdomain.DownloadFile,
	pages string,
) error {
	info, err := os.Stat(file.AbsolutePath)
	if err != nil {
		return err
	}
	key := pdfRangeCacheKey{
		groupID:       groupID,
		assetID:       assetID,
		pages:         pages,
		sourceSize:    info.Size(),
		sourceModUnix: info.ModTime().UnixNano(),
	}
	if a.pdfRangeCache != nil {
		if payload, ok := a.pdfRangeCache.Get(key); ok {
			w.Header().Set("X-AGP-PDF-Cache", "HIT")
			servePDFRangeBytes(w, r, file.OriginalName, pages, info.ModTime(), payload)
			return nil
		}
	}

	payload, err := trimPDFRange(file.AbsolutePath, pages)
	if err != nil {
		return err
	}
	if a.pdfRangeCache != nil {
		a.pdfRangeCache.Put(key, payload)
	}
	w.Header().Set("X-AGP-PDF-Cache", "MISS")
	servePDFRangeBytes(w, r, file.OriginalName, pages, info.ModTime(), payload)
	return nil
}

func trimPDFRange(srcPath, pages string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "agp-pdf-range-*.pdf")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return nil, err
	}
	defer os.Remove(tmpName)
	if err := pdfapi.TrimFile(srcPath, tmpName, []string{pages}, nil); err != nil {
		return nil, err
	}
	payload, err := os.ReadFile(tmpName)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func servePDFRangeBytes(
	w http.ResponseWriter,
	r *http.Request,
	original string,
	pages string,
	modTime time.Time,
	payload []byte,
) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filepath.Base(original)))
	w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	checksum := sha256.Sum256(payload)
	w.Header().Set("ETag", fmt.Sprintf(`"agp-pdf-%x-%s"`, checksum[:8], pages))
	http.ServeContent(w, r, original, modTime, bytes.NewReader(payload))
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
