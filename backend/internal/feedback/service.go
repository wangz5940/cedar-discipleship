package feedback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"agp/backend/internal/logctx"
)

var (
	ErrNotFound            = errors.New("feedback_not_found")
	ErrMessageRequired     = errors.New("feedback_message_required")
	ErrMessageTooLong      = errors.New("feedback_message_too_long")
	ErrReplyRequired       = errors.New("feedback_reply_required")
	ErrReplyTooLong        = errors.New("feedback_reply_too_long")
	ErrInvalidStatus       = errors.New("invalid_feedback_status")
	ErrInvalidSource       = errors.New("invalid_feedback_source")
	ErrTooManyImages       = errors.New("too_many_feedback_images")
	ErrImageTooLarge       = errors.New("feedback_image_too_large")
	ErrInvalidImage        = errors.New("invalid_feedback_image")
	ErrInvalidDiagnostics  = errors.New("invalid_feedback_diagnostics")
	ErrInvalidAutoSettings = errors.New("invalid_automatic_feedback_settings")
)

const (
	maxMessageRunes           = 5000
	maxReplyRunes             = 2000
	maxImagePixels      int64 = 20_000_000
	maxStoredImageBytes       = 10 << 20
	maxDiagnosticsBytes       = 8 << 10
	maxMutedErrorTypes        = 200
)

var diagnosticLimits = map[string]int{
	"app_version":                128,
	"page":                       256,
	"page_origin":                512,
	"environment":                32,
	"action_context":             128,
	"recent_log_id":              32,
	"error_log_id_source":        32,
	"request_network_online":     8,
	"request_visibility_end":     32,
	"request_network_online_end": 8,
	"transport_reason":           64,
	"user_agent":                 1024,
	"language":                   64,
	"platform":                   128,
	"viewport":                   64,
	"screen":                     64,
	"client_time":                64,
	"network_online":             8,
	"visibility_state":           32,
	"page_title":                 128,
	"script_sources":             1024,
	"error_name":                 128,
	"error_message":              512,
	"error_stack":                4096,
	"error_stack_source":         32,
	"request_method":             16,
	"request_path":               512,
	"request_started_at":         64,
	"request_duration_ms":        32,
	"request_visibility":         32,
	"response_received":          8,
	"response_log_id":            32,
	"response_content_type":      128,
	"http_status":                3,
	"error_code":                 128,
	"business_action":            64,
	"resource_title":             256,
	"task_title":                 256,
	"logical_date":               32,
	"script_url":                 512,
	"line":                       16,
	"column":                     16,
	"event_target":               128,
	"group_name":                 128,
	"user_display_name":          128,
}

type Service struct {
	repo    Repository
	storage Storage
	reads   *ReadStateStore
}

func NewService(repo Repository, storage Storage, reads ...*ReadStateStore) *Service {
	s := &Service{repo: repo, storage: storage}
	if len(reads) > 0 {
		s.reads = reads[0]
	}
	return s
}

func (s *Service) Create(ctx context.Context, input CreateInput, now time.Time) (*UserView, error) {
	if input.Source != "" && input.Source != SourceManual {
		return nil, ErrInvalidSource
	}
	input.Source = SourceManual
	return s.create(ctx, input, now, false)
}

func (s *Service) CreateAutomatic(
	ctx context.Context,
	input CreateInput,
	now time.Time,
) (*AutomaticCreateResult, error) {
	return s.createAutomatic(ctx, input, now, false)
}

// CreateSystemAutomatic is for trusted background observers, never HTTP callers.
// System feedback has no user or uploaded attachments.
func (s *Service) CreateSystemAutomatic(
	ctx context.Context, input CreateInput, now time.Time,
) (*AutomaticCreateResult, error) {
	input.UserID = 0
	input.Images = nil
	return s.createAutomatic(ctx, input, now, true)
}

func (s *Service) createAutomatic(
	ctx context.Context, input CreateInput, now time.Time, system bool,
) (*AutomaticCreateResult, error) {
	settings, err := s.repo.AutomaticSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return &AutomaticCreateResult{Reason: "disabled"}, nil
	}
	errorType := automaticErrorType(input.Diagnostics)
	for _, muted := range settings.MutedErrorTypes {
		if errorType != "" && normalizeErrorType(muted) == errorType {
			return &AutomaticCreateResult{Reason: "muted"}, nil
		}
	}
	input.Source = SourceAutomatic
	item, err := s.create(ctx, input, now, system)
	if err != nil {
		return nil, err
	}
	return &AutomaticCreateResult{Feedback: item}, nil
}

