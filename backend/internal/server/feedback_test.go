package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	feedbackdomain "agp/backend/internal/feedback"
	"agp/backend/internal/logctx"
)

type feedbackHandlerRepository struct {
	feedbackdomain.Repository
	item     feedbackdomain.Feedback
	settings feedbackdomain.AutomaticSettings
}

func (r *feedbackHandlerRepository) Create(
	_ context.Context,
	item feedbackdomain.Feedback,
	attachments []feedbackdomain.Attachment,
) (uint64, error) {
	item.ID = 73
	for index := range attachments {
		attachments[index].ID = uint64(index + 1)
		attachments[index].FeedbackID = item.ID
	}
	item.Attachments = attachments
	item.AttachmentCount = len(attachments)
	r.item = item
	return item.ID, nil
}

func (r *feedbackHandlerRepository) AutomaticSettings(context.Context) (feedbackdomain.AutomaticSettings, error) {
	return r.settings, nil
}

func (r *feedbackHandlerRepository) SaveAutomaticSettings(
	_ context.Context,
	settings feedbackdomain.AutomaticSettings,
	_ uint64,
	_ time.Time,
) error {
	r.settings = settings
	return nil
}

func (r *feedbackHandlerRepository) ListByUser(_ context.Context, userID uint64, _ int) ([]feedbackdomain.Feedback, error) {
	if r.item.UserID != userID {
		return nil, nil
	}
	return []feedbackdomain.Feedback{r.item}, nil
}

func (r *feedbackHandlerRepository) FindByUser(_ context.Context, userID, feedbackID uint64) (*feedbackdomain.Feedback, error) {
	if r.item.ID != feedbackID || r.item.UserID != userID {
		return nil, feedbackdomain.ErrNotFound
	}
	item := r.item
	return &item, nil
}

func (r *feedbackHandlerRepository) ListAll(context.Context, feedbackdomain.Status, int) ([]feedbackdomain.Feedback, error) {
	return []feedbackdomain.Feedback{r.item}, nil
}

func (r *feedbackHandlerRepository) FindByID(_ context.Context, feedbackID uint64) (*feedbackdomain.Feedback, error) {
	if r.item.ID != feedbackID {
		return nil, feedbackdomain.ErrNotFound
	}
	item := r.item
	return &item, nil
}

type feedbackHandlerStorage struct{}

func (*feedbackHandlerStorage) Save(_ context.Context, extension string, content []byte) (*feedbackdomain.StoredObject, error) {
	return &feedbackdomain.StoredObject{
		StoragePath: "feedback/objects/test" + extension,
		FileSize:    uint64(len(content)),
	}, nil
}

func (*feedbackHandlerStorage) Resolve(context.Context, string) (*feedbackdomain.ResolvedObject, error) {
	return nil, feedbackdomain.ErrNotFound
}

func (*feedbackHandlerStorage) Delete(context.Context, string) error {
	return nil
}

func TestCreateFeedbackKeepsDiagnosticsOutOfUserResponse(t *testing.T) {
	t.Parallel()

	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("message", "  页面无法打开  "); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("diagnostics", `{"page":"resources","user_agent":"ignored"}`); err != nil {
		t.Fatal(err)
	}
	part, err := form.CreateFormFile("images", "screen.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(imageData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	repo := &feedbackHandlerRepository{
		settings: feedbackdomain.AutomaticSettings{Enabled: true},
	}
	application := &app{
		feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{}),
		audits:    auditdomain.NewService(&serverAuditRepository{}),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/feedback", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("User-Agent", "trusted-browser")
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID: 11, CurrentGroupID: 7,
	}))
	request = request.WithContext(logctx.WithLogID(request.Context(), "0123456789abcdef0123456789abcdef"))
	recorder := httptest.NewRecorder()

	application.handleCreateFeedback(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if repo.item.UserID != 11 || repo.item.GroupID != 7 || repo.item.Message != "页面无法打开" {
		t.Fatalf("stored feedback = %#v", repo.item)
	}
	if repo.item.LogID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("stored log ID = %q", repo.item.LogID)
	}
	if !strings.Contains(repo.item.DiagnosticsJSON, "trusted-browser") ||
		strings.Contains(repo.item.DiagnosticsJSON, "ignored") {
		t.Fatalf("stored diagnostics = %s", repo.item.DiagnosticsJSON)
	}
	if strings.Contains(recorder.Body.String(), "diagnostics") ||
		strings.Contains(recorder.Body.String(), "resources") {
		t.Fatalf("user response exposed diagnostics: %s", recorder.Body)
	}
}

