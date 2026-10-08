package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	feedbackdomain "agp/backend/internal/feedback"
	"agp/backend/internal/logctx"
)

const feedbackMultipartMemory = int64(8 << 20)

func automaticFeedbackIdentity(user currentUser) (string, string) {
	displayName := strings.TrimSpace(user.MemberName)
	if displayName == "" {
		displayName = strings.TrimSpace(user.DisplayName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}
	for _, group := range user.Groups {
		if group.ID == user.CurrentGroupID {
			return displayName, strings.TrimSpace(group.Name)
		}
	}
	return displayName, ""
}

func readableAutomaticFeedbackMessage(diagnostics map[string]string, fallback string) string {
	action := strings.TrimSpace(diagnostics["business_action"])
	if action == "" {
		return fallback
	}
	content := ""
	if title := strings.TrimSpace(diagnostics["resource_title"]); title != "" {
		content = "《" + title + "》"
	} else if title := strings.TrimSpace(diagnostics["task_title"]); title != "" {
		content = "“" + title + "”"
	}
	groupName := strings.TrimSpace(diagnostics["group_name"])
	userDisplayName := strings.TrimSpace(diagnostics["user_display_name"])
	subject := userDisplayName
	if groupName != "" && userDisplayName != "" {
		subject = groupName + "的" + userDisplayName
	} else if groupName != "" {
		subject = groupName
	}
	if subject != "" {
		subject += "在"
	}
	return "系统自动上报：" + subject + action + content + "时发生错误"
}

func (a *app) handleCreateFeedback(w http.ResponseWriter, r *http.Request) {
	a.handleFeedbackCreate(w, r, feedbackdomain.SourceManual)
}

func (a *app) handleAutomaticFeedback(w http.ResponseWriter, r *http.Request) {
	a.handleFeedbackCreate(w, r, feedbackdomain.SourceAutomatic)
}

func (a *app) handleAutomaticFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.feedbacks.AutomaticSettings(r.Context())
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *app) handleSuperUpdateAutomaticFeedbackSettings(w http.ResponseWriter, r *http.Request) {
	var input feedbackdomain.AutomaticSettings
	if !readJSON(w, r, &input) {
		return
	}
	before, err := a.feedbacks.AutomaticSettings(r.Context())
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	user := mustUser(r)
	settings, err := a.feedbacks.UpdateAutomaticSettings(r.Context(), input, user.ID, time.Now())
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	a.audit(user.CurrentGroupID, user.ID, "update_automatic_feedback_settings", "feedback_settings", 1, before, settings, r)
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (a *app) handleFeedbackCreate(w http.ResponseWriter, r *http.Request, source feedbackdomain.Source) {
	user := mustUser(r)
	r.Body = http.MaxBytesReader(w, r.Body, feedbackdomain.MaxRequestBytes)
	if err := r.ParseMultipartForm(feedbackMultipartMemory); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "feedback_request_too_large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid_feedback_form")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()

	diagnostics := map[string]string(nil)
	if raw := strings.TrimSpace(r.FormValue("diagnostics")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &diagnostics); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_feedback_diagnostics")
			return
		}
	}
	if diagnostics == nil {
		diagnostics = make(map[string]string)
	}
	diagnostics["user_agent"] = r.UserAgent()
	if source == feedbackdomain.SourceAutomatic {
		userDisplayName, groupName := automaticFeedbackIdentity(user)
		diagnostics["user_display_name"] = userDisplayName
		diagnostics["group_name"] = groupName
	}
	files := r.MultipartForm.File["images"]
	if len(files) > feedbackdomain.MaxImages {
		writeError(w, http.StatusRequestEntityTooLarge, feedbackdomain.ErrTooManyImages.Error())
		return
	}
	uploads := make([]feedbackdomain.ImageUpload, 0, len(files))
	opened := make([]interface{ Close() error }, 0, len(files))
	defer func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}()
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_feedback_image")
			return
		}
		opened = append(opened, file)
		uploads = append(uploads, feedbackdomain.ImageUpload{
			FileName: header.Filename,
			Reader:   file,
		})
	}

	logID := logctx.LogID(r.Context())
	if source == feedbackdomain.SourceAutomatic {
		if errorLogID := strings.TrimSpace(r.FormValue("error_log_id")); logctx.Valid(errorLogID) {
			logID = errorLogID
		}
	}
	input := feedbackdomain.CreateInput{
		UserID:      user.ID,
		GroupID:     user.CurrentGroupID,
		Message:     r.FormValue("message"),
		Source:      source,
		LogID:       logID,
		Diagnostics: diagnostics,
		Images:      uploads,
	}
	if source == feedbackdomain.SourceAutomatic {
		input.Message = readableAutomaticFeedbackMessage(diagnostics, input.Message)
	}
	var item *feedbackdomain.UserView
	var err error
	if source == feedbackdomain.SourceAutomatic {
		result, createErr := a.feedbacks.CreateAutomatic(r.Context(), input, time.Now())
		if createErr != nil {
			writeFeedbackError(w, r, createErr)
			return
		}
		if result.Feedback == nil {
			markAuditHandled(r)
			writeJSON(w, http.StatusOK, map[string]any{
				"created": false,
				"reason":  result.Reason,
			})
			return
		}
		item = result.Feedback
		slog.InfoContext(r.Context(), "automatic feedback created",
			"feedback_id", item.ID,
			"error_log_id", input.LogID,
			"group_id", user.CurrentGroupID,
			"group_name", diagnostics["group_name"],
			"user_id", user.ID,
			"user_display_name", diagnostics["user_display_name"],
			"business_action", diagnostics["business_action"],
			"resource_title", diagnostics["resource_title"],
			"task_title", diagnostics["task_title"],
			"error_name", diagnostics["error_name"],
			"error_code", diagnostics["error_code"],
		)
	} else {
		item, err = a.feedbacks.Create(r.Context(), input, time.Now())
	}
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	a.audit(user.CurrentGroupID, user.ID, "create_feedback", "feedbacks", item.ID, nil, map[string]any{
		"status":           item.Status,
		"source":           item.Source,
		"attachment_count": item.AttachmentCount,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"created": true, "feedback": item})
}

