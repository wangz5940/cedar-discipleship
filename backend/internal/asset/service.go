package asset

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var (
	ErrInvalidFilename  = errors.New("invalid_filename")
	ErrStorageDirectory = errors.New("asset_dir_failed")
	ErrStorageWrite     = errors.New("asset_write_failed")
)

type Service struct {
	repo        Repository
	storage     Storage
	contentRoot string
}

func NewService(repo Repository, storage Storage, contentRoot string) *Service {
	return &Service{repo: repo, storage: storage, contentRoot: contentRoot}
}

func (s *Service) List(ctx context.Context, groupID uint64, limit int) ([]AssetVO, error) {
	items, err := s.repo.List(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	vos := make([]AssetVO, 0, len(items))
	for _, item := range items {
		vos = append(vos, toAssetVO(item))
	}
	return vos, nil
}

func (s *Service) DownloadFile(ctx context.Context, groupID, id uint64) (*DownloadFile, error) {
	item, err := s.repo.FindByID(ctx, groupID, id)
	if err != nil {
		return nil, err
	}
	resolved, err := s.storage.Resolve(ctx, item.StoragePath)
	if err != nil {
		return nil, err
	}
	original := firstNonEmpty(item.OriginalName, resolved.OriginalName)
	mt := item.MimeType
	if mt == "" {
		mt = mime.TypeByExtension(filepath.Ext(original))
	}
	return &DownloadFile{
		AbsolutePath: resolved.AbsolutePath,
		OriginalName: original,
		MimeType:     mt,
	}, nil
}

func (s *Service) Upload(ctx context.Context, req UploadRequest) (*AssetVO, error) {
	safeName := sanitizeUploadName(req.FileName)
	if safeName == "" {
		return nil, ErrInvalidFilename
	}
	category := firstNonEmpty(req.Category, "uploaded")
	relativeDir := filepath.Join(strconv.FormatUint(req.GroupID, 10), category)
	stored, err := s.storage.Save(ctx, relativeDir, safeName, req.Reader)
	if err != nil {
		return nil, err
	}
	original := filepath.Base(strings.TrimSpace(req.FileName))
	title := strings.TrimSuffix(original, filepath.Ext(original))
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(original)))
	item := &Asset{
		GroupID:        req.GroupID,
		Category:       category,
		Title:          title,
		OriginalName:   original,
		StoragePath:    stored.StoragePath,
		MimeType:       mt,
		FileSize:       stored.FileSize,
		ChecksumSHA256: stored.ChecksumSHA256,
		Visibility:     "group",
	}
	id, err := s.repo.Create(ctx, item, req.ActorID)
	if err != nil {
		return nil, err
	}
	item.ID = id
	vo := toAssetVO(*item)
	return &vo, nil
}

func (s *Service) CreateMetadata(ctx context.Context, req CreateRequest) (uint64, error) {
	item := &Asset{
		GroupID:      req.GroupID,
		Category:     req.Category,
		Title:        req.Title,
		OriginalName: filepath.Base(req.StoragePath),
		StoragePath:  req.StoragePath,
		MimeType:     req.MimeType,
		FileSize:     req.FileSize,
		Visibility:   "group",
	}
	return s.repo.Create(ctx, item, req.ActorID)
}

func (s *Service) ResourceLibrary(ctx context.Context, groupID uint64) ([]LibrarySection, error) {
	sections := []LibrarySection{
		s.scanStaticLibrarySection("markdown", "Markdown 读物", "", "/", []string{".md"}),
		s.scanStaticLibrarySection("book", "PDF 读物", "Book", "/Book", []string{".pdf"}),
		s.scanStaticLibrarySection("video", "视频文件", "Newtestament", "/Newtestament", []string{".mp4", ".webm", ".mov", ".m4v"}),
		s.scanStaticLibrarySection("handout", "讲义 PDF", "PPT", "/PPT", []string{".pdf"}),
	}
	uploaded, err := s.uploadedLibrarySections(ctx, groupID)
	if err != nil {
		return nil, err
	}
	sections = append(sections, uploaded...)
	return sections, nil
}