func TestOwnFeedbackDetailDoesNotExposeAnotherUser(t *testing.T) {
	t.Parallel()

	repo := &feedbackHandlerRepository{item: feedbackdomain.Feedback{
		ID: 73, UserID: 11, Message: "private", Status: feedbackdomain.StatusPending,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}}
	application := &app{feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{})}
	request := httptest.NewRequest(http.MethodGet, "/api/feedback/73", nil)
	request.SetPathValue("id", "73")
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 12}))
	recorder := httptest.NewRecorder()

	application.handleOwnFeedbackDetail(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestAutomaticFeedbackUsesFailingRequestLogID(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("message", "系统自动上报：资源打开发生错误"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("error_log_id", "fedcba9876543210fedcba9876543210"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("diagnostics", `{"request_method":"GET","request_path":"/api/assets/7/download","http_status":"404","error_code":"asset_not_found"}`); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	repo := &feedbackHandlerRepository{
		settings: feedbackdomain.AutomaticSettings{Enabled: true},
	}
	application := &app{
		feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{}),
		audits:    auditdomain.NewService(&serverAuditRepository{}),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/feedback/automatic", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("User-Agent", "trusted-browser")
	request = request.WithContext(context.WithValue(
		request.Context(),
		currentUserKey,
		currentUser{ID: 11, CurrentGroupID: 7},
	))
	request = request.WithContext(logctx.WithLogID(
		request.Context(),
		"0123456789abcdef0123456789abcdef",
	))
	recorder := httptest.NewRecorder()

	application.handleAutomaticFeedback(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if repo.item.Source != feedbackdomain.SourceAutomatic {
		t.Fatalf("source = %q", repo.item.Source)
	}
	if repo.item.LogID != "fedcba9876543210fedcba9876543210" {
		t.Fatalf("log ID = %q", repo.item.LogID)
	}
}

func TestAutomaticFeedbackSkipsMutedErrorType(t *testing.T) {
	t.Parallel()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("message", "系统自动上报：资源打开错误"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("diagnostics", `{"error_code":"asset_not_found"}`); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	repo := &feedbackHandlerRepository{settings: feedbackdomain.AutomaticSettings{
		Enabled:         true,
		MutedErrorTypes: []string{"asset_not_found"},
	}}
	application := &app{feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{})}
	request := httptest.NewRequest(http.MethodPost, "/api/feedback/automatic", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	request = request.WithContext(context.WithValue(
		request.Context(),
		currentUserKey,
		currentUser{ID: 11, CurrentGroupID: 7},
	))
	auditState := &requestAuditState{}
	request = request.WithContext(context.WithValue(request.Context(), requestAuditStateKey, auditState))
	recorder := httptest.NewRecorder()

	application.handleAutomaticFeedback(recorder, request)

	if recorder.Code != http.StatusOK || repo.item.ID != 0 {
		t.Fatalf("status = %d, item = %#v, body = %s", recorder.Code, repo.item, recorder.Body)
	}
	var response struct {
		Created bool   `json:"created"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Created || response.Reason != "muted" {
		t.Fatalf("response = %#v", response)
	}
	if !auditState.handled || auditState.recorded {
		t.Fatalf("audit state = %#v", auditState)
	}
}

func TestUpdateAutomaticFeedbackSettings(t *testing.T) {
	t.Parallel()

	repo := &feedbackHandlerRepository{
		settings: feedbackdomain.AutomaticSettings{Enabled: true},
	}
	application := &app{
		feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{}),
		audits:    auditdomain.NewService(&serverAuditRepository{}),
	}
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/super-admin/feedback/automatic-settings",
		strings.NewReader(`{"enabled":false,"muted_error_types":[" TypeError ","typeerror"]}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request = request.WithContext(context.WithValue(
		request.Context(),
		currentUserKey,
		currentUser{ID: 99},
	))
	recorder := httptest.NewRecorder()

	application.handleSuperUpdateAutomaticFeedbackSettings(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if repo.settings.Enabled || len(repo.settings.MutedErrorTypes) != 1 ||
		repo.settings.MutedErrorTypes[0] != "typeerror" {
		t.Fatalf("settings = %#v", repo.settings)
	}
}

func TestFeedbackAdminHandlerRequiresSuperAdmin(t *testing.T) {
	t.Parallel()

	called := false
	application := &app{}
	handler := application.requireSuper(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/feedback", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 12}))
	recorder := httptest.NewRecorder()

	handler(recorder, request)

	if recorder.Code != http.StatusForbidden || called {
		t.Fatalf("status = %d, called = %t", recorder.Code, called)
	}
}
