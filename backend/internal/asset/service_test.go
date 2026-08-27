package asset

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestServiceUploadDeletesStoredFileWhenMetadataCreateFails(t *testing.T) {
	createErr := errors.New("create failed")
	storage := &fakeStorage{
		stored: &StoredObject{StoragePath: "1/book/upload.pdf"},
	}
	service := NewService(&fakeRepository{createErr: createErr}, storage, "")

	_, err := service.Upload(context.Background(), UploadRequest{
		GroupID:  1,
		ActorID:  2,
		Category: "book",
		FileName: "upload.pdf",
		Reader:   strings.NewReader("pdf"),
	})
	if !errors.Is(err, createErr) {
		t.Fatalf("Upload error = %v, want %v", err, createErr)
	}
	if storage.deleted != storage.stored.StoragePath {
		t.Fatalf("deleted path = %q, want %q", storage.deleted, storage.stored.StoragePath)
	}
}

func TestServiceUploadDefaultsToAllGroupsVisibility(t *testing.T) {
	repo := &fakeRepository{nextID: 12}
	storage := &fakeStorage{
		stored: &StoredObject{StoragePath: "1/book/upload.pdf"},
	}
	service := NewService(repo, storage, "")

	_, err := service.Upload(context.Background(), UploadRequest{
		GroupID:  1,
		ActorID:  2,
		Category: "book",
		FileName: "upload.pdf",
		Reader:   strings.NewReader("pdf"),
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}
	if repo.created.Visibility != string(ShareScopeAllGroups) {
		t.Fatalf("upload visibility = %q, want %q", repo.created.Visibility, ShareScopeAllGroups)
	}
}

func TestInferTaskBindingType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		taskType string
		url      string
		fileName string
		want     string
	}{
		{name: "PNG image", fileName: "本周提纲.png", want: "image"},
		{name: "JPEG image without outline in name", fileName: "week-12.jpeg", want: "image"},
		{name: "WebP image", fileName: "diagram.webp", want: "image"},
		{name: "video", fileName: "lesson.mp4", want: "video"},
		{name: "markdown", fileName: "lesson.md", want: "markdown"},
		{name: "PDF reading", fileName: "lesson.pdf", want: "reading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inferTaskBindingType(tt.taskType, tt.url, tt.fileName); got != tt.want {
				t.Fatalf("inferTaskBindingType(%q, %q, %q) = %q, want %q", tt.taskType, tt.url, tt.fileName, got, tt.want)
			}
		})
	}
}

func TestNormalizeShareInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   ShareInput
		want    ShareInput
		wantErr error
	}{
		{
			name:  "empty scope becomes private",
			input: ShareInput{ConsumerGroupIDs: []uint64{2}},
			want:  ShareInput{Scope: ShareScopePrivate},
		},
		{
			name:  "all groups ignores consumer list",
			input: ShareInput{Scope: ShareScopeAllGroups, ConsumerGroupIDs: []uint64{2}},
			want:  ShareInput{Scope: ShareScopeAllGroups},
		},
		{
			name:  "selected groups deduplicates and drops zero",
			input: ShareInput{Scope: ShareScopeSelectedGroups, ConsumerGroupIDs: []uint64{2, 0, 2, 3}},
			want:  ShareInput{Scope: ShareScopeSelectedGroups, ConsumerGroupIDs: []uint64{2, 3}},
		},
		{
			name:    "invalid scope",
			input:   ShareInput{Scope: "bad"},
			wantErr: ErrInvalidShareScope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeShareInput(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("normalizeShareInput() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if got.Scope != tt.want.Scope || strings.Join(uintsToStrings(got.ConsumerGroupIDs), ",") != strings.Join(uintsToStrings(tt.want.ConsumerGroupIDs), ",") {
				t.Fatalf("normalizeShareInput() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeBatchAssetIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   []uint64
		want    []uint64
		wantErr error
	}{
		{name: "deduplicates and drops zero", input: []uint64{3, 0, 2, 3}, want: []uint64{3, 2}},
		{name: "empty after normalization", input: []uint64{0, 0}, wantErr: ErrInvalidBatchInput},
		{name: "too many IDs", input: make([]uint64, maxBatchAssetIDs+1), wantErr: ErrInvalidBatchInput},
	}
	for index := range tests[2].input {
		tests[2].input[index] = uint64(index + 1)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeBatchAssetIDs(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("normalizeBatchAssetIDs() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if strings.Join(uintsToStrings(got), ",") != strings.Join(uintsToStrings(tt.want), ",") {
				t.Fatalf("normalizeBatchAssetIDs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceObjectDir(t *testing.T) {
	t.Parallel()

	got := filepath.ToSlash(resourceObjectDir("agape-a", "0123456789abcdef0123456789abcdef"))
	want := "team-agape-a-resources/objects/0123456789abcdef0123456789abcdef"
	if got != want {
		t.Fatalf("resourceObjectDir() = %q, want %q", got, want)
	}
	for _, code := range []string{"agape-a", "team2", "a"} {
		if !groupCodePattern.MatchString(code) {
			t.Fatalf("valid group code %q rejected", code)
		}
	}
	for _, code := range []string{"AGAPE_A", "../other", "中文名"} {
		if groupCodePattern.MatchString(code) {
			t.Fatalf("invalid group code %q accepted", code)
		}
	}
}

func TestResourceLibraryReturnsDatabaseResources(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{{
		ID:           12,
		Category:     "book",
		Title:        "课程资料",
		OriginalName: "lesson.pdf",
		StoragePath:  "team-demo-resources/objects/key/lesson.pdf",
		MimeType:     "application/pdf",
	}}}, &fakeStorage{}, "")
	sections, err := service.ResourceLibrary(context.Background(), 6)
	if err != nil {
		t.Fatalf("ResourceLibrary() error = %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("ResourceLibrary() returned %d sections, want 1", len(sections))
	}
	if sections[0].Key != "uploaded_book" || sections[0].Count != 1 {
		t.Fatalf("section = %+v, want uploaded_book with 1 item", sections[0])
	}
	if got := sections[0].Items[0].URL; got != "/api/assets/12/download" {
		t.Fatalf("item URL = %q, want /api/assets/12/download", got)
	}
}

func TestResourceLibraryLabelsMentorResources(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{{
		ID:           18,
		Category:     "mentor",
		Title:        "马太福音导读",
		OriginalName: "马太福音导读.pdf",
		StoragePath:  "team-demo-resources/objects/00000000000000000000000000000012/mentor.pdf",
		MimeType:     "application/pdf",
	}}}, &fakeStorage{}, "")
	sections, err := service.ResourceLibrary(context.Background(), 6)
	if err != nil {
		t.Fatalf("ResourceLibrary() error = %v", err)
	}
	if len(sections) != 1 {
		t.Fatalf("ResourceLibrary() returned %d sections, want 1", len(sections))
	}
	if sections[0].Key != "uploaded_mentor" || sections[0].Label != "上传 Mentor 导读" {
		t.Fatalf("section = %+v, want Mentor section", sections[0])
	}
}

func TestResourceLibraryDeduplicatesSameFileAndPrefersCanonicalTitle(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{
		{
			ID:             7,
			Category:       "book",
			Title:          "《基督是一切》36-40页",
			OriginalName:   "基督是一切-江守道.pdf",
			StoragePath:    "team-demo-resources/objects/00000000000000000000000000000007/基督是一切-江守道.pdf",
			MimeType:       "application/pdf",
			FileSize:       1024,
			ChecksumSHA256: "0ede4c556a220000000000000000000000000000000000000000000000000000",
			AssetKind:      AssetKindOwned,
		},
		{
			ID:             21,
			Category:       "book",
			Title:          "基督是一切-江守道",
			OriginalName:   "基督是一切-江守道.pdf",
			StoragePath:    "team-demo-resources/objects/00000000000000000000000000000021/基督是一切-江守道.pdf",
			MimeType:       "application/pdf",
			FileSize:       1024,
			ChecksumSHA256: "0ede4c556a220000000000000000000000000000000000000000000000000000",
			AssetKind:      AssetKindOwned,
		},
	}}, &fakeStorage{}, "")
	sections, err := service.ResourceLibrary(context.Background(), 6)
	if err != nil {
		t.Fatalf("ResourceLibrary() error = %v", err)
	}
	if len(sections) != 1 || len(sections[0].Items) != 1 {
		t.Fatalf("sections = %+v, want one deduplicated item", sections)
	}
	item := sections[0].Items[0]
	if item.ID != 21 || item.Title != "基督是一切-江守道" {
		t.Fatalf("deduplicated item = %+v, want canonical asset 21", item)
	}
}

func TestServiceListDeduplicatesSameFile(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{
		{
			ID:             4,
			Category:       "video",
			Title:          "新约圣经-08-220714-马可福音(下)",
			OriginalName:   "[B311]新约圣经-08-220714-马可福音(下).mp4",
			StoragePath:    "team-demo-resources/objects/00000000000000000000000000000004/[B311]新约圣经-08-220714-马可福音(下).mp4",
			MimeType:       "video/mp4",
			FileSize:       2048,
			ChecksumSHA256: "231b5f2a534f0000000000000000000000000000000000000000000000000000",
			AssetKind:      AssetKindOwned,
		},
		{
			ID:             40,
			Category:       "video",
			Title:          "[B311]新约圣经-08-220714-马可福音(下)",
			OriginalName:   "[B311]新约圣经-08-220714-马可福音(下).mp4",
			StoragePath:    "team-demo-resources/objects/00000000000000000000000000000040/[B311]新约圣经-08-220714-马可福音(下).mp4",
			MimeType:       "video/mp4",
			FileSize:       2048,
			ChecksumSHA256: "231b5f2a534f0000000000000000000000000000000000000000000000000000",
			AssetKind:      AssetKindOwned,
		},
	}}, &fakeStorage{}, "")
	items, err := service.List(context.Background(), 6, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 || items[0].ID != 40 {
		t.Fatalf("List() = %+v, want canonical video asset 40 only", items)
	}
}

func TestResourceLibraryOrdersMentorBeforeOtherCategories(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{
		{ID: 4, Category: "video", Title: "视频", OriginalName: "video.mp4", StoragePath: "team-demo-resources/objects/4/video.mp4"},
		{ID: 2, Category: "book", Title: "读物", OriginalName: "book.pdf", StoragePath: "team-demo-resources/objects/2/book.pdf"},
		{ID: 1, Category: "mentor", Title: "导读", OriginalName: "mentor.pdf", StoragePath: "team-demo-resources/objects/1/mentor.pdf"},
		{ID: 3, Category: "handout", Title: "讲义", OriginalName: "handout.pdf", StoragePath: "team-demo-resources/objects/3/handout.pdf"},
	}}, &fakeStorage{}, "")
	sections, err := service.ResourceLibrary(context.Background(), 6)
	if err != nil {
		t.Fatalf("ResourceLibrary() error = %v", err)
	}
	got := []string{}
	for _, section := range sections {
		got = append(got, section.Key)
	}
	want := []string{"uploaded_mentor", "uploaded_book", "uploaded_handout", "uploaded_video"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("section keys = %v, want %v", got, want)
	}
}

