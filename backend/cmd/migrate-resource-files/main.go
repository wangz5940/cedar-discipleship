package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "github.com/go-sql-driver/mysql"
)

const (
	shareScopeAllGroups          = "all_groups"
	assetKindOwned               = "owned"
	newResourceStorageSQLPattern = "team-%-resources/objects/%"
)

var taskScopedTitleRegexp = regexp.MustCompile(`[0-9]{1,4}[[:space:]]*(?:[-~—–至到][[:space:]]*[0-9]{1,4})?[[:space:]]*页`)

var legacyResourceDirs = []struct {
	Name     string
	Category string
}{
	{Name: "Mentor", Category: "mentor"},
	{Name: "Book", Category: "book"},
	{Name: "Passage", Category: "passage"},
	{Name: "PPT", Category: "handout"},
	{Name: "Newtestament", Category: "video"},
}

var legacyRootResourceFiles = []struct {
	Name     string
	Category string
}{
	{Name: "newtestament.md", Category: "markdown"},
	{Name: "weekly_task.md", Category: "markdown"},
	{Name: "Kuangye.md", Category: "markdown"},
}

type options struct {
	dsn              string
	groupName        string
	groupCode        string
	legacyRoot       string
	legacyAssetsRoot string
	resourceRoot     string
	dryRun           bool
}

type studyGroup struct {
	ID   uint64
	Code string
	Name string
}

type legacyAsset struct {
	ID             uint64
	GroupID        uint64
	Category       string
	Title          string
	OriginalName   string
	StoragePath    string
	MimeType       string
	CreatedBy      uint64
	ResourceKey    string
	BindingDeleted sql.NullTime
}

type storedObject struct {
	StoragePath    string
	FileSize       uint64
	ChecksumSHA256 string
	MimeType       string
}

type legacyResourceFile struct {
	RelativePath string
	Category     string
}

func main() {
	var opt options
	flag.StringVar(&opt.dsn, "dsn", "", "MySQL DSN")
	flag.StringVar(&opt.groupName, "group-name", "", "study group name from database")
	flag.StringVar(&opt.groupCode, "group-code", "", "study group code from database")
	flag.StringVar(&opt.legacyRoot, "legacy-root", ".", "root containing existing resource files")
	flag.StringVar(&opt.legacyAssetsRoot, "legacy-assets-root", "", "root containing legacy uploaded assets")
	flag.StringVar(&opt.resourceRoot, "resource-root", "../data/resources", "new resource root")
	flag.BoolVar(&opt.dryRun, "dry-run", true, "report without copying files or updating database")
	flag.Parse()

	if err := run(opt); err != nil {
		log.Fatal(err)
	}
}