func (s *Service) AutomaticSettings(ctx context.Context) (AutomaticSettings, error) {
	return s.repo.AutomaticSettings(ctx)
}

func (s *Service) UpdateAutomaticSettings(
	ctx context.Context,
	settings AutomaticSettings,
	adminUserID uint64,
	now time.Time,
) (AutomaticSettings, error) {
	normalized, err := normalizeAutomaticSettings(settings)
	if err != nil {
		return AutomaticSettings{}, err
	}
	if err := s.repo.SaveAutomaticSettings(ctx, normalized, adminUserID, now.UTC()); err != nil {
		return AutomaticSettings{}, err
	}
	return normalized, nil
}

func (s *Service) create(ctx context.Context, input CreateInput, now time.Time, system bool) (*UserView, error) {
	message := strings.TrimSpace(input.Message)
	switch {
	case message == "":
		return nil, ErrMessageRequired
	case utf8.RuneCountInString(message) > maxMessageRunes:
		return nil, ErrMessageTooLong
	case input.UserID == 0 && !system:
		return nil, ErrNotFound
	case len(input.Images) > MaxImages:
		return nil, ErrTooManyImages
	}
	if input.Source != SourceManual && input.Source != SourceAutomatic {
		return nil, ErrInvalidSource
	}

	diagnosticsJSON, err := normalizeDiagnostics(input.Diagnostics)
	if err != nil {
		return nil, err
	}
	if !logctx.Valid(input.LogID) {
		input.LogID = ""
	}

	attachments := make([]Attachment, 0, len(input.Images))
	storedPaths := make([]string, 0, len(input.Images))
	cleanup := func() error {
		var errs []error
		for _, storagePath := range storedPaths {
			if err := s.storage.Delete(context.WithoutCancel(ctx), storagePath); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	for _, upload := range input.Images {
		sanitized, err := sanitizeImage(upload)
		if err != nil {
			_ = cleanup()
			return nil, err
		}
		stored, err := s.storage.Save(ctx, sanitized.extension, sanitized.content)
		if err != nil {
			_ = cleanup()
			return nil, err
		}
		storedPaths = append(storedPaths, stored.StoragePath)
		attachments = append(attachments, Attachment{
			OriginalName: sanitized.originalName,
			StoragePath:  stored.StoragePath,
			MimeType:     sanitized.mimeType,
			FileSize:     stored.FileSize,
			Width:        sanitized.width,
			Height:       sanitized.height,
			CreatedAt:    now.UTC(),
		})
	}

	item := Feedback{
		GroupID:         input.GroupID,
		UserID:          input.UserID,
		Message:         message,
		Source:          input.Source,
		Status:          StatusPending,
		LogID:           input.LogID,
		DiagnosticsJSON: diagnosticsJSON,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}
	if system {
		item.LegacyName = "系统"
	}
	id, err := s.repo.Create(ctx, item, attachments)
	if err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return nil, errors.Join(err, fmt.Errorf("remove feedback uploads: %w", cleanupErr))
		}
		return nil, err
	}
	if persisted, findErr := s.repo.FindByUser(ctx, input.UserID, id); findErr == nil {
		view := toUserView(*persisted)
		return &view, nil
	}
	item.ID = id
	for index := range attachments {
		attachments[index].FeedbackID = id
	}
	item.Attachments = attachments
	item.AttachmentCount = len(attachments)
	view := toUserView(item)
	return &view, nil
}

func normalizeAutomaticSettings(settings AutomaticSettings) (AutomaticSettings, error) {
	if len(settings.MutedErrorTypes) > maxMutedErrorTypes {
		return AutomaticSettings{}, ErrInvalidAutoSettings
	}
	normalized := AutomaticSettings{
		Enabled:         settings.Enabled,
		MutedErrorTypes: make([]string, 0, len(settings.MutedErrorTypes)),
	}
	seen := make(map[string]struct{}, len(settings.MutedErrorTypes))
	for _, value := range settings.MutedErrorTypes {
		errorType := normalizeErrorType(value)
		if errorType == "" || utf8.RuneCountInString(errorType) > diagnosticLimits["error_code"] {
			return AutomaticSettings{}, ErrInvalidAutoSettings
		}
		if _, exists := seen[errorType]; exists {
			continue
		}
		seen[errorType] = struct{}{}
		normalized.MutedErrorTypes = append(normalized.MutedErrorTypes, errorType)
	}
	return normalized, nil
}

