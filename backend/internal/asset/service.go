package asset

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var (
	ErrInvalidFilename    = errors.New("invalid_filename")
	ErrStorageDirectory   = errors.New("asset_dir_failed")
	ErrStorageWrite       = errors.New("asset_write_failed")
	ErrSharingUnsupported = errors.New("asset_sharing_unsupported")
	ErrInvalidShareScope  = errors.New("invalid_share_scope")
	ErrInvalidGroupCode   = errors.New("invalid_group_code")
	ErrInvalidBatchInput  = errors.New("invalid_batch_input")
)

var (
	groupCodePattern      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	taskScopedTitleRegexp = regexp.MustCompile(`[0-9]{1,4}[[:space:]]*(?:[-~—–至到][[:space:]]*[0-9]{1,4})?[[:space:]]*页`)
)

const maxBatchAssetIDs = 200

type Service struct {
	repo    Repository
	storage Storage
}

func NewService(repo Repository, storage Storage, _ string) *Service {
	return &Service{repo: repo, storage: storage}
}

func (s *Service) List(ctx context.Context, groupID uint64, limit int) ([]AssetVO, error) {
	items, err := s.repo.List(ctx, groupID, limit)
	if err != nil {
		return nil, err
	}
	items = dedupeVisibleAssets(items)
	vos := make([]AssetVO, 0, len(items))
	for _, item := range items {
		vos = append(vos, toAssetVO(item))
	}
	return vos, nil
}

func (s *Service) DownloadFile(ctx context.Context, groupID, id uint64) (*DownloadFile, error) {
	item, err := s.downloadTarget(ctx, groupID, id)
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
	repo, ok := s.repo.(interface {
		GroupCode(context.Context, uint64) (string, error)
	})
	if !ok {
		return nil, ErrSharingUnsupported
	}
	groupCode, err := repo.GroupCode(ctx, req.GroupID)
	if err != nil {
		return nil, err
	}
	if !groupCodePattern.MatchString(groupCode) {
		return nil, ErrInvalidGroupCode
	}
	resourceKey, err := randomResourceKey()
	if err != nil {
		return nil, err
	}
	relativeDir := resourceObjectDir(groupCode, resourceKey)
	stored, err := s.storage.Save(ctx, relativeDir, safeName, req.Reader)
	if err != nil {
		return nil, err
	}
	original := filepath.Base(strings.TrimSpace(req.FileName))
	title := strings.TrimSuffix(original, filepath.Ext(original))
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(original)))
	item := &Asset{
		GroupID:        req.GroupID,
		ResourceKey:    resourceKey,
		AssetKind:      AssetKindOwned,
		Category:       category,
		Title:          title,
		OriginalName:   original,
		StoragePath:    stored.StoragePath,
		MimeType:       mt,
		FileSize:       stored.FileSize,
		ChecksumSHA256: stored.ChecksumSHA256,
		Visibility:     string(ShareScopeAllGroups),
	}
	id, err := s.repo.Create(ctx, item, req.ActorID)
	if err != nil {
		cleanupErr := s.storage.Delete(context.WithoutCancel(ctx), stored.StoragePath)
		if cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			return nil, errors.Join(err, fmt.Errorf("remove stored upload: %w", cleanupErr))
		}
		return nil, err
	}
	item.ID = id
	vo := toAssetVO(*item)
	return &vo, nil
}

func (s *Service) ShareSettings(ctx context.Context, groupID, assetID uint64) (*SharingSettings, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	return repo.ShareSettings(ctx, groupID, assetID)
}

func (s *Service) SaveShareSettings(ctx context.Context, groupID, assetID, actorID uint64, input ShareInput) error {
	repo, err := s.sharingRepo()
	if err != nil {
		return err
	}
	normalized, err := normalizeShareInput(input)
	if err != nil {
		return err
	}
	return repo.SaveShareSettings(ctx, groupID, assetID, actorID, normalized, time.Now().UTC())
}

func (s *Service) SharedResources(ctx context.Context, targetGroupID uint64, filter SharedFilter) ([]SharedResource, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	return repo.SharedResources(ctx, targetGroupID, filter)
}

func (s *Service) ImportPreview(ctx context.Context, targetGroupID uint64, input ImportInput) (*ImportPreview, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	return repo.ImportPreview(ctx, targetGroupID, input.SourceAssetID)
}