func run(opt options) error {
	if strings.TrimSpace(opt.dsn) == "" {
		return errors.New("--dsn is required")
	}
	if strings.TrimSpace(opt.groupName) == "" && strings.TrimSpace(opt.groupCode) == "" {
		return errors.New("--group-name or --group-code is required")
	}
	if strings.TrimSpace(opt.legacyRoot) == "" {
		return errors.New("--legacy-root is required")
	}
	if strings.TrimSpace(opt.resourceRoot) == "" {
		return errors.New("--resource-root is required")
	}

	db, err := sql.Open("mysql", opt.dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	group, err := lookupGroup(ctx, db, opt)
	if err != nil {
		return err
	}
	assets, err := listLegacyAssets(ctx, db, group.ID)
	if err != nil {
		return err
	}

	var migrated, missing, skipped int
	for _, asset := range assets {
		result, err := migrateAsset(ctx, db, opt, group, asset)
		if err != nil {
			return fmt.Errorf("asset %d %q: %w", asset.ID, asset.StoragePath, err)
		}
		switch result {
		case "migrated":
			migrated++
		case "missing":
			missing++
		default:
			skipped++
		}
	}
	discoveredFiles, err := discoverLegacyResourceFiles(opt.legacyRoot)
	if err != nil {
		return err
	}
	registeredFiles, existingFiles, discoveredMissing, discoveredSkipped, err := registerDiscoveredLegacyFiles(ctx, db, opt, group, discoveredFiles)
	if err != nil {
		return err
	}
	missing += discoveredMissing
	skipped += discoveredSkipped
	repairedLinks, err := repairTaskAssetLinks(ctx, db, group.ID, opt.dryRun)
	if err != nil {
		return err
	}
	dedupedResources, err := cleanupDuplicateResourceBindings(ctx, db, group.ID, opt.dryRun)
	if err != nil {
		return err
	}
	repairedAssetTitles, err := repairTaskScopedAssetTitles(ctx, db, group.ID, opt.dryRun)
	if err != nil {
		return err
	}
	repairedConfigPaths, err := repairLearningConfigPaths(ctx, db, group.ID, opt.dryRun)
	if err != nil {
		return err
	}

	mode := "apply"
	if opt.dryRun {
		mode = "dry-run"
	}
	fmt.Printf("resource_file_migration mode=%s group_id=%d group_code=%s group_name=%q total=%d legacy_assets=%d discovered_files=%d migrated=%d registered_files=%d existing_files=%d missing=%d skipped=%d repaired_task_links=%d repaired_asset_titles=%d repaired_config_paths=%d deduped_resources=%d\n",
		mode, group.ID, group.Code, group.Name, len(assets)+len(discoveredFiles), len(assets), len(discoveredFiles), migrated, registeredFiles, existingFiles, missing, skipped, repairedLinks, repairedAssetTitles, repairedConfigPaths, dedupedResources)
	return nil
}

func lookupGroup(ctx context.Context, db *sql.DB, opt options) (studyGroup, error) {
	var rows *sql.Rows
	var err error
	switch {
	case strings.TrimSpace(opt.groupName) != "" && strings.TrimSpace(opt.groupCode) != "":
		rows, err = db.QueryContext(ctx, "SELECT id, code, name FROM study_groups WHERE name=? AND code=? ORDER BY id", strings.TrimSpace(opt.groupName), strings.TrimSpace(opt.groupCode))
	case strings.TrimSpace(opt.groupName) != "":
		rows, err = db.QueryContext(ctx, "SELECT id, code, name FROM study_groups WHERE name=? ORDER BY id", strings.TrimSpace(opt.groupName))
	default:
		rows, err = db.QueryContext(ctx, "SELECT id, code, name FROM study_groups WHERE code=? ORDER BY id", strings.TrimSpace(opt.groupCode))
	}
	if err != nil {
		return studyGroup{}, err
	}
	defer rows.Close()

	var groups []studyGroup
	for rows.Next() {
		var group studyGroup
		if err := rows.Scan(&group.ID, &group.Code, &group.Name); err != nil {
			return studyGroup{}, err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return studyGroup{}, err
	}
	if len(groups) == 0 {
		return studyGroup{}, errors.New("study group not found")
	}
	if len(groups) > 1 {
		return studyGroup{}, errors.New("multiple study groups matched; use --group-code with --group-name")
	}
	return groups[0], nil
}

func listLegacyAssets(ctx context.Context, db *sql.DB, groupID uint64) ([]legacyAsset, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.group_id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.created_by,
	       COALESCE(b.resource_key,''),b.deleted_at
	  FROM assets a
	  LEFT JOIN asset_bindings b ON b.asset_id=a.id
	 WHERE a.group_id=? AND a.storage_path NOT LIKE 'team-%-resources/objects/%'
	 ORDER BY a.id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []legacyAsset
	for rows.Next() {
		var asset legacyAsset
		if err := rows.Scan(
			&asset.ID,
			&asset.GroupID,
			&asset.Category,
			&asset.Title,
			&asset.OriginalName,
			&asset.StoragePath,
			&asset.MimeType,
			&asset.CreatedBy,
			&asset.ResourceKey,
			&asset.BindingDeleted,
		); err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

func migrateAsset(ctx context.Context, db *sql.DB, opt options, group studyGroup, asset legacyAsset) (string, error) {
	relativeSource, ok, err := legacyRelativePath(asset.StoragePath)
	if err != nil {
		return "", err
	}
	if !ok {
		return "skipped", nil
	}
	sourcePath, found, err := resolveLegacySourcePath(opt.legacyRoot, opt.legacyAssetsRoot, relativeSource)
	if err != nil {
		return "", err
	}
	if !found {
		fmt.Printf("missing asset_id=%d source=%s\n", asset.ID, relativeSource)
		return "missing", nil
	}

	fileName := safeFileName(firstNonEmpty(asset.OriginalName, filepath.Base(relativeSource), fmt.Sprintf("asset-%d", asset.ID)))
	resourceKey := asset.ResourceKey
	if !isHexResourceKey(resourceKey) {
		resourceKey = fmt.Sprintf("%032x", asset.ID)
	}
	storagePath := path.Join("team-"+group.Code+"-resources", "objects", resourceKey, fileName)
	category := migratedCategory(asset)
	if opt.dryRun {
		fmt.Printf("would migrate asset_id=%d group=%q source=%s target=%s category=%s\n", asset.ID, group.Name, relativeSource, storagePath, category)
		return "migrated", nil
	}

	stored, err := copyResourceFile(opt.resourceRoot, storagePath, sourcePath)
	if err != nil {
		return "", err
	}
	if stored.MimeType == "" {
		stored.MimeType = firstNonEmpty(asset.MimeType, mime.TypeByExtension(filepath.Ext(fileName)))
	}
	if err := updateAsset(ctx, db, group, asset, resourceKey, category, fileName, stored, relativeSource); err != nil {
		return "", err
	}
	fmt.Printf("migrated asset_id=%d group=%q source=%s target=%s\n", asset.ID, group.Name, relativeSource, storagePath)
	return "migrated", nil
}

func discoverLegacyResourceFiles(legacyRoot string) ([]legacyResourceFile, error) {
	root, err := filepath.Abs(legacyRoot)
	if err != nil {
		return nil, err
	}
	var files []legacyResourceFile
	for _, legacyFile := range legacyRootResourceFiles {
		filePath, err := safeJoin(root, legacyFile.Name)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if info.IsDir() {
			continue
		}
		files = append(files, legacyResourceFile{
			RelativePath: filepath.Clean(legacyFile.Name),
			Category:     legacyFile.Category,
		})
	}
	for _, legacyDir := range legacyResourceDirs {
		dirPath, err := safeJoin(root, legacyDir.Name)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(dirPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		if err := filepath.WalkDir(dirPath, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if filePath != dirPath && strings.HasPrefix(entry.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasPrefix(entry.Name(), ".") || !isSupportedLegacyResourceFile(entry.Name()) {
				return nil
			}
			relativePath, err := filepath.Rel(root, filePath)
			if err != nil {
				return err
			}
			files = append(files, legacyResourceFile{
				RelativePath: filepath.Clean(relativePath),
				Category:     legacyDir.Category,
			})
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sortLegacyResourceFiles(files)
	return files, nil
}

func sortLegacyResourceFiles(files []legacyResourceFile) {
	sort.Slice(files, func(i, j int) bool {
		left := filepath.ToSlash(files[i].RelativePath)
		right := filepath.ToSlash(files[j].RelativePath)
		return left < right
	})
}

func isSupportedLegacyResourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf", ".md", ".markdown", ".mp4", ".m4v", ".mov", ".webm", ".png", ".jpg", ".jpeg", ".webp", ".ppt", ".pptx", ".doc", ".docx", ".xls", ".xlsx":
		return true
	default:
		return false
	}
}

func registerDiscoveredLegacyFiles(ctx context.Context, db *sql.DB, opt options, group studyGroup, files []legacyResourceFile) (int, int, int, int, error) {
	var registered, existing, missing, skipped int
	for _, file := range files {
		result, err := registerDiscoveredLegacyFile(ctx, db, opt, group, file)
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("legacy file %q: %w", file.RelativePath, err)
		}
		switch result {
		case "registered":
			registered++
		case "existing":
			existing++
		case "missing":
			missing++
		default:
			skipped++
		}
	}
	return registered, existing, missing, skipped, nil
}

func registerDiscoveredLegacyFile(ctx context.Context, db *sql.DB, opt options, group studyGroup, file legacyResourceFile) (string, error) {
	sourcePath, found, err := resolveLegacySourcePath(opt.legacyRoot, opt.legacyAssetsRoot, file.RelativePath)
	if err != nil {
		return "", err
	}
	if !found {
		fmt.Printf("missing legacy_file source=%s\n", filepath.ToSlash(file.RelativePath))
		return "missing", nil
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "skipped", nil
	}
	fileName := safeFileName(filepath.Base(file.RelativePath))
	checksum, err := fileChecksum(sourcePath)
	if err != nil {
		return "", err
	}
	category := firstNonEmpty(file.Category, legacyCategoryFromPath(file.RelativePath), "share")
	exists, err := discoveredAssetExists(ctx, db, group.ID, category, fileName, uint64(info.Size()), checksum)
	if err != nil {
		return "", err
	}
	if exists {
		return "existing", nil
	}

	resourceKey, err := randomHexKey()
	if err != nil {
		return "", err
	}
	storagePath := path.Join("team-"+group.Code+"-resources", "objects", resourceKey, fileName)
	if opt.dryRun {
		fmt.Printf("would register legacy_file group=%q source=%s target=%s category=%s\n",
			group.Name, filepath.ToSlash(file.RelativePath), storagePath, category)
		return "registered", nil
	}

	stored, err := copyResourceFile(opt.resourceRoot, storagePath, sourcePath)
	if err != nil {
		return "", err
	}
	assetID, err := createDiscoveredAsset(ctx, db, group, resourceKey, category, strings.TrimSuffix(fileName, filepath.Ext(fileName)), fileName, stored, file.RelativePath)
	if err != nil {
		if targetPath, pathErr := safeJoin(opt.resourceRoot, filepath.FromSlash(storagePath)); pathErr == nil {
			_ = os.Remove(targetPath)
		}
		return "", err
	}
	fmt.Printf("registered legacy_file asset_id=%d group=%q source=%s target=%s\n",
		assetID, group.Name, filepath.ToSlash(file.RelativePath), storagePath)
	return "registered", nil
}

func resolveLegacySourcePath(legacyRoot, legacyAssetsRoot, relativePath string) (string, bool, error) {
	type candidate struct {
		root     string
		relative string
	}
	candidates := []candidate{{root: legacyRoot, relative: relativePath}}
	normalized := filepath.ToSlash(relativePath)
	if strings.TrimSpace(legacyAssetsRoot) != "" {
		if strings.HasPrefix(normalized, "data/assets/") {
			candidates = append(candidates, candidate{root: legacyAssetsRoot, relative: strings.TrimPrefix(normalized, "data/assets/")})
		} else {
			candidates = append(candidates, candidate{root: legacyAssetsRoot, relative: relativePath})
		}
	} else if !strings.HasPrefix(normalized, "data/assets/") {
		candidates = append(candidates, candidate{root: legacyRoot, relative: filepath.Join("data", "assets", relativePath)})
	}
	for _, candidate := range candidates {
		sourcePath, err := safeJoin(candidate.root, candidate.relative)
		if err != nil {
			return "", false, err
		}
		info, err := os.Stat(sourcePath)
		if err == nil {
			if info.IsDir() {
				return "", false, errors.New("source path is a directory")
			}
			return sourcePath, true, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}
	}
	return "", false, nil
}

func discoveredAssetExists(ctx context.Context, db *sql.DB, groupID uint64, category, originalName string, fileSize uint64, checksum string) (bool, error) {
	var id uint64
	err := db.QueryRowContext(ctx, `SELECT id FROM assets
		WHERE group_id=? AND category=? AND original_name=? AND file_size=? AND checksum_sha256=?
		  AND storage_path LIKE 'team-%-resources/objects/%'
		ORDER BY id LIMIT 1`, groupID, category, originalName, fileSize, checksum).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return id > 0, nil
}

func createDiscoveredAsset(ctx context.Context, db *sql.DB, group studyGroup, resourceKey, category, title, fileName string, stored storedObject, relativeSource string) (uint64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO assets
		(group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		group.ID, category, title, fileName, stored.StoragePath, stored.MimeType, stored.FileSize, stored.ChecksumSHA256, shareScopeAllGroups, 1, now, now)
	if err != nil {
		return 0, err
	}
	assetID, err := res.LastInsertId()
	if err != nil || assetID <= 0 {
		return 0, errors.New("invalid_insert_id")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
		(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES (?,?,?,?,NULL,NULL,NULL,?,?)`,
		uint64(assetID), group.ID, resourceKey, assetKindOwned, now, now); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_share_grants
		(asset_id,owner_group_id,consumer_group_id,permission,status,created_by,created_at,revoked_by,revoked_at)
		VALUES (?,?,NULL,'import','active',1,?,NULL,NULL)
		ON DUPLICATE KEY UPDATE status='active',revoked_by=NULL,revoked_at=NULL`,
		uint64(assetID), group.ID, now); err != nil {
		return 0, err
	}
	if err := linkTasksForAsset(ctx, tx, group.ID, uint64(assetID), relativeSource, relativeSource, category, now); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(assetID), nil
}

func randomHexKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate resource key: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func legacyRelativePath(storagePath string) (string, bool, error) {
	value := strings.TrimSpace(storagePath)
	if value == "" {
		return "", false, nil
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "/api/") {
		return "", false, nil
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", false, err
	}
	decoded = strings.TrimPrefix(decoded, "/")
	decoded = filepath.FromSlash(decoded)
	if decoded == "" || filepath.IsAbs(decoded) || !filepath.IsLocal(decoded) {
		return "", false, errors.New("invalid relative source path")
	}
	clean := filepath.Clean(decoded)
	if clean == "." {
		return "", false, nil
	}
	return clean, true, nil
}

func safeJoin(root, relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) || !filepath.IsLocal(relativePath) {
		return "", errors.New("invalid relative path")
	}
	base, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	full := filepath.Join(base, relativePath)
	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("path escapes root")
	}
	return full, nil
}

func copyResourceFile(resourceRoot, storagePath, sourcePath string) (storedObject, error) {
	targetPath, err := safeJoin(resourceRoot, filepath.FromSlash(storagePath))
	if err != nil {
		return storedObject{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o750); err != nil {
		return storedObject{}, err
	}

	src, err := os.Open(sourcePath)
	if err != nil {
		return storedObject{}, err
	}
	defer src.Close()

	hasher := sha256.New()
	temp, err := os.CreateTemp(filepath.Dir(targetPath), ".resource-*")
	if err != nil {
		return storedObject{}, err
	}
	tempPath := temp.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tempPath)
		}
	}()

	size, copyErr := io.Copy(temp, io.TeeReader(src, hasher))
	closeErr := temp.Close()
	if copyErr != nil {
		return storedObject{}, copyErr
	}
	if closeErr != nil {
		return storedObject{}, closeErr
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))

	if existingInfo, err := os.Stat(targetPath); err == nil {
		if existingInfo.IsDir() {
			return storedObject{}, errors.New("target path is a directory")
		}
		existingChecksum, err := fileChecksum(targetPath)
		if err != nil {
			return storedObject{}, err
		}
		if existingChecksum != checksum {
			return storedObject{}, errors.New("target file exists with different checksum")
		}
		_ = os.Remove(tempPath)
		keepTemp = true
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tempPath, targetPath); err != nil {
			return storedObject{}, err
		}
		keepTemp = true
	} else {
		return storedObject{}, err
	}

	return storedObject{
		StoragePath:    filepath.ToSlash(storagePath),
		FileSize:       uint64(size),
		ChecksumSHA256: checksum,
		MimeType:       mime.TypeByExtension(filepath.Ext(targetPath)),
	}, nil
}

func fileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func updateAsset(ctx context.Context, db *sql.DB, group studyGroup, asset legacyAsset, resourceKey, category, fileName string, stored storedObject, relativeSource string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE assets
		SET category=?,original_name=?,storage_path=?,mime_type=?,file_size=?,checksum_sha256=?,visibility=?,updated_at=?
		WHERE id=? AND group_id=?`,
		category, fileName, stored.StoragePath, firstNonEmpty(stored.MimeType, asset.MimeType), stored.FileSize, stored.ChecksumSHA256, shareScopeAllGroups, now, asset.ID, group.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
		(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES (?,?,?,?,NULL,NULL,NULL,?,?)
		ON DUPLICATE KEY UPDATE group_id=VALUES(group_id),resource_key=VALUES(resource_key),asset_kind=VALUES(asset_kind),
			source_asset_id=NULL,imported_at=NULL,deleted_at=NULL,updated_at=VALUES(updated_at)`,
		asset.ID, group.ID, resourceKey, assetKindOwned, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_share_grants
		(asset_id,owner_group_id,consumer_group_id,permission,status,created_by,created_at,revoked_by,revoked_at)
		VALUES (?,?,NULL,'import','active',?,?,NULL,NULL)
		ON DUPLICATE KEY UPDATE status='active',revoked_by=NULL,revoked_at=NULL`,
		asset.ID, group.ID, firstNonZero(asset.CreatedBy, 1), now); err != nil {
		return err
	}
	if err := linkTasksForAsset(ctx, tx, group.ID, asset.ID, asset.StoragePath, relativeSource, category, now); err != nil {
		return err
	}
	return tx.Commit()
}

func linkTasksForAsset(ctx context.Context, tx *sql.Tx, groupID, assetID uint64, oldStoragePath, relativeSource, category string, now time.Time) error {
	rows, err := tx.QueryContext(ctx, `SELECT id,content FROM study_tasks WHERE group_id=? AND COALESCE(content,'')<>''`, groupID)
	if err != nil {
		return err
	}
	defer rows.Close()

	assetKey := normalizedLegacyKey(oldStoragePath)
	relativeKey := normalizedLegacyKey(relativeSource)
	var taskIDs []uint64
	for rows.Next() {
		var taskID uint64
		var content string
		if err := rows.Scan(&taskID, &content); err != nil {
			return err
		}
		contentKey := normalizedLegacyKey(content)
		if contentKey == "" || (contentKey != assetKey && contentKey != relativeKey) {
			continue
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, taskID := range taskIDs {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO task_assets
			(group_id,task_id,asset_id,usage_type,sort_order,created_at)
			VALUES (?,?,?,?,0,?)`, groupID, taskID, assetID, usageTypeForCategory(category), now); err != nil {
			return err
		}
	}
	return nil
}

