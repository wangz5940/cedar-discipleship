package asset

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

func TestServiceUploadHonorsPrivateVisibility(t *testing.T) {
	repo := &fakeRepository{nextID: 12}
	storage := &fakeStorage{
		stored: &StoredObject{StoragePath: "1/ministry/upload.pdf"},
	}
	service := NewService(repo, storage, "")

	_, err := service.Upload(context.Background(), UploadRequest{
		GroupID:    1,
		ActorID:    2,
		Category:   "ministry-3",
		Visibility: ShareScopePrivate,
		FileName:   "upload.pdf",
		Reader:     strings.NewReader("pdf"),
	})
	if err != nil {
		t.Fatalf("Upload error = %v", err)
	}
	if repo.created.Visibility != string(ShareScopePrivate) {
		t.Fatalf("upload visibility = %q, want %q", repo.created.Visibility, ShareScopePrivate)
	}
}

func TestServiceUploadValidatesCategoryAgainstFileFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category string
		fileName string
		wantErr  error
	}{
		{name: "markdown accepts markdown", category: "markdown", fileName: "课程.md"},
		{name: "markdown rejects PDF", category: "markdown", fileName: "课程.pdf", wantErr: ErrAssetCategoryFileMismatch},
		{name: "handout accepts PDF", category: "handout", fileName: "课程.pdf"},
		{name: "handout rejects markdown", category: "handout", fileName: "课程.md", wantErr: ErrAssetCategoryFileMismatch},
		{name: "media accepts audio", category: "video", fileName: "课程.mp3"},
		{name: "media accepts video", category: "video", fileName: "课程.mp4"},
		{name: "media rejects PDF", category: "video", fileName: "课程.pdf", wantErr: ErrAssetCategoryFileMismatch},
		{name: "unknown category rejected", category: "unknown", fileName: "课程.pdf", wantErr: ErrInvalidAssetCategory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepository{}
			storage := &fakeStorage{stored: &StoredObject{StoragePath: "test/object"}}
			service := NewService(repo, storage, "")

			_, err := service.Upload(context.Background(), UploadRequest{
				GroupID:  1,
				ActorID:  2,
				Category: tt.category,
				FileName: tt.fileName,
				Reader:   strings.NewReader("content"),
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Upload() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && storage.saves != 0 {
				t.Fatalf("storage saves = %d, want 0", storage.saves)
			}
		})
	}
}

func TestServiceUploadMergesLegacyAudioCategoryIntoMedia(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{}
	service := NewService(repo, &fakeStorage{
		stored: &StoredObject{StoragePath: "test/audio.mp3"},
	}, "")

	if _, err := service.Upload(context.Background(), UploadRequest{
		GroupID: 1, ActorID: 2, Category: "audio", FileName: "课程.mp3", Reader: strings.NewReader("audio"),
	}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if repo.created.Category != "video" {
		t.Fatalf("created category = %q, want video", repo.created.Category)
	}
}

func TestServiceRenameNormalizesTitle(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		find: &Asset{ID: 12, GroupID: 6, Title: "新名称"},
	}
	service := NewService(repo, &fakeStorage{}, "")
	item, err := service.Rename(context.Background(), 6, 12, RenameInput{Title: "  新名称  "})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if repo.renamedTitle != "新名称" {
		t.Fatalf("repository title = %q, want %q", repo.renamedTitle, "新名称")
	}
	if item.ID != 12 || item.Title != "新名称" {
		t.Fatalf("Rename() = %+v, want renamed asset", item)
	}
}

func TestServiceChangeCategoryValidatesFormatAndUpdatesAsset(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{
		find: &Asset{ID: 12, GroupID: 6, Category: "markdown", OriginalName: "课程.pdf"},
	}
	service := NewService(repo, &fakeStorage{}, "")

	item, err := service.ChangeCategory(context.Background(), 6, 12, ChangeCategoryInput{Category: "handout"})
	if err != nil {
		t.Fatalf("ChangeCategory() error = %v", err)
	}
	if repo.changedCategory != "handout" || item.Category != "handout" {
		t.Fatalf("changed category = repository %q, response %q", repo.changedCategory, item.Category)
	}

	_, err = service.ChangeCategory(context.Background(), 6, 12, ChangeCategoryInput{Category: "markdown"})
	if !errors.Is(err, ErrAssetCategoryFileMismatch) {
		t.Fatalf("ChangeCategory() mismatch error = %v, want %v", err, ErrAssetCategoryFileMismatch)
	}
}