func (s *Service) Import(ctx context.Context, targetGroupID, actorID uint64, input ImportInput) (*AssetVO, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	item, err := repo.Import(ctx, targetGroupID, actorID, input, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	vo := toAssetVO(*item)
	return &vo, nil
}

func (s *Service) ImportHistory(ctx context.Context, groupID uint64, limit int) ([]ImportEvent, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	return repo.ImportHistory(ctx, groupID, limit)
}

func (s *Service) RemoveImport(ctx context.Context, groupID, importedAssetID, actorID uint64) error {
	repo, err := s.sharingRepo()
	if err != nil {
		return err
	}
	return repo.RemoveImport(ctx, groupID, importedAssetID, actorID, time.Now().UTC())
}

func (s *Service) DependencyGraph(ctx context.Context, groupID uint64, isSuperAdmin bool, filter DependencyFilter) (*DependencyGraph, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	return repo.DependencyGraph(ctx, groupID, isSuperAdmin, filter)
}

func (s *Service) BatchSaveShareSettings(ctx context.Context, groupID, actorID uint64, input BatchShareInput) (*BatchShareResult, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	shareInput, err := normalizeShareInput(ShareInput{
		Scope:            input.Scope,
		ConsumerGroupIDs: input.ConsumerGroupIDs,
	})
	if err != nil {
		return nil, err
	}
	assetIDs, err := normalizeBatchAssetIDs(input.AssetIDs)
	if err != nil {
		return nil, err
	}
	return repo.BatchSaveShareSettings(ctx, groupID, actorID, BatchShareInput{
		AssetIDs:         assetIDs,
		Scope:            shareInput.Scope,
		ConsumerGroupIDs: shareInput.ConsumerGroupIDs,
	}, time.Now().UTC())
}

func (s *Service) BatchDelete(ctx context.Context, groupID, actorID uint64, input BatchDeleteInput) (*BatchDeleteResult, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	assetIDs, err := normalizeBatchAssetIDs(input.AssetIDs)
	if err != nil {
		return nil, err
	}
	return repo.BatchDelete(ctx, groupID, actorID, BatchDeleteInput{AssetIDs: assetIDs}, time.Now().UTC())
}

func (s *Service) BatchImport(ctx context.Context, targetGroupID, actorID uint64, input BatchImportInput) (*BatchImportResult, error) {
	repo, err := s.sharingRepo()
	if err != nil {
		return nil, err
	}
	sourceAssetIDs, err := normalizeBatchAssetIDs(input.SourceAssetIDs)
	if err != nil {
		return nil, err
	}
	return repo.BatchImport(ctx, targetGroupID, actorID, BatchImportInput{SourceAssetIDs: sourceAssetIDs}, time.Now().UTC())
}

func (s *Service) ResourceLibrary(ctx context.Context, groupID uint64) ([]LibrarySection, error) {
	return s.uploadedLibrarySections(ctx, groupID)
}

func (s *Service) uploadedLibrarySections(ctx context.Context, groupID uint64) ([]LibrarySection, error) {
	items, err := s.repo.List(ctx, groupID, 0)
	if err != nil {
		return nil, err
	}
	items = dedupeVisibleAssets(items)
	grouped := map[string][]LibraryItem{}
	for _, item := range items {
		key := firstNonEmpty(item.Category, "uploaded")
		itemType := inferTaskBindingType("", "", firstNonEmpty(item.OriginalName, item.MimeType))
		if key == "outline" {
			itemType = "image"
		}
		grouped[key] = append(grouped[key], LibraryItem{
			ID:           item.ID,
			Title:        item.Title,
			OriginalName: item.OriginalName,
			URL:          fmt.Sprintf("/api/assets/%d/download", item.ID),
			Category:     key,
			Source:       "uploaded",
			Type:         itemType,
		})
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		leftRank, rightRank := resourceCategoryRank(keys[i]), resourceCategoryRank(keys[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return keys[i] < keys[j]
	})
	sections := make([]LibrarySection, 0, len(keys))
	for _, key := range keys {
		label := "上传资源"
		switch key {
		case "markdown":
			label = "上传 Markdown"
		case "mentor":
			label = "上传 Mentor 导读"
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

func resourceCategoryRank(category string) int {
	switch category {
	case "mentor":
		return 0
	case "book", "passage", "markdown":
		return 1
	case "handout":
		return 2
	case "outline":
		return 3
	case "video":
		return 4
	default:
		return 100
	}
}

func dedupeVisibleAssets(items []Asset) []Asset {
	if len(items) < 2 {
		return items
	}
	out := make([]Asset, 0, len(items))
	indexes := map[string]int{}
	for _, item := range items {
		key := visibleAssetDedupeKey(item)
		if key == "" {
			out = append(out, item)
			continue
		}
		index, exists := indexes[key]
		if !exists {
			indexes[key] = len(out)
			out = append(out, item)
			continue
		}
		if preferVisibleAsset(item, out[index]) {
			out[index] = item
		}
	}
	return out
}

func visibleAssetDedupeKey(item Asset) string {
	category := strings.TrimSpace(strings.ToLower(item.Category))
	originalName := strings.TrimSpace(strings.ToLower(item.OriginalName))
	checksum := strings.TrimSpace(strings.ToLower(item.ChecksumSHA256))
	if category == "" || originalName == "" || checksum == "" || item.FileSize == 0 {
		return ""
	}
	return category + "\x00" + originalName + "\x00" + checksum
}

func preferVisibleAsset(candidate, current Asset) bool {
	candidateScore := visibleAssetScore(candidate)
	currentScore := visibleAssetScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate.ID < current.ID
}

func visibleAssetScore(item Asset) int {
	score := 0
	baseTitle := strings.TrimSuffix(filepath.Base(item.OriginalName), filepath.Ext(item.OriginalName))
	if normalizeAssetDisplayTitle(item.Title) == normalizeAssetDisplayTitle(baseTitle) {
		score += 100
	}
	if !taskScopedTitleRegexp.MatchString(item.Title) {
		score += 20
	}
	if item.AssetKind == AssetKindOwned {
		score += 10
	}
	if item.SourceAssetID == 0 {
		score += 5
	}
	if item.FileSize > 0 {
		score += 2
	}
	if strings.TrimSpace(item.ChecksumSHA256) != "" {
		score += 2
	}
	return score
}

func normalizeAssetDisplayTitle(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeBatchAssetIDs(values []uint64) ([]uint64, error) {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		if value == 0 || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	if len(out) == 0 || len(out) > maxBatchAssetIDs {
		return nil, ErrInvalidBatchInput
	}
	return out, nil
}

func toAssetVO(item Asset) AssetVO {
	return AssetVO{
		ID:            item.ID,
		Category:      item.Category,
		Title:         item.Title,
		OriginalName:  item.OriginalName,
		MimeType:      item.MimeType,
		FileSize:      item.FileSize,
		URL:           fmt.Sprintf("/api/assets/%d/download", item.ID),
		AssetKind:     item.AssetKind,
		SourceAssetID: item.SourceAssetID,
		ImportedAt:    item.ImportedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func resourceObjectDir(groupCode, resourceKey string) string {
	return filepath.Join(
		"team-"+groupCode+"-resources",
		"objects",
		resourceKey,
	)
}

func (s *Service) downloadTarget(ctx context.Context, groupID, id uint64) (*Asset, error) {
	if repo, ok := s.repo.(SharingRepository); ok {
		return repo.FindDownloadTarget(ctx, groupID, id)
	}
	return s.repo.FindByID(ctx, groupID, id)
}

func (s *Service) sharingRepo() (SharingRepository, error) {
	repo, ok := s.repo.(SharingRepository)
	if !ok {
		return nil, ErrSharingUnsupported
	}
	return repo, nil
}

func normalizeShareInput(input ShareInput) (ShareInput, error) {
	switch input.Scope {
	case "", ShareScopePrivate:
		input.Scope = ShareScopePrivate
		input.ConsumerGroupIDs = nil
	case ShareScopeAllGroups:
		input.ConsumerGroupIDs = nil
	case ShareScopeSelectedGroups:
		seen := map[uint64]bool{}
		out := make([]uint64, 0, len(input.ConsumerGroupIDs))
		for _, id := range input.ConsumerGroupIDs {
			if id == 0 || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
		input.ConsumerGroupIDs = out
	default:
		return ShareInput{}, ErrInvalidShareScope
	}
	return input, nil
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

func inferTaskBindingType(taskType, urlValue, fileName string) string {
	text := strings.ToLower(strings.TrimSpace(firstNonEmpty(fileName, urlValue, taskType)))
	switch {
	case strings.Contains(text, "video") || strings.HasSuffix(text, ".mp4") || strings.HasSuffix(text, ".webm") || strings.HasSuffix(text, ".mov") || strings.HasSuffix(text, ".m4v"):
		return "video"
	case strings.Contains(text, "outline") || strings.Contains(text, "提纲") || hasImageExtension(text):
		return "image"
	case strings.HasSuffix(text, ".md"):
		return "markdown"
	default:
		return "reading"
	}
}

func hasImageExtension(value string) bool {
	switch filepath.Ext(strings.TrimSpace(value)) {
	case ".avif", ".bmp", ".gif", ".jpeg", ".jpg", ".png", ".svg", ".tif", ".tiff", ".webp":
		return true
	default:
		return false
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