func repairTaskAssetLinks(ctx context.Context, db *sql.DB, groupID uint64, dryRun bool) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,task_type,title,COALESCE(content,'') FROM study_tasks
		WHERE group_id=?`, groupID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type taskContent struct {
		ID       uint64
		TaskType string
		Title    string
		Content  string
	}
	var tasks []taskContent
	for rows.Next() {
		var task taskContent
		if err := rows.Scan(&task.ID, &task.TaskType, &task.Title, &task.Content); err != nil {
			return 0, err
		}
		if normalizedLegacyKey(task.Content) != "" || task.TaskType == "weekly_book" {
			tasks = append(tasks, task)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(tasks) == 0 {
		return 0, nil
	}

	repaired := 0
	for _, task := range tasks {
		assetID, category, err := findAssetForTaskContent(ctx, db, groupID, task.Content)
		if err != nil {
			return 0, err
		}
		if assetID == 0 && task.TaskType == "weekly_book" {
			hasLinks, err := taskHasAssetLinks(ctx, db, groupID, task.ID)
			if err != nil {
				return 0, err
			}
			if hasLinks {
				continue
			}
			assetID, category, err = findAssetForHistoricalReading(ctx, db, groupID, task.Title, task.Content)
			if err != nil {
				return 0, err
			}
		}
		if assetID == 0 {
			continue
		}
		usageType := usageTypeForTask(task.TaskType, category)
		if dryRun {
			exists, err := taskAssetLinkExists(ctx, db, groupID, task.ID, assetID, usageType)
			if err != nil {
				return 0, err
			}
			if !exists {
				repaired++
			}
			continue
		}
		res, err := db.ExecContext(ctx, `INSERT IGNORE INTO task_assets
			(group_id,task_id,asset_id,usage_type,sort_order,created_at)
			VALUES (?,?,?,?,0,?)`, groupID, task.ID, assetID, usageType, time.Now().UTC())
		if err != nil {
			return 0, err
		}
		if affected, _ := res.RowsAffected(); affected > 0 {
			repaired++
		}
	}
	return repaired, nil
}

func repairLearningConfigPaths(ctx context.Context, db *sql.DB, groupID uint64, dryRun bool) (int, error) {
	var raw sql.NullString
	err := db.QueryRowContext(ctx, `SELECT settings FROM group_settings WHERE group_id=?`, groupID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) || !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw.String), &settings); err != nil {
		return 0, fmt.Errorf("parse learning settings: %w", err)
	}
	if settings == nil {
		return 0, nil
	}
	repaired, err := repairSettingsDailyPaths(ctx, db, groupID, settings)
	if err != nil || repaired == 0 || dryRun {
		return repaired, err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return 0, fmt.Errorf("marshal learning settings: %w", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE group_settings SET settings=?,updated_at=? WHERE group_id=?`, string(payload), time.Now().UTC(), groupID)
	return repaired, err
}