func TestServiceBatchChangeCategoryValidatesAllAssetsBeforeWrite(t *testing.T) {
	t.Parallel()

	repo := &fakeRepository{finds: map[uint64]*Asset{
		11: {ID: 11, GroupID: 6, Category: "markdown", OriginalName: "第一课.pdf"},
		12: {ID: 12, GroupID: 6, Category: "book", OriginalName: "第二课.pdf"},
	}}
	service := NewService(repo, &fakeStorage{}, "")

	result, err := service.BatchChangeCategory(context.Background(), 6, BatchCategoryInput{
		AssetIDs: []uint64{11, 12, 11},
		Category: "share",
	})
	if err != nil {
		t.Fatalf("BatchChangeCategory() error = %v", err)
	}
	if strings.Join(uintsToStrings(repo.batchCategoryIDs), ",") != "11,12" ||
		repo.batchCategory != "handout" || result.Count != 2 {
		t.Fatalf("batch update = ids %v category %q result %+v", repo.batchCategoryIDs, repo.batchCategory, result)
	}

	repo.finds[12].OriginalName = "第二课.md"
	repo.batchCategoryIDs = nil
	_, err = service.BatchChangeCategory(context.Background(), 6, BatchCategoryInput{
		AssetIDs: []uint64{11, 12},
		Category: "handout",
	})
	if !errors.Is(err, ErrAssetCategoryFileMismatch) {
		t.Fatalf("BatchChangeCategory() mismatch error = %v, want %v", err, ErrAssetCategoryFileMismatch)
	}
	if repo.batchCategoryIDs != nil {
		t.Fatalf("repository received a partial batch: %v", repo.batchCategoryIDs)
	}
}

func TestServiceRenameRejectsInvalidTitle(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		" \t ",
		"包含\n换行",
		strings.Repeat("名", 256),
	}
	for _, title := range tests {
		title := title
		t.Run(strconv.Quote(title), func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepository{}
			service := NewService(repo, &fakeStorage{}, "")
			if _, err := service.Rename(context.Background(), 6, 12, RenameInput{Title: title}); !errors.Is(err, ErrInvalidAssetTitle) {
				t.Fatalf("Rename() error = %v, want %v", err, ErrInvalidAssetTitle)
			}
			if repo.renamedTitle != "" {
				t.Fatalf("repository called with title %q", repo.renamedTitle)
			}
		})
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
		{name: "MP3 audio", fileName: "lesson.mp3", want: "audio"},
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

func TestResourceLibraryLabelsAudioAndPassageResources(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeRepository{list: []Asset{
		{
			ID:           19,
			Category:     "passage",
			Title:        "课程文字稿",
			OriginalName: "transcript.pdf",
			StoragePath:  "team-demo-resources/objects/00000000000000000000000000000013/transcript.pdf",
			MimeType:     "application/pdf",
		},
		{
			ID:           20,
			Category:     "audio",
			Title:        "课程音频",
			OriginalName: "lesson.mp3",
			StoragePath:  "team-demo-resources/objects/00000000000000000000000000000014/lesson.mp3",
			MimeType:     "audio/mpeg",
		},
	}}, &fakeStorage{}, "")
	sections, err := service.ResourceLibrary(context.Background(), 6)
	if err != nil {
		t.Fatalf("ResourceLibrary() error = %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("ResourceLibrary() returned %d sections, want 2", len(sections))
	}
	if sections[0].Key != "uploaded_passage" || sections[0].Label != "上传文字稿 / 读物" {
		t.Fatalf("passage section = %+v", sections[0])
	}
	if sections[1].Key != "uploaded_audio" || sections[1].Label != "上传音频" {
		t.Fatalf("audio section = %+v", sections[1])
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
	createErr        error
	created          Asset
	find             *Asset
	finds            map[uint64]*Asset
	list             []Asset
	listErr          error
	groupCode        string
	nextID           uint64
	renamedTitle     string
	changedCategory  string
	batchCategoryIDs []uint64
	batchCategory    string
}

func (r *fakeRepository) FindByID(_ context.Context, _, assetID uint64) (*Asset, error) {
	if item := r.finds[assetID]; item != nil {
		return item, nil
	}
	if r.find != nil {
		return r.find, nil
	}
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

func (r *fakeRepository) Rename(_ context.Context, _, _ uint64, title string, _ time.Time) error {
	r.renamedTitle = title
	return nil
}

func (r *fakeRepository) ChangeCategory(_ context.Context, _, _ uint64, category string, _ time.Time) error {
	r.changedCategory = category
	if r.find != nil {
		r.find.Category = category
	}
	return nil
}

func (r *fakeRepository) BatchChangeCategory(_ context.Context, _ uint64, input BatchCategoryInput, _ time.Time) (*BatchCategoryResult, error) {
	r.batchCategoryIDs = append([]uint64(nil), input.AssetIDs...)
	r.batchCategory = input.Category
	return &BatchCategoryResult{
		AssetIDs: input.AssetIDs,
		Category: input.Category,
		Count:    len(input.AssetIDs),
	}, nil
}

type fakeStorage struct {
	stored  *StoredObject
	deleted string
	saves   int
}

func (s *fakeStorage) Save(context.Context, string, string, io.Reader) (*StoredObject, error) {
	s.saves++
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