func (s *Service) scanStaticLibrarySection(key, label, subdir, publicPrefix string, extensions []string) LibrarySection {
	root := s.contentRoot
	if subdir != "" {
		root = filepath.Join(root, subdir)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return LibrarySection{Key: key, Label: label, Items: []LibraryItem{}, Count: 0}
	}
	allowed := map[string]bool{}
	for _, ext := range extensions {
		allowed[strings.ToLower(ext)] = true
	}
	items := make([]LibraryItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if len(allowed) > 0 && !allowed[ext] {
			continue
		}
		title := strings.TrimSuffix(name, ext)
		if strings.HasPrefix(title, "[B311]") {
			title = strings.TrimPrefix(title, "[B311]")
		}
		urlPath := publicPrefix
		if !strings.HasPrefix(urlPath, "/") {
			urlPath = "/" + strings.TrimPrefix(urlPath, "/")
		}
		if urlPath == "/" {
			urlPath = "/" + encodeURLPath(name)
		} else {
			urlPath = strings.TrimRight(urlPath, "/") + "/" + encodeURLPath(name)
		}
		items = append(items, LibraryItem{
			Title:        title,
			OriginalName: name,
			URL:          urlPath,
			Category:     key,
			Source:       "static",
			Type:         inferTaskBindingType("", "", name),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Title < items[j].Title
	})
	return LibrarySection{Key: key, Label: label, Items: items, Count: len(items)}
}

func (s *Service) uploadedLibrarySections(ctx context.Context, groupID uint64) ([]LibrarySection, error) {
	items, err := s.repo.List(ctx, groupID, 0)
	if err != nil {
		return nil, err
	}
	grouped := map[string][]LibraryItem{}
	for _, item := range items {
		key := firstNonEmpty(item.Category, "uploaded")
		grouped[key] = append(grouped[key], LibraryItem{
			ID:           item.ID,
			Title:        item.Title,
			OriginalName: item.OriginalName,
			URL:          fmt.Sprintf("/api/assets/%d/download", item.ID),
			Category:     key,
			Source:       "uploaded",
			Type:         inferTaskBindingType("", "", firstNonEmpty(item.OriginalName, item.MimeType)),
		})
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sections := make([]LibrarySection, 0, len(keys))
	for _, key := range keys {
		label := "上传资源"
		switch key {
		case "markdown":
			label = "上传 Markdown"
		case "book":
			label = "上传 PDF 读物"
		case "video":
			label = "上传视频"
		case "handout":
			label = "上传讲义"
		case "outline":
			label = "上传提纲图片"
		}
		items := grouped[key]
		sections = append(sections, LibrarySection{Key: "uploaded_" + key, Label: label, Items: items, Count: len(items)})
	}
	return sections, nil
}

func toAssetVO(item Asset) AssetVO {
	return AssetVO{
		ID:           item.ID,
		Category:     item.Category,
		Title:        item.Title,
		OriginalName: item.OriginalName,
		MimeType:     item.MimeType,
		FileSize:     item.FileSize,
		URL:          fmt.Sprintf("/api/assets/%d/download", item.ID),
	}
}

func sanitizeUploadName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
		case strings.ContainsRune("._-()[]（）【】 ", r):
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return strings.TrimSpace(b.String())
}

func encodeURLPath(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func inferTaskBindingType(taskType, urlValue, fileName string) string {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(fileName, urlValue, taskType)))
	switch {
	case strings.Contains(text, "video") || strings.HasSuffix(text, ".mp4") || strings.HasSuffix(text, ".webm") || strings.HasSuffix(text, ".mov") || strings.HasSuffix(text, ".m4v"):
		return "video"
	case strings.Contains(text, "outline") || strings.Contains(text, "提纲"):
		return "outline"
	case strings.HasSuffix(text, ".md"):
		return "markdown"
	default:
		return "reading"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
