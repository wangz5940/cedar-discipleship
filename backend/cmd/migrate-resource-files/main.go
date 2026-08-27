package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

const (
	shareScopeAllGroups = "all_groups"
	assetKindOwned      = "owned"
)

type options struct {
	dsn          string
	groupName    string
	groupCode    string
	legacyRoot   string
	resourceRoot string
	dryRun       bool
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

func main() {
	var opt options
	flag.StringVar(&opt.dsn, "dsn", "", "MySQL DSN")
	flag.StringVar(&opt.groupName, "group-name", "", "study group name from database")
	flag.StringVar(&opt.groupCode, "group-code", "", "study group code from database")
	flag.StringVar(&opt.legacyRoot, "legacy-root", ".", "root containing existing resource files")
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

	mode := "apply"
	if opt.dryRun {
		mode = "dry-run"
	}
	fmt.Printf("resource_file_migration mode=%s group_id=%d group_code=%s group_name=%q total=%d migrated=%d missing=%d skipped=%d\n",
		mode, group.ID, group.Code, group.Name, len(assets), migrated, missing, skipped)
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
	sourcePath, err := safeJoin(opt.legacyRoot, relativeSource)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("missing asset_id=%d source=%s\n", asset.ID, relativeSource)
			return "missing", nil
		}
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("source path is a directory")
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
	if err := updateAsset(ctx, db, group, asset, resourceKey, category, fileName, stored); err != nil {
		return "", err
	}
	fmt.Printf("migrated asset_id=%d group=%q source=%s target=%s\n", asset.ID, group.Name, relativeSource, storagePath)
	return "migrated", nil
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

func updateAsset(ctx context.Context, db *sql.DB, group studyGroup, asset legacyAsset, resourceKey, category, fileName string, stored storedObject) error {
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
	return tx.Commit()
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