func automaticErrorType(diagnostics map[string]string) string {
	if errorType := normalizeErrorType(diagnostics["error_code"]); errorType != "" {
		return errorType
	}
	return normalizeErrorType(diagnostics["error_name"])
}

func normalizeErrorType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s *Service) ListOwn(ctx context.Context, userID uint64, source Source, limit int) ([]UserView, error) {
	if !validSourceFilter(source) {
		return nil, ErrInvalidSource
	}
	items, err := s.repo.ListByUser(ctx, userID, source, normalizedLimit(limit))
	if err != nil {
		return nil, err
	}
	result := make([]UserView, 0, len(items))
	for _, item := range items {
		result = append(result, toUserView(item))
	}
	return result, nil
}

func (s *Service) OwnDetail(ctx context.Context, userID, feedbackID uint64) (*UserView, error) {
	item, err := s.repo.FindByUser(ctx, userID, feedbackID)
	if err != nil {
		return nil, err
	}
	view := toUserView(*item)
	return &view, nil
}

func (s *Service) AdminList(ctx context.Context, status Status, source Source, limit int) ([]AdminView, error) {
	if status != "" && !validStatus(status) {
		return nil, ErrInvalidStatus
	}
	if !validSourceFilter(source) {
		return nil, ErrInvalidSource
	}
	items, err := s.repo.ListAll(ctx, status, source, normalizedLimit(limit))
	if err != nil {
		return nil, err
	}
	result := make([]AdminView, 0, len(items))
	for _, item := range items {
		view := toAdminView(item)
		view.Diagnostics = nil
		result = append(result, view)
	}
	return result, nil
}

func (s *Service) AdminDetail(ctx context.Context, feedbackID uint64) (*AdminView, error) {
	item, err := s.repo.FindByID(ctx, feedbackID)
	if err != nil {
		return nil, err
	}
	view := toAdminView(*item)
	return &view, nil
}

func (s *Service) UpdateStatus(ctx context.Context, feedbackID uint64, status Status, now time.Time) error {
	if !validStatus(status) {
		return ErrInvalidStatus
	}
	updated, err := s.repo.UpdateStatus(ctx, feedbackID, status, now.UTC())
	if err != nil {
		return err
	}
	if !updated {
		return ErrNotFound
	}
	return nil
}

func (s *Service) Reply(ctx context.Context, feedbackID, adminUserID uint64, message string, now time.Time) (*ReplyView, error) {
	message = strings.TrimSpace(message)
	switch {
	case message == "":
		return nil, ErrReplyRequired
	case utf8.RuneCountInString(message) > maxReplyRunes:
		return nil, ErrReplyTooLong
	}
	reply := Reply{
		FeedbackID:  feedbackID,
		AdminUserID: adminUserID,
		Message:     message,
		CreatedAt:   now.UTC(),
	}
	id, err := s.repo.CreateReply(ctx, reply)
	if err != nil {
		return nil, err
	}
	reply.ID = id
	view := toReplyView(reply)
	return &view, nil
}

func (s *Service) OwnAttachment(ctx context.Context, userID, feedbackID, attachmentID uint64) (*Attachment, *ResolvedObject, error) {
	if _, err := s.repo.FindByUser(ctx, userID, feedbackID); err != nil {
		return nil, nil, err
	}
	return s.resolveAttachment(ctx, feedbackID, attachmentID)
}

func (s *Service) AdminAttachment(ctx context.Context, feedbackID, attachmentID uint64) (*Attachment, *ResolvedObject, error) {
	if _, err := s.repo.FindByID(ctx, feedbackID); err != nil {
		return nil, nil, err
	}
	return s.resolveAttachment(ctx, feedbackID, attachmentID)
}