type fakeRepository struct {
	createErr error
	created   Asset
	list      []Asset
	listErr   error
	groupCode string
	nextID    uint64
}

func (r *fakeRepository) FindByID(context.Context, uint64, uint64) (*Asset, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeRepository) List(context.Context, uint64, int) ([]Asset, error) {
	return r.list, r.listErr
}

func (r *fakeRepository) GroupCode(context.Context, uint64) (string, error) {
	if r.groupCode != "" {
		return r.groupCode, nil
	}
	return "agp", nil
}

func (r *fakeRepository) Create(_ context.Context, item *Asset, _ uint64) (uint64, error) {
	if item != nil {
		r.created = *item
	}
	if r.createErr != nil {
		return 0, r.createErr
	}
	if r.nextID > 0 {
		return r.nextID, nil
	}
	return 1, nil
}

func (r *fakeRepository) Delete(context.Context, uint64, uint64) error {
	return errors.New("not implemented")
}

type fakeStorage struct {
	stored  *StoredObject
	deleted string
}

func (s *fakeStorage) Save(context.Context, string, string, io.Reader) (*StoredObject, error) {
	return s.stored, nil
}

func (s *fakeStorage) Resolve(context.Context, string) (*ResolvedObject, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeStorage) Delete(_ context.Context, objectKey string) error {
	s.deleted = objectKey
	return nil
}

func uintsToStrings(values []uint64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.FormatUint(value, 10))
	}
	return out
}