func repairSettingsDailyPaths(ctx context.Context, db *sql.DB, groupID uint64, settings map[string]any) (int, error) {
	daily, ok := nestedSettingsMap(settings, "task_sections", "daily")
	if !ok {
		return 0, nil
	}
	repaired := 0
	dailyPath, dailyOK, changed, err := repairSettingPathValue(ctx, db, groupID, daily["path"])
	if err != nil {
		return 0, err
	}
	if dailyOK {
		daily["path"] = dailyPath
		if changed {
			repaired++
		}
	} else if shouldDropConfigSettingPath(daily["path"]) {
		delete(daily, "path")
		repaired++
	}

	devotion, ok := nestedSettingsMap(daily, "devotion")
	if !ok {
		return repaired, nil
	}
	devotionPath, devotionOK, changed, err := repairSettingPathValue(ctx, db, groupID, devotion["path"])
	if err != nil {
		return 0, err
	}
	switch {
	case devotionOK:
		devotion["path"] = devotionPath
		if changed {
			repaired++
		}
	case dailyOK && shouldDropConfigSettingPath(devotion["path"]):
		devotion["path"] = dailyPath
		repaired++
	case shouldDropConfigSettingPath(devotion["path"]):
		delete(devotion, "path")
		repaired++
	}
	return repaired, nil
}

func repairSettingPathValue(ctx context.Context, db *sql.DB, groupID uint64, value any) (string, bool, bool, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false, false, nil
	}
	assetID, err := findAssetForConfigPath(ctx, db, groupID, text)
	if err != nil {
		return "", false, false, err
	}
	if assetID == 0 {
		if shouldDropConfigSettingPath(text) {
			return "", false, true, nil
		}
		return "", false, false, nil
	}
	downloadURL := assetDownloadURL(assetID)
	return downloadURL, true, strings.TrimSpace(text) != downloadURL, nil
}