func (a *app) handleListOwnFeedback(w http.ResponseWriter, r *http.Request) {
	source := feedbackdomain.Source(strings.TrimSpace(r.URL.Query().Get("source")))
	items, err := a.feedbacks.ListOwn(r.Context(), mustUser(r).ID, source, 100)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *app) handleOwnFeedbackDetail(w http.ResponseWriter, r *http.Request) {
	id, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return
	}
	item, err := a.feedbacks.OwnDetail(r.Context(), mustUser(r).ID, id)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": item})
}

func (a *app) handleOwnFeedbackAttachment(w http.ResponseWriter, r *http.Request) {
	feedbackID, attachmentID, ok := feedbackAttachmentPathIDs(w, r)
	if !ok {
		return
	}
	attachment, file, err := a.feedbacks.OwnAttachment(
		r.Context(), mustUser(r).ID, feedbackID, attachmentID,
	)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	serveFeedbackAttachment(w, r, attachment, file)
}

func (a *app) handleSuperListFeedback(w http.ResponseWriter, r *http.Request) {
	status := feedbackdomain.Status(strings.TrimSpace(r.URL.Query().Get("status")))
	source := feedbackdomain.Source(strings.TrimSpace(r.URL.Query().Get("source")))
	items, err := a.feedbacks.AdminList(r.Context(), status, source, 200)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *app) handleSuperFeedbackDetail(w http.ResponseWriter, r *http.Request) {
	id, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return
	}
	item, err := a.feedbacks.AdminDetail(r.Context(), id)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": item})
}

func (a *app) handleSuperFeedbackAttachment(w http.ResponseWriter, r *http.Request) {
	feedbackID, attachmentID, ok := feedbackAttachmentPathIDs(w, r)
	if !ok {
		return
	}
	attachment, file, err := a.feedbacks.AdminAttachment(r.Context(), feedbackID, attachmentID)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	serveFeedbackAttachment(w, r, attachment, file)
}

