package feedback

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"
)

type memoryRepository struct {
	item        Feedback
	attachments []Attachment
	status      Status
	reply       Reply
	settings    AutomaticSettings
}

func (r *memoryRepository) Create(_ context.Context, item Feedback, attachments []Attachment) (uint64, error) {
	item.ID = 41
	for index := range attachments {
		attachments[index].ID = uint64(index + 1)
		attachments[index].FeedbackID = item.ID
	}
	item.Attachments = append([]Attachment(nil), attachments...)
	item.AttachmentCount = len(attachments)
	r.item = item
	r.attachments = append([]Attachment(nil), attachments...)
	return item.ID, nil
}

func (r *memoryRepository) AutomaticSettings(context.Context) (AutomaticSettings, error) {
	return r.settings, nil
}

func (r *memoryRepository) SaveAutomaticSettings(
	_ context.Context,
	settings AutomaticSettings,
	_ uint64,
	_ time.Time,
) error {
	r.settings = settings
	return nil
}

func (r *memoryRepository) ListByUser(_ context.Context, userID uint64, _ int) ([]Feedback, error) {
	if r.item.UserID != userID {
		return nil, nil
	}
	return []Feedback{r.item}, nil
}

func (r *memoryRepository) FindByUser(_ context.Context, userID, feedbackID uint64) (*Feedback, error) {
	if r.item.UserID != userID || r.item.ID != feedbackID {
		return nil, ErrNotFound
	}
	item := r.item
	return &item, nil
}

func (r *memoryRepository) ListAll(context.Context, Status, int) ([]Feedback, error) {
	return []Feedback{r.item}, nil
}

func (r *memoryRepository) FindByID(_ context.Context, feedbackID uint64) (*Feedback, error) {
	if r.item.ID != feedbackID {
		return nil, ErrNotFound
	}
	item := r.item
	return &item, nil
}

func (r *memoryRepository) UpdateStatus(_ context.Context, feedbackID uint64, status Status, updatedAt time.Time) (bool, error) {
	if r.item.ID != feedbackID {
		return false, nil
	}
	r.item.Status = status
	r.item.UpdatedAt = updatedAt
	if status == StatusClosed {
		r.item.DiagnosticsJSON = ""
	}
	r.status = status
	return true, nil
}

func (r *memoryRepository) CreateReply(_ context.Context, reply Reply) (uint64, error) {
	if r.item.ID != reply.FeedbackID {
		return 0, ErrNotFound
	}
	reply.ID = 7
	r.reply = reply
	r.item.Replies = append(r.item.Replies, reply)
	r.item.ReplyCount++
	return reply.ID, nil
}

func (r *memoryRepository) FindAttachment(_ context.Context, feedbackID, attachmentID uint64) (*Attachment, error) {
	for _, item := range r.attachments {
		if item.FeedbackID == feedbackID && item.ID == attachmentID {
			copy := item
			return &copy, nil
		}
	}
	return nil, ErrNotFound
}

type memoryStorage struct {
	content []byte
	deleted []string
}

func (s *memoryStorage) Save(_ context.Context, extension string, content []byte) (*StoredObject, error) {
	s.content = append([]byte(nil), content...)
	return &StoredObject{StoragePath: "objects/image" + extension, FileSize: uint64(len(content))}, nil
}

func (s *memoryStorage) Resolve(context.Context, string) (*ResolvedObject, error) {
	return &ResolvedObject{AbsolutePath: "/tmp/image.png"}, nil
}

func (s *memoryStorage) Delete(_ context.Context, path string) error {
	s.deleted = append(s.deleted, path)
	return nil
}