func nestedSettingsMap(root map[string]any, path ...string) (map[string]any, bool) {
	current := root
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func findAssetForConfigPath(ctx context.Context, db *sql.DB, groupID uint64, value string) (uint64, error) {
	if assetID := assetIDFromDownloadURL(value); assetID > 0 {
		exists, err := activeAssetExists(ctx, db, groupID, assetID)
		if err != nil || !exists {
			return 0, err
		}
		return assetID, nil
	}
	fileName := configResourceFileName(value)
	if fileName == "" {
		return 0, nil
	}
	preferredCategory := configResourceCategory(value, fileName)
	var assetID uint64
	err := db.QueryRowContext(ctx, `SELECT a.id
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.storage_path LIKE ?
		  AND (a.original_name=? OR SUBSTRING_INDEX(a.storage_path,'/',-1)=?)
		ORDER BY CASE WHEN a.category=? THEN 0 ELSE 1 END,a.id
		LIMIT 1`, groupID, newResourceStorageSQLPattern, fileName, fileName, preferredCategory).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return assetID, err
}

func activeAssetExists(ctx context.Context, db *sql.DB, groupID, assetID uint64) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.id=? AND a.group_id=? AND a.storage_path LIKE ?
		LIMIT 1`, assetID, groupID, newResourceStorageSQLPattern).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func assetDownloadURL(assetID uint64) string {
	if assetID == 0 {
		return ""
	}
	return fmt.Sprintf("/api/assets/%d/download", assetID)
}

func assetIDFromDownloadURL(value string) uint64 {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0
	}
	pathValue := text
	if parsed, err := url.Parse(text); err == nil && parsed.Path != "" {
		pathValue = parsed.Path
	}
	rest, ok := strings.CutPrefix(pathValue, "/api/assets/")
	if !ok {
		return 0
	}
	idText, ok := strings.CutSuffix(rest, "/download")
	if !ok || idText == "" {
		return 0
	}
	assetID, err := strconv.ParseUint(idText, 10, 64)
	if err != nil {
		return 0
	}
	return assetID
}

func configResourceFileName(value string) string {
	text := strings.TrimSpace(value)
	if text == "" || assetIDFromDownloadURL(text) > 0 {
		return ""
	}
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return ""
		}
		text = parsed.Path
	}
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	name := path.Base(strings.ReplaceAll(text, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		return ""
	}
	return name
}

func configResourceCategory(value, fileName string) string {
	switch strings.ToLower(path.Ext(fileName)) {
	case ".md", ".markdown":
		return "markdown"
	case ".mp4", ".m4v", ".mov", ".webm":
		return "video"
	}
	if relative, ok, err := legacyRelativePath(value); err == nil && ok {
		return legacyCategoryFromPath(relative)
	}
	return ""
}

func shouldDropConfigSettingPath(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if assetIDFromDownloadURL(text) > 0 {
		return true
	}
	if parsed, err := url.Parse(text); err == nil && parsed.Scheme != "" {
		return false
	}
	return true
}

func taskAssetLinkExists(ctx context.Context, db *sql.DB, groupID, taskID, assetID uint64, usageType string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM task_assets
		WHERE group_id=? AND task_id=? AND asset_id=? AND usage_type=?
		LIMIT 1`, groupID, taskID, assetID, usageType).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func findAssetForTaskContent(ctx context.Context, db *sql.DB, groupID uint64, content string) (uint64, string, error) {
	relative, ok, err := legacyRelativePath(content)
	if err != nil || !ok {
		return 0, "", err
	}
	fileName := filepath.Base(relative)
	category := legacyCategoryFromPath(relative)
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.category FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.original_name=? AND a.storage_path LIKE 'team-%-resources/objects/%'
		ORDER BY CASE WHEN category=? THEN 0 ELSE 1 END,
		         CASE WHEN LOWER(TRIM(title))=LOWER(TRIM(REGEXP_REPLACE(original_name,'\\.[^.]+$',''))) THEN 0 ELSE 1 END,
		         CASE WHEN title REGEXP '[0-9]{1,4}[[:space:]]*([-~—–至到][[:space:]]*[0-9]{1,4})?[[:space:]]*页' THEN 1 ELSE 0 END,
		         id LIMIT 1`, groupID, fileName, category)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, "", rows.Err()
	}
	var assetID uint64
	var assetCategory string
	if err := rows.Scan(&assetID, &assetCategory); err != nil {
		return 0, "", err
	}
	return assetID, assetCategory, rows.Err()
}

func taskHasAssetLinks(ctx context.Context, db *sql.DB, groupID, taskID uint64) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM task_assets
		WHERE group_id=? AND task_id=?
		LIMIT 1`, groupID, taskID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

type historicalReadingMetadata struct {
	BookName    string `json:"book_name"`
	SourceTitle string `json:"source_title"`
}

type historicalReadingAsset struct {
	ID             uint64
	Category       string
	Title          string
	OriginalName   string
	FileSize       uint64
	ChecksumSHA256 string
	AssetKind      string
	SourceAssetID  uint64
	Active         bool
}

func findAssetForHistoricalReading(ctx context.Context, db *sql.DB, groupID uint64, title, content string) (uint64, string, error) {
	refs := historicalReadingRefs(title, content)
	if len(refs) == 0 {
		return 0, "", nil
	}
	candidates, err := listHistoricalReadingAssets(ctx, db, groupID)
	if err != nil {
		return 0, "", err
	}
	return chooseHistoricalReadingAsset(refs, candidates)
}

func historicalReadingRefs(title, content string) []string {
	var refs []string
	add := func(value string) {
		if normalizeHistoricalReadingText(value) == "" {
			return
		}
		for _, existing := range refs {
			if normalizeHistoricalReadingText(existing) == normalizeHistoricalReadingText(value) {
				return
			}
		}
		refs = append(refs, strings.TrimSpace(value))
	}

	var metadata historicalReadingMetadata
	if strings.HasPrefix(strings.TrimSpace(content), "{") && json.Unmarshal([]byte(content), &metadata) == nil {
		add(metadata.SourceTitle)
		add(metadata.BookName)
	}
	add(title)
	return refs
}

func listHistoricalReadingAssets(ctx context.Context, db *sql.DB, groupID uint64) ([]historicalReadingAsset, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.category,a.title,a.original_name,a.file_size,a.checksum_sha256,
			COALESCE(b.asset_kind,''),COALESCE(b.source_asset_id,0),
			EXISTS(SELECT 1 FROM asset_bindings active_b
				WHERE active_b.asset_id=a.id AND active_b.group_id=a.group_id AND active_b.deleted_at IS NULL)
		FROM assets a
		LEFT JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
		WHERE a.group_id=? AND a.storage_path LIKE ? AND a.category IN ('book','passage','markdown')
		ORDER BY a.category,a.original_name,a.id`, groupID, newResourceStorageSQLPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []historicalReadingAsset
	for rows.Next() {
		var item historicalReadingAsset
		var active int
		if err := rows.Scan(
			&item.ID,
			&item.Category,
			&item.Title,
			&item.OriginalName,
			&item.FileSize,
			&item.ChecksumSHA256,
			&item.AssetKind,
			&item.SourceAssetID,
			&active,
		); err != nil {
			return nil, err
		}
		item.Active = active > 0
		candidates = append(candidates, item)
	}
	return candidates, rows.Err()
}

func chooseHistoricalReadingAsset(refs []string, candidates []historicalReadingAsset) (uint64, string, error) {
	type scoredAsset struct {
		ID       uint64
		Category string
		Score    int
	}
	bestByCanonical := map[uint64]scoredAsset{}
	for _, candidate := range candidates {
		canonicalID, category, ok := canonicalHistoricalReadingAsset(candidate, candidates)
		if !ok {
			continue
		}
		score := historicalReadingAssetMatchScore(refs, candidate)
		if score == 0 {
			continue
		}
		if current, ok := bestByCanonical[canonicalID]; !ok || score > current.Score {
			bestByCanonical[canonicalID] = scoredAsset{ID: canonicalID, Category: category, Score: score}
		}
	}

	var best scoredAsset
	for _, item := range bestByCanonical {
		if item.Score > best.Score {
			best = item
		}
	}
	if best.Score == 0 {
		return 0, "", nil
	}
	for _, item := range bestByCanonical {
		if item.Score == best.Score && item.ID != best.ID {
			return 0, "", nil
		}
	}
	return best.ID, best.Category, nil
}

func canonicalHistoricalReadingAsset(candidate historicalReadingAsset, candidates []historicalReadingAsset) (uint64, string, bool) {
	key := historicalReadingAssetDedupeKey(candidate)
	if key == "" {
		if candidate.Active {
			return candidate.ID, candidate.Category, true
		}
		return 0, "", false
	}

	var canonical historicalReadingAsset
	found := false
	for _, item := range candidates {
		if !item.Active || historicalReadingAssetDedupeKey(item) != key {
			continue
		}
		if !found || preferCleanupAsset(toCleanupAssetCandidate(item), toCleanupAssetCandidate(canonical)) {
			canonical = item
			found = true
		}
	}
	if !found {
		return 0, "", false
	}
	return canonical.ID, canonical.Category, true
}

func historicalReadingAssetDedupeKey(item historicalReadingAsset) string {
	return cleanupAssetDedupeKey(toCleanupAssetCandidate(item))
}

func toCleanupAssetCandidate(item historicalReadingAsset) cleanupAssetCandidate {
	return cleanupAssetCandidate{
		ID:             item.ID,
		Category:       item.Category,
		Title:          item.Title,
		OriginalName:   item.OriginalName,
		FileSize:       item.FileSize,
		ChecksumSHA256: item.ChecksumSHA256,
		AssetKind:      item.AssetKind,
		SourceAssetID:  item.SourceAssetID,
	}
}

func historicalReadingAssetMatchScore(refs []string, candidate historicalReadingAsset) int {
	best := 0
	for _, ref := range refs {
		refKey := normalizeHistoricalReadingText(ref)
		if refKey == "" {
			continue
		}
		for _, candidateValue := range []string{candidate.Title, candidate.OriginalName} {
			score := historicalReadingTextMatchScore(refKey, normalizeHistoricalReadingText(candidateValue))
			if score > best {
				best = score
			}
		}
	}
	return best
}

func historicalReadingTextMatchScore(refKey, candidateKey string) int {
	if refKey == "" || candidateKey == "" {
		return 0
	}
	switch {
	case refKey == candidateKey:
		return 100
	case strings.Contains(refKey, candidateKey) && len([]rune(candidateKey)) >= 4:
		return 90
	case strings.Contains(candidateKey, refKey) && len([]rune(refKey)) >= 4:
		return 80
	default:
		return 0
	}
}

func normalizeHistoricalReadingText(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if decoded, err := url.PathUnescape(text); err == nil {
		text = decoded
	}
	text = path.Base(strings.ReplaceAll(text, "\\", "/"))
	text = strings.TrimSuffix(text, path.Ext(text))
	text = taskScopedTitleRegexp.ReplaceAllString(text, "")

	var builder strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		}
	}
	return builder.String()
}

type cleanupAssetCandidate struct {
	ID             uint64
	Category       string
	Title          string
	OriginalName   string
	FileSize       uint64
	ChecksumSHA256 string
	AssetKind      string
	SourceAssetID  uint64
}

type duplicateAssetBinding struct {
	AssetID          uint64
	CanonicalAssetID uint64
	Category         string
}

func cleanupDuplicateResourceBindings(ctx context.Context, db *sql.DB, groupID uint64, dryRun bool) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.category,a.title,a.original_name,a.file_size,a.checksum_sha256,b.asset_kind,COALESCE(b.source_asset_id,0)
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.storage_path LIKE 'team-%-resources/objects/%'
		  AND a.original_name<>'' AND a.checksum_sha256<>'' AND a.file_size>0
		ORDER BY a.category,a.original_name,a.checksum_sha256,a.id`, groupID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	groups := map[string][]cleanupAssetCandidate{}
	for rows.Next() {
		var item cleanupAssetCandidate
		if err := rows.Scan(
			&item.ID,
			&item.Category,
			&item.Title,
			&item.OriginalName,
			&item.FileSize,
			&item.ChecksumSHA256,
			&item.AssetKind,
			&item.SourceAssetID,
		); err != nil {
			return 0, err
		}
		key := cleanupAssetDedupeKey(item)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var duplicates []duplicateAssetBinding
	for _, items := range groups {
		if len(items) < 2 {
			continue
		}
		canonical := items[0]
		for _, item := range items[1:] {
			if preferCleanupAsset(item, canonical) {
				canonical = item
			}
		}
		for _, item := range items {
			if item.ID == canonical.ID {
				continue
			}
			duplicates = append(duplicates, duplicateAssetBinding{
				AssetID:          item.ID,
				CanonicalAssetID: canonical.ID,
				Category:         canonical.Category,
			})
		}
	}
	if len(duplicates) == 0 || dryRun {
		return len(duplicates), nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, duplicate := range duplicates {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO task_assets
			(group_id,task_id,asset_id,usage_type,sort_order,created_at)
			SELECT group_id,task_id,?,usage_type,sort_order,created_at
			FROM task_assets
			WHERE group_id=? AND asset_id=?`, duplicate.CanonicalAssetID, groupID, duplicate.AssetID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM task_assets WHERE group_id=? AND asset_id=?`, groupID, duplicate.AssetID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE asset_dependencies
			SET status='removed',updated_at=?
			WHERE status='active' AND (consumer_asset_id=? OR provider_asset_id=?)`,
			now, duplicate.AssetID, duplicate.AssetID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE asset_share_grants
			SET status='revoked',revoked_at=COALESCE(revoked_at,?)
			WHERE asset_id=? AND status='active'`, now, duplicate.AssetID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE asset_bindings
			SET deleted_at=COALESCE(deleted_at,?),updated_at=?
			WHERE group_id=? AND asset_id=? AND deleted_at IS NULL`,
			now, now, groupID, duplicate.AssetID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(duplicates), nil
}

func repairTaskScopedAssetTitles(ctx context.Context, db *sql.DB, groupID uint64, dryRun bool) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT a.id,a.category,a.title,a.original_name
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.storage_path LIKE ? AND a.original_name<>''`,
		groupID, newResourceStorageSQLPattern)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type titleRepair struct {
		ID    uint64
		Title string
	}
	var repairs []titleRepair
	for rows.Next() {
		var id uint64
		var category, title, originalName string
		if err := rows.Scan(&id, &category, &title, &originalName); err != nil {
			return 0, err
		}
		if repaired, ok := canonicalAssetTitle(category, title, originalName); ok {
			repairs = append(repairs, titleRepair{ID: id, Title: repaired})
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(repairs) == 0 || dryRun {
		return len(repairs), nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, repair := range repairs {
		if _, err := tx.ExecContext(ctx, `UPDATE assets SET title=?,updated_at=? WHERE id=? AND group_id=?`,
			repair.Title, now, repair.ID, groupID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(repairs), nil
}

func canonicalAssetTitle(category, title, originalName string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "book", "passage":
	default:
		return "", false
	}
	if !taskScopedTitleRegexp.MatchString(title) {
		return "", false
	}
	baseTitle := strings.TrimSpace(strings.TrimSuffix(filepath.Base(originalName), filepath.Ext(originalName)))
	if baseTitle == "" || normalizeCleanupAssetTitle(baseTitle) == normalizeCleanupAssetTitle(title) {
		return "", false
	}
	return baseTitle, true
}

func cleanupAssetDedupeKey(item cleanupAssetCandidate) string {
	category := strings.TrimSpace(strings.ToLower(item.Category))
	originalName := strings.TrimSpace(strings.ToLower(item.OriginalName))
	checksum := strings.TrimSpace(strings.ToLower(item.ChecksumSHA256))
	if category == "" || originalName == "" || checksum == "" || item.FileSize == 0 {
		return ""
	}
	return category + "\x00" + originalName + "\x00" + checksum
}

func preferCleanupAsset(candidate, current cleanupAssetCandidate) bool {
	candidateScore := cleanupAssetScore(candidate)
	currentScore := cleanupAssetScore(current)
	if candidateScore != currentScore {
		return candidateScore > currentScore
	}
	return candidate.ID < current.ID
}

func cleanupAssetScore(item cleanupAssetCandidate) int {
	score := 0
	baseTitle := strings.TrimSuffix(filepath.Base(item.OriginalName), filepath.Ext(item.OriginalName))
	if normalizeCleanupAssetTitle(item.Title) == normalizeCleanupAssetTitle(baseTitle) {
		score += 100
	}
	if !taskScopedTitleRegexp.MatchString(item.Title) {
		score += 20
	}
	if item.AssetKind == assetKindOwned {
		score += 10
	}
	if item.SourceAssetID == 0 {
		score += 5
	}
	return score
}

func normalizeCleanupAssetTitle(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func legacyCategoryFromPath(relativePath string) string {
	first, _, _ := strings.Cut(filepath.ToSlash(relativePath), "/")
	switch strings.ToLower(first) {
	case "newtestament":
		return "video"
	case "ppt":
		return "handout"
	case "mentor":
		return "mentor"
	case "book":
		return "book"
	case "passage":
		return "passage"
	default:
		return ""
	}
}

func normalizedLegacyKey(value string) string {
	relative, ok, err := legacyRelativePath(value)
	if err != nil || !ok {
		return ""
	}
	return filepath.ToSlash(relative)
}

func usageTypeForTask(taskType, category string) string {
	switch taskType {
	case "weekly_video":
		return "video"
	case "weekly_outline":
		return "outline"
	}
	return usageTypeForCategory(category)
}

func usageTypeForCategory(category string) string {
	switch category {
	case "video":
		return "video"
	case "outline":
		return "outline"
	case "mentor", "handout", "share":
		return "share"
	default:
		return "reading"
	}
}

func migratedCategory(asset legacyAsset) string {
	text := strings.ToLower(asset.Category + " " + asset.Title + " " + asset.OriginalName + " " + asset.StoragePath)
	if strings.Contains(text, "mentor") ||
		strings.Contains(text, "导读") ||
		strings.Contains(text, "内容概要") ||
		strings.Contains(text, "圣经纵览的目的与价值") {
		return "mentor"
	}
	return asset.Category
}

func safeFileName(name string) string {
	value := filepath.Base(strings.TrimSpace(filepath.FromSlash(name)))
	value = strings.ReplaceAll(value, "\x00", "")
	if value == "." || value == string(filepath.Separator) || value == "" {
		return "resource"
	}
	return value
}

func isHexResourceKey(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZero(values ...uint64) uint64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