func (s *Service) resolveAttachment(ctx context.Context, feedbackID, attachmentID uint64) (*Attachment, *ResolvedObject, error) {
	attachment, err := s.repo.FindAttachment(ctx, feedbackID, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	resolved, err := s.storage.Resolve(ctx, attachment.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return attachment, resolved, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusPending, StatusProcessing, StatusNeedsInfo, StatusResolved, StatusClosed:
		return true
	default:
		return false
	}
}

func validSourceFilter(source Source) bool {
	return source == "" || source == SourceManual || source == SourceAutomatic
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func normalizeDiagnostics(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		limit, ok := diagnosticLimits[key]
		if !ok {
			return "", ErrInvalidDiagnostics
		}
		value = strings.TrimSpace(value)
		if utf8.RuneCountInString(value) > limit {
			return "", ErrInvalidDiagnostics
		}
		if value != "" {
			normalized[key] = value
		}
	}
	payload, err := json.Marshal(normalized)
	if err != nil || len(payload) > maxDiagnosticsBytes {
		return "", ErrInvalidDiagnostics
	}
	return string(payload), nil
}

type sanitizedImage struct {
	originalName string
	extension    string
	mimeType     string
	content      []byte
	width        int
	height       int
}

func sanitizeImage(upload ImageUpload) (*sanitizedImage, error) {
	if upload.Reader == nil {
		return nil, ErrInvalidImage
	}
	data, err := io.ReadAll(io.LimitReader(upload.Reader, MaxImageBytes+1))
	if err != nil {
		return nil, ErrInvalidImage
	}
	if int64(len(data)) > MaxImageBytes {
		return nil, ErrImageTooLarge
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxImagePixels {
		return nil, ErrInvalidImage
	}
	if format != "jpeg" && format != "png" {
		return nil, ErrInvalidImage
	}
	decoded, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format {
		return nil, ErrInvalidImage
	}

	output := &limitedBuffer{limit: maxStoredImageBytes}
	extension, mimeType := ".jpg", "image/jpeg"
	if format == "png" {
		extension, mimeType = ".png", "image/png"
		err = png.Encode(output, decoded)
	} else {
		err = jpeg.Encode(output, decoded, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		if errors.Is(err, ErrImageTooLarge) {
			return nil, err
		}
		return nil, ErrInvalidImage
	}
	return &sanitizedImage{
		originalName: safeOriginalName(upload.FileName, extension),
		extension:    extension,
		mimeType:     mimeType,
		content:      output.Bytes(),
		width:        config.Width,
		height:       config.Height,
	}, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (w *limitedBuffer) Write(data []byte) (int, error) {
	if w.Len()+len(data) > w.limit {
		return 0, ErrImageTooLarge
	}
	return w.Buffer.Write(data)
}

func safeOriginalName(value, extension string) string {
	value = path.Base(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"))
	value = strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return -1
		}
		return char
	}, value)
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	if value == "" || value == "." {
		return "image" + extension
	}
	base := strings.TrimSuffix(value, path.Ext(value))
	if strings.TrimSpace(base) == "" {
		base = "image"
	}
	return base + extension
}

func toUserView(item Feedback) UserView {
	attachments := make([]AttachmentView, 0, len(item.Attachments))
	for _, attachment := range item.Attachments {
		attachments = append(attachments, AttachmentView{
			ID:           attachment.ID,
			OriginalName: attachment.OriginalName,
			MimeType:     attachment.MimeType,
			FileSize:     attachment.FileSize,
			Width:        attachment.Width,
			Height:       attachment.Height,
		})
	}
	replies := make([]ReplyView, 0, len(item.Replies))
	for _, reply := range item.Replies {
		replies = append(replies, toReplyView(reply))
	}
	return UserView{
		ID:              item.ID,
		Message:         item.Message,
		Source:          item.Source,
		Status:          item.Status,
		AttachmentCount: item.AttachmentCount,
		ReplyCount:      item.ReplyCount,
		CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		Attachments:     attachments,
		Replies:         replies,
	}
}

func toAdminView(item Feedback) AdminView {
	diagnostics := map[string]string(nil)
	if item.DiagnosticsJSON != "" {
		_ = json.Unmarshal([]byte(item.DiagnosticsJSON), &diagnostics)
	}
	return AdminView{
		UserView:      toUserView(item),
		GroupID:       item.GroupID,
		UserID:        item.UserID,
		Username:      item.Username,
		DisplayName:   item.DisplayName,
		MemberName:    item.MemberName,
		GroupName:     item.GroupName,
		LegacyName:    item.LegacyName,
		LegacyContact: item.LegacyContact,
		LogID:         item.LogID,
		Diagnostics:   diagnostics,
	}
}

func toReplyView(item Reply) ReplyView {
	return ReplyView{
		ID:               item.ID,
		AdminDisplayName: item.AdminDisplayName,
		Message:          item.Message,
		CreatedAt:        item.CreatedAt.Format(time.RFC3339),
	}
}