func TestCreateValidatesContentAndDiagnostics(t *testing.T) {
	t.Parallel()

	service := NewService(&memoryRepository{}, &memoryStorage{})
	for _, test := range []struct {
		name  string
		input CreateInput
		want  error
	}{
		{name: "blank message", input: CreateInput{UserID: 1, Message: " \n "}, want: ErrMessageRequired},
		{name: "unknown diagnostic", input: CreateInput{
			UserID: 1, Message: "建议",
			Diagnostics: map[string]string{"cookie": "secret"},
		}, want: ErrInvalidDiagnostics},
		{name: "invalid source", input: CreateInput{
			UserID: 1, Message: "建议", Source: Source("imported"),
		}, want: ErrInvalidSource},
		{name: "too many images", input: CreateInput{
			UserID: 1, Message: "建议", Images: make([]ImageUpload, MaxImages+1),
		}, want: ErrTooManyImages},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := service.Create(t.Context(), test.input, time.Now())
			if !errors.Is(err, test.want) {
				t.Fatalf("Create() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestCreateSanitizesImageAndSeparatesViews(t *testing.T) {
	t.Parallel()

	var source bytes.Buffer
	picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
	picture.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&source, picture); err != nil {
		t.Fatal(err)
	}
	source.WriteString("private-image-metadata")

	repo := &memoryRepository{}
	storage := &memoryStorage{}
	service := NewService(repo, storage)
	created, err := service.Create(t.Context(), CreateInput{
		UserID:  9,
		GroupID: 3,
		Message: "  请增加历史筛选  ",
		LogID:   "0123456789abcdef0123456789abcdef",
		Diagnostics: map[string]string{
			"page":          "dashboard",
			"recent_log_id": "fedcba9876543210fedcba9876543210",
		},
		Images: []ImageUpload{{FileName: "../screen.png", Reader: bytes.NewReader(source.Bytes())}},
	}, time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Message != "请增加历史筛选" ||
		created.Source != SourceManual ||
		created.Status != StatusPending {
		t.Fatalf("created feedback = %#v", created)
	}
	if len(created.Attachments) != 1 || created.Attachments[0].OriginalName != "screen.png" {
		t.Fatalf("attachments = %#v", created.Attachments)
	}
	if bytes.Contains(storage.content, []byte("private-image-metadata")) {
		t.Fatal("stored image retained trailing metadata")
	}

	admin, err := service.AdminDetail(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Diagnostics["page"] != "dashboard" {
		t.Fatalf("admin diagnostics = %#v", admin.Diagnostics)
	}
	own, err := service.OwnDetail(t.Context(), 9, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if own.ID != created.ID || len(own.Attachments) != 1 {
		t.Fatalf("own detail = %#v", own)
	}
}

func TestAutomaticFeedbackHonorsSettings(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{settings: AutomaticSettings{Enabled: false}}
	service := NewService(repo, &memoryStorage{})
	input := CreateInput{
		UserID:  9,
		Message: "系统自动上报：资源打开错误",
		Diagnostics: map[string]string{
			"error_code": "asset_not_found",
			"error_name": "Error",
		},
	}

	result, err := service.CreateAutomatic(t.Context(), input, time.Now())
	if err != nil || result.Feedback != nil || result.Reason != "disabled" {
		t.Fatalf("disabled result = %#v, err = %v", result, err)
	}

	repo.settings = AutomaticSettings{
		Enabled:         true,
		MutedErrorTypes: []string{"asset_not_found"},
	}
	result, err = service.CreateAutomatic(t.Context(), input, time.Now())
	if err != nil || result.Feedback != nil || result.Reason != "muted" {
		t.Fatalf("muted result = %#v, err = %v", result, err)
	}

	repo.settings.MutedErrorTypes = nil
	result, err = service.CreateAutomatic(t.Context(), input, time.Now())
	if err != nil || result.Feedback == nil || result.Reason != "" {
		t.Fatalf("created result = %#v, err = %v", result, err)
	}
	if repo.item.Source != SourceAutomatic {
		t.Fatalf("source = %q", repo.item.Source)
	}
}

func TestUpdateAutomaticSettingsNormalizesErrorTypes(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{}
	service := NewService(repo, &memoryStorage{})
	settings, err := service.UpdateAutomaticSettings(t.Context(), AutomaticSettings{
		Enabled:         true,
		MutedErrorTypes: []string{" TypeError ", "typeerror", "ASSET_NOT_FOUND"},
	}, 99, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.MutedErrorTypes) != 2 ||
		settings.MutedErrorTypes[0] != "typeerror" ||
		settings.MutedErrorTypes[1] != "asset_not_found" {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestStatusAndReplyValidation(t *testing.T) {
	t.Parallel()

	repo := &memoryRepository{item: Feedback{ID: 5, UserID: 2, Status: StatusPending}}
	service := NewService(repo, &memoryStorage{})
	if err := service.UpdateStatus(t.Context(), 5, Status("unknown"), time.Now()); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if _, err := service.Reply(t.Context(), 5, 8, "   ", time.Now()); !errors.Is(err, ErrReplyRequired) {
		t.Fatalf("Reply() error = %v", err)
	}
	if _, err := service.Reply(t.Context(), 5, 8, " 已处理 ", time.Now()); err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if repo.reply.Message != "已处理" {
		t.Fatalf("reply = %#v", repo.reply)
	}
	if err := service.UpdateStatus(t.Context(), 5, StatusClosed, time.Now()); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}
	if repo.status != StatusClosed {
		t.Fatalf("status = %q", repo.status)
	}
}
