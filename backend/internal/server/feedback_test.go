package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	feedbackdomain "agp/backend/internal/feedback"
	"agp/backend/internal/logctx"
	userdomain "agp/backend/internal/user"
)

type feedbackHandlerRepository struct {
	feedbackdomain.Repository
	item     feedbackdomain.Feedback
	settings feedbackdomain.AutomaticSettings
	source   feedbackdomain.Source
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

func (r *feedbackHandlerRepository) ListByUser(
	_ context.Context,
	userID uint64,
	source feedbackdomain.Source,
	_ int,
) ([]feedbackdomain.Feedback, error) {
	r.source = source
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

func (r *feedbackHandlerRepository) ListAll(
	_ context.Context,
	_ feedbackdomain.Status,
	source feedbackdomain.Source,
	_ int,
) ([]feedbackdomain.Feedback, error) {
	r.source = source
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
	if err := form.WriteField("diagnostics", `{"page":"resources","user_agent":"ignored","page_origin":"https://cedar.example.test","environment":"production"}`); err != nil {
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
	var logOutput bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(logctx.NewHandler(slog.NewTextHandler(&logOutput, nil))))
	defer slog.SetDefault(previousLogger)

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("message", "系统自动上报：阅读资源时发生错误"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("error_log_id", "fedcba9876543210fedcba9876543210"); err != nil {
		t.Fatal(err)
	}
	if err := form.WriteField("diagnostics", `{"request_method":"GET","request_path":"/api/assets/7/download","http_status":"404","error_code":"asset_not_found","business_action":"阅读","resource_title":"马可福音","group_name":"伪造小组","user_display_name":"伪造用户","page_origin":"https://cedar.example.test","environment":"production"}`); err != nil {
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
		currentUser{
			ID: 11, Username: "member", DisplayName: "成员账号", MemberName: "小泽",
			CurrentGroupID: 7,
			Groups:         []userdomain.Group{{ID: 7, Name: "科大门训"}},
		},
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
	if repo.item.Message != "系统自动上报：科大门训的小泽在阅读《马可福音》时发生错误" {
		t.Fatalf("message = %q", repo.item.Message)
	}
	var diagnostics map[string]string
	if err := json.Unmarshal([]byte(repo.item.DiagnosticsJSON), &diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics["group_name"] != "科大门训" || diagnostics["user_display_name"] != "小泽" {
		t.Fatalf("diagnostics identity = %#v", diagnostics)
	}
	for _, expected := range []string{
		`msg="automatic feedback created"`,
		"feedback_id=73",
		"error_log_id=fedcba9876543210fedcba9876543210",
		"group_name=科大门训",
		"user_display_name=小泽",
		"business_action=阅读",
		"resource_title=马可福音",
	} {
		if !strings.Contains(logOutput.String(), expected) {
			t.Fatalf("log output %q does not contain %q", logOutput.String(), expected)
		}
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

func TestFeedbackListHandlersForwardAndValidateSource(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		admin  bool
		source string
		status int
		want   feedbackdomain.Source
	}{
		{name: "own manual", source: "manual", status: http.StatusOK, want: feedbackdomain.SourceManual},
		{name: "admin automatic", admin: true, source: "automatic", status: http.StatusOK, want: feedbackdomain.SourceAutomatic},
		{name: "own all", status: http.StatusOK},
		{name: "invalid", source: "unknown", status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &feedbackHandlerRepository{item: feedbackdomain.Feedback{
				ID: 73, UserID: 11, Source: test.want, Message: "反馈",
				Status: feedbackdomain.StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}}
			application := &app{feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{})}
			path := "/api/feedback?source=" + test.source
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request = request.WithContext(context.WithValue(
				request.Context(), currentUserKey, currentUser{ID: 11, IsSuperAdmin: test.admin},
			))
			recorder := httptest.NewRecorder()
			if test.admin {
				application.handleSuperListFeedback(recorder, request)
			} else {
				application.handleListOwnFeedback(recorder, request)
			}
			if recorder.Code != test.status {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.status, recorder.Body)
			}
			if test.status == http.StatusOK && repo.source != test.want {
				t.Fatalf("source=%q want=%q", repo.source, test.want)
			}
		})
	}
}

type feedbackUnreadHandlerRepository struct {
	feedbackHandlerRepository
	adminQueries int
}

func (r *feedbackUnreadHandlerRepository) UnreadCandidates(_ context.Context, _ uint64, admin bool, _ time.Time) ([]feedbackdomain.UnreadCandidate, error) {
	if admin {
		r.adminQueries++
		return []feedbackdomain.UnreadCandidate{{ID: 73}, {ID: 74}}, nil
	}
	return []feedbackdomain.UnreadCandidate{{ID: 73, LastReplyID: 9}, {ID: 74, LastReplyID: 10}}, nil
}

func TestFeedbackUnreadHandlersKeepReadsScopedAndDetailsReadOnly(t *testing.T) {
	repo := &feedbackUnreadHandlerRepository{feedbackHandlerRepository: feedbackHandlerRepository{item: feedbackdomain.Feedback{ID: 73, UserID: 11, Replies: []feedbackdomain.Reply{{ID: 9}}}}}
	reads, err := feedbackdomain.NewReadStateStore(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	application := &app{feedbacks: feedbackdomain.NewService(repo, &feedbackHandlerStorage{}, reads)}
	request := httptest.NewRequest(http.MethodGet, "/api/feedback/unread", nil)
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 11}))
	response := httptest.NewRecorder()
	application.handleFeedbackUnread(response, request)
	if response.Code != http.StatusOK || repo.adminQueries != 0 || !strings.Contains(response.Body.String(), `"own_ids":[73,74]`) || !strings.Contains(response.Body.String(), `"admin_ids":[]`) {
		t.Fatalf("unread=%d %s admin queries=%d", response.Code, response.Body.String(), repo.adminQueries)
	}
	request.SetPathValue("id", "73")
	response = httptest.NewRecorder()
	application.handleOwnFeedbackDetail(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail=%d %s", response.Code, response.Body.String())
	}
	unread, err := application.feedbacks.Unread(t.Context(), 11, false)
	if err != nil || len(unread.OwnIDs) != 2 {
		t.Fatalf("GET marked read=%+v err=%v", unread, err)
	}
	for _, tc := range []struct {
		name       string
		user       uint64
		adminRoute bool
		want       int
	}{
		{"foreign owner", 12, false, http.StatusNotFound},
		{"ordinary user admin route", 11, true, http.StatusForbidden},
		{"owner reads one", 11, false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/feedback/73/read", strings.NewReader(`{"reply_id":9,"admin":true}`))
			r.SetPathValue("id", "73")
			r = r.WithContext(context.WithValue(r.Context(), currentUserKey, currentUser{ID: tc.user}))
			w := httptest.NewRecorder()
			if tc.adminRoute {
				application.requireSuper(application.handleSuperFeedbackRead)(w, r)
			} else {
				application.handleOwnFeedbackRead(w, r)
			}
			if w.Code != tc.want {
				t.Fatalf("read=%d %s", w.Code, w.Body.String())
			}
		})
	}
	unread, err = application.feedbacks.Unread(t.Context(), 11, false)
	if err != nil || len(unread.OwnIDs) != 1 || unread.OwnIDs[0] != 74 {
		t.Fatalf("per-item read=%+v err=%v", unread, err)
	}
}