func (a *app) handleSuperUpdateFeedbackStatus(w http.ResponseWriter, r *http.Request) {
	id, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return
	}
	var input struct {
		Status feedbackdomain.Status `json:"status"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if err := a.feedbacks.UpdateStatus(r.Context(), id, input.Status, time.Now()); err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	user := mustUser(r)
	a.audit(user.CurrentGroupID, user.ID, "update_feedback_status", "feedbacks", id, nil, map[string]any{
		"status": input.Status,
	}, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleSuperReplyFeedback(w http.ResponseWriter, r *http.Request) {
	id, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return
	}
	var input struct {
		Message string `json:"message"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	user := mustUser(r)
	reply, err := a.feedbacks.Reply(r.Context(), id, user.ID, input.Message, time.Now())
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	a.audit(user.CurrentGroupID, user.ID, "reply_feedback", "feedbacks", id, nil, map[string]any{
		"reply_id": reply.ID,
	}, r)
	writeJSON(w, http.StatusCreated, map[string]any{"reply": reply})
}

func feedbackPathID(r *http.Request) (uint64, error) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, errors.New("invalid_feedback_id")
	}
	return id, nil
}

func feedbackAttachmentPathIDs(w http.ResponseWriter, r *http.Request) (uint64, uint64, bool) {
	feedbackID, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return 0, 0, false
	}
	attachmentID, err := strconv.ParseUint(r.PathValue("attachment_id"), 10, 64)
	if err != nil || attachmentID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_feedback_attachment_id")
		return 0, 0, false
	}
	return feedbackID, attachmentID, true
}

func serveFeedbackAttachment(
	w http.ResponseWriter,
	r *http.Request,
	attachment *feedbackdomain.Attachment,
	file *feedbackdomain.ResolvedObject,
) {
	handle, err := os.Open(file.AbsolutePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "feedback_attachment_not_found")
		return
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "feedback_attachment_not_found")
		return
	}
	w.Header().Set("Content-Type", attachment.MimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
		"filename": attachment.OriginalName,
	}))
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, attachment.OriginalName, info.ModTime(), handle)
}

func writeFeedbackError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, feedbackdomain.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, feedbackdomain.ErrImageTooLarge),
		errors.Is(err, feedbackdomain.ErrTooManyImages):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, feedbackdomain.ErrMessageRequired),
		errors.Is(err, feedbackdomain.ErrMessageTooLong),
		errors.Is(err, feedbackdomain.ErrReplyRequired),
		errors.Is(err, feedbackdomain.ErrReplyTooLong),
		errors.Is(err, feedbackdomain.ErrInvalidStatus),
		errors.Is(err, feedbackdomain.ErrInvalidSource),
		errors.Is(err, feedbackdomain.ErrInvalidImage),
		errors.Is(err, feedbackdomain.ErrInvalidDiagnostics),
		errors.Is(err, feedbackdomain.ErrInvalidAutoSettings):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		slog.ErrorContext(r.Context(), "feedback request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "feedback_failed")
	}
}

func (a *app) handleFeedbackUnread(w http.ResponseWriter, r *http.Request) {
	user := mustUser(r)
	unread, err := a.feedbacks.Unread(r.Context(), user.ID, user.IsSuperAdmin)
	if err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, unread)
}

func (a *app) handleOwnFeedbackRead(w http.ResponseWriter, r *http.Request) {
	a.handleFeedbackRead(w, r, false)
}

func (a *app) handleSuperFeedbackRead(w http.ResponseWriter, r *http.Request) {
	a.handleFeedbackRead(w, r, true)
}

func (a *app) handleFeedbackRead(w http.ResponseWriter, r *http.Request, admin bool) {
	id, err := feedbackPathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_feedback_id")
		return
	}
	var input struct {
		ReplyID uint64 `json:"reply_id"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if err := a.feedbacks.MarkRead(r.Context(), mustUser(r).ID, id, admin, input.ReplyID); err != nil {
		writeFeedbackError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
