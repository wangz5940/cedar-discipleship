package asset

import (
	"context"
	"database/sql"
	"errors"
	pathpkg "path"
	"strings"
	"time"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) FindByID(ctx context.Context, groupID, id uint64) (*Asset, error) {
	var item Asset
	var source sql.NullInt64
	var imported sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT a.id,a.group_id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256,a.visibility,
	       b.resource_key,b.asset_kind,b.source_asset_id,b.imported_at,a.created_at,a.updated_at
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.id=? AND a.group_id=? AND a.storage_path LIKE ?`, id, groupID, newResourceStorageSQLPattern).
		Scan(&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility, &item.ResourceKey, &item.AssetKind, &source, &imported, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if source.Valid && source.Int64 > 0 {
		item.SourceAssetID = uint64(source.Int64)
	}
	if imported.Valid {
		item.ImportedAt = &imported.Time
	}
	return &item, nil
}

func (r *MySQLRepository) List(ctx context.Context, groupID uint64, limit int) ([]Asset, error) {
	query := `SELECT a.id,a.group_id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256,a.visibility,
	       b.resource_key,b.asset_kind,b.source_asset_id,b.imported_at,a.created_at,a.updated_at
		FROM assets a
		JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.deleted_at IS NULL
		WHERE a.group_id=? AND a.storage_path LIKE ? ORDER BY a.category,a.title,a.id`
	args := []any{groupID, newResourceStorageSQLPattern}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Asset
	for rows.Next() {
		var item Asset
		var source sql.NullInt64
		var imported sql.NullTime
		if err := rows.Scan(&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility, &item.ResourceKey, &item.AssetKind, &source, &imported, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if source.Valid && source.Int64 > 0 {
			item.SourceAssetID = uint64(source.Int64)
		}
		if imported.Valid {
			item.ImportedAt = &imported.Time
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) Create(ctx context.Context, item *Asset, actorID uint64) (uint64, error) {
	at := time.Now().UTC()
	now := at.Format("2006-01-02 15:04:05.000")
	visibility := firstNonEmpty(item.Visibility, string(ShareScopeAllGroups))
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO assets (group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.GroupID, item.Category, item.Title, item.OriginalName, item.StoragePath, item.MimeType, item.FileSize, item.ChecksumSHA256, visibility, actorID, now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, errors.New("invalid_insert_id")
	}
	assetID := uint64(id)
	resourceKey := item.ResourceKey
	if resourceKey == "" {
		resourceKey, err = randomResourceKey()
		if err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
		(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		assetID, item.GroupID, resourceKey, AssetKindOwned, nil, nil, nil, now, now); err != nil {
		return 0, err
	}
	if visibility == string(ShareScopeAllGroups) {
		if err := upsertShareGrant(ctx, tx, assetID, item.GroupID, nil, actorID, at); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return assetID, nil
}

func (r *MySQLRepository) Delete(ctx context.Context, groupID, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM assets WHERE id=? AND group_id=?`, id, groupID)
	return err
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}

const newResourceStorageSQLPattern = "team-%-resources/objects/%"

func isNewResourceStoragePath(storagePath string) bool {
	value := strings.TrimSpace(strings.ReplaceAll(storagePath, "\\", "/"))
	if value == "" || strings.HasPrefix(value, "/") {
		return false
	}
	clean := pathpkg.Clean(value)
	if clean != value {
		return false
	}
	parts := strings.Split(clean, "/")
	if len(parts) != 4 {
		return false
	}
	if !strings.HasPrefix(parts[0], "team-") || !strings.HasSuffix(parts[0], "-resources") || parts[0] == "team--resources" {
		return false
	}
	if parts[1] != "objects" || !isHexResourceKey(parts[2]) || parts[3] == "" {
		return false
	}
	return true
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
