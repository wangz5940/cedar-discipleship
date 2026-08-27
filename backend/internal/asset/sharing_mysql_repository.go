package asset

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	sharePermissionImport = "import"
	shareStatusActive     = "active"
	shareStatusRevoked    = "revoked"
	dependencyTypeImport  = "import"
	eventTypeImported     = "imported"
)

func (r *MySQLRepository) FindDownloadTarget(ctx context.Context, groupID, assetID uint64) (*Asset, error) {
	item, binding, err := r.assetWithBinding(ctx, r.db, groupID, assetID)
	if err != nil {
		return nil, err
	}
	if binding.AssetKind == AssetKindOwned {
		return item, nil
	}
	return r.importedSource(ctx, groupID, assetID)
}

func (r *MySQLRepository) ShareSettings(ctx context.Context, groupID, assetID uint64) (*SharingSettings, error) {
	if _, binding, err := r.assetWithBinding(ctx, r.db, groupID, assetID); err != nil {
		return nil, err
	} else if binding.AssetKind != AssetKindOwned {
		return nil, sql.ErrNoRows
	}
	rows, err := r.db.QueryContext(ctx, `SELECT consumer_group_id FROM asset_share_grants
		WHERE asset_id=? AND owner_group_id=? AND permission=? AND status=?
		ORDER BY consumer_group_id`, assetID, groupID, sharePermissionImport, shareStatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := &SharingSettings{AssetID: assetID, OwnerGroupID: groupID, Scope: ShareScopePrivate}
	for rows.Next() {
		var consumer sql.NullInt64
		if err := rows.Scan(&consumer); err != nil {
			return nil, err
		}
		if !consumer.Valid {
			settings.Scope = ShareScopeAllGroups
			settings.ConsumerGroupIDs = nil
			continue
		}
		if settings.Scope != ShareScopeAllGroups {
			settings.Scope = ShareScopeSelectedGroups
			settings.ConsumerGroupIDs = append(settings.ConsumerGroupIDs, uint64(consumer.Int64))
		}
	}
	return settings, rows.Err()
}

func (r *MySQLRepository) SaveShareSettings(ctx context.Context, groupID, assetID, actorID uint64, input ShareInput, at time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := r.saveShareSettingsTx(ctx, tx, groupID, assetID, actorID, input, at); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *MySQLRepository) BatchSaveShareSettings(ctx context.Context, groupID, actorID uint64, input BatchShareInput, at time.Time) (*BatchShareResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, assetID := range input.AssetIDs {
		if err := r.saveShareSettingsTx(ctx, tx, groupID, assetID, actorID, ShareInput{
			Scope:            input.Scope,
			ConsumerGroupIDs: input.ConsumerGroupIDs,
		}, at); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &BatchShareResult{AssetIDs: input.AssetIDs, Scope: input.Scope, Count: len(input.AssetIDs)}, nil
}

func (r *MySQLRepository) saveShareSettingsTx(ctx context.Context, tx *sql.Tx, groupID, assetID, actorID uint64, input ShareInput, at time.Time) error {
	if _, binding, err := r.assetWithBinding(ctx, tx, groupID, assetID); err != nil {
		return err
	} else if binding.AssetKind != AssetKindOwned {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `UPDATE asset_share_grants
		SET status=?,revoked_by=?,revoked_at=?
		WHERE asset_id=? AND owner_group_id=? AND permission=? AND status=?`,
		shareStatusRevoked, actorID, at, assetID, groupID, sharePermissionImport, shareStatusActive); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE assets SET visibility=?,updated_at=? WHERE id=? AND group_id=?`,
		string(input.Scope), at, assetID, groupID); err != nil {
		return err
	}
	switch input.Scope {
	case ShareScopePrivate:
	case ShareScopeAllGroups:
		if err := upsertShareGrant(ctx, tx, assetID, groupID, nil, actorID, at); err != nil {
			return err
		}
	case ShareScopeSelectedGroups:
		for _, consumerID := range input.ConsumerGroupIDs {
			id := consumerID
			if err := upsertShareGrant(ctx, tx, assetID, groupID, &id, actorID, at); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidShareScope
	}
	return nil
}

func (r *MySQLRepository) SharedResources(ctx context.Context, targetGroupID uint64, filter SharedFilter) ([]SharedResource, error) {
	query := `SELECT DISTINCT a.id,a.group_id,sg.code,sg.name,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256,a.updated_at,
	       COALESCE(imported.asset_id,0),imported.imported_at
	  FROM asset_share_grants g
	  JOIN assets a ON a.id=g.asset_id
	  JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.asset_kind=? AND b.deleted_at IS NULL
	  JOIN study_groups sg ON sg.id=a.group_id AND sg.status=1
	  LEFT JOIN asset_bindings imported
	    ON imported.group_id=? AND imported.source_asset_id=a.id AND imported.deleted_at IS NULL
	 WHERE g.permission=? AND g.status=? AND a.group_id<>?
	   AND (g.consumer_group_id IS NULL OR g.consumer_group_id=?)
	   AND a.storage_path LIKE ?`
	args := []any{AssetKindOwned, targetGroupID, sharePermissionImport, shareStatusActive, targetGroupID, targetGroupID, newResourceStorageSQLPattern}
	if filter.OwnerGroupID > 0 {
		query += " AND a.group_id=?"
		args = append(args, filter.OwnerGroupID)
	}
	if strings.TrimSpace(filter.Category) != "" {
		query += " AND a.category=?"
		args = append(args, strings.TrimSpace(filter.Category))
	}
	query += " ORDER BY sg.name,a.category,a.title,a.id"
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []SharedResource{}
	for rows.Next() {
		item, err := scanSharedResource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) ImportPreview(ctx context.Context, targetGroupID, sourceAssetID uint64) (*ImportPreview, error) {
	source, err := r.sharedSource(ctx, targetGroupID, sourceAssetID)
	if err != nil {
		return nil, err
	}
	preview := &ImportPreview{
		Allowed: true, SourceGroup: source.OwnerGroup, Resource: source,
		Permissions: []string{sharePermissionImport},
	}
	var importedAt sql.NullTime
	err = r.db.QueryRowContext(ctx, `SELECT asset_id,imported_at FROM asset_bindings
		WHERE group_id=? AND source_asset_id=? AND deleted_at IS NULL`,
		targetGroupID, sourceAssetID).Scan(&preview.Resource.ImportedAssetID, &importedAt)
	if err == nil {
		preview.Imported = true
		preview.Resource.Imported = true
		if importedAt.Valid {
			preview.ImportedAt = &importedAt.Time
			preview.Resource.ImportedAt = &importedAt.Time
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return preview, nil
}

func (r *MySQLRepository) Import(ctx context.Context, targetGroupID, actorID uint64, input ImportInput, at time.Time) (*Asset, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	importedAssetID, err := r.importTx(ctx, tx, targetGroupID, actorID, input.SourceAssetID, at, nil)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.FindByID(ctx, targetGroupID, importedAssetID)
}

func (r *MySQLRepository) BatchImport(ctx context.Context, targetGroupID, actorID uint64, input BatchImportInput, at time.Time) (*BatchImportResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	detail := `{"batch":true}`
	importedIDs := make([]uint64, 0, len(input.SourceAssetIDs))
	for _, sourceAssetID := range input.SourceAssetIDs {
		importedAssetID, err := r.importTx(ctx, tx, targetGroupID, actorID, sourceAssetID, at, detail)
		if err != nil {
			return nil, err
		}
		importedIDs = append(importedIDs, importedAssetID)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &BatchImportResult{
		SourceAssetIDs:   input.SourceAssetIDs,
		ImportedAssetIDs: importedIDs,
		Count:            len(importedIDs),
	}, nil
}

func (r *MySQLRepository) importTx(
	ctx context.Context,
	tx *sql.Tx,
	targetGroupID uint64,
	actorID uint64,
	sourceAssetID uint64,
	at time.Time,
	detail any,
) (uint64, error) {
	source, err := r.sharedSourceTx(ctx, tx, targetGroupID, sourceAssetID)
	if err != nil {
		return 0, err
	}
	var importedAssetID uint64
	var existing sql.NullInt64
	var deletedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT asset_id,deleted_at FROM asset_bindings
		WHERE group_id=? AND source_asset_id=? FOR UPDATE`,
		targetGroupID, sourceAssetID).Scan(&existing, &deletedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if existing.Valid && existing.Int64 > 0 {
		importedAssetID = uint64(existing.Int64)
		if _, err := tx.ExecContext(ctx, `UPDATE asset_bindings
			SET imported_at=COALESCE(imported_at,?),deleted_at=NULL,updated_at=?
			WHERE asset_id=? AND group_id=?`, at, at, importedAssetID, targetGroupID); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE assets
			SET category=?,title=?,original_name=?,storage_path=?,mime_type=?,file_size=?,checksum_sha256=?,updated_at=?
			WHERE id=? AND group_id=?`,
			source.Category, source.Title, source.OriginalName, source.StoragePath, source.MimeType,
			source.FileSize, source.ChecksumSHA256, at, importedAssetID, targetGroupID); err != nil {
			return 0, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `INSERT INTO assets
			(group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			targetGroupID, source.Category, source.Title, source.OriginalName, source.StoragePath, source.MimeType,
			source.FileSize, source.ChecksumSHA256, "imported", actorID, at, at)
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		if err != nil || id <= 0 {
			return 0, errors.New("invalid_import_insert_id")
		}
		importedAssetID = uint64(id)
		resourceKey, err := randomResourceKey()
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO asset_bindings
			(asset_id,group_id,resource_key,asset_kind,source_asset_id,imported_at,deleted_at,created_at,updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			importedAssetID, targetGroupID, resourceKey, AssetKindImported, sourceAssetID, at, nil, at, at); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_dependencies
		(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,dependency_type,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE status=VALUES(status),updated_at=VALUES(updated_at)`,
		targetGroupID, importedAssetID, source.OwnerGroup.ID, sourceAssetID, dependencyTypeImport, shareStatusActive, at, at); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO asset_import_events
		(target_group_id,imported_asset_id,source_asset_id,event_type,actor_user_id,detail,created_at)
		VALUES (?,?,?,?,?,?,?)`,
		targetGroupID, importedAssetID, sourceAssetID, eventTypeImported, actorID, detail, at); err != nil {
		return 0, err
	}
	return importedAssetID, nil
}

func (r *MySQLRepository) ImportHistory(ctx context.Context, groupID uint64, limit int) ([]ImportEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,target_group_id,imported_asset_id,source_asset_id,event_type,actor_user_id,COALESCE(CAST(detail AS CHAR),''),created_at
		FROM asset_import_events WHERE target_group_id=? ORDER BY id DESC LIMIT ?`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ImportEvent{}
	for rows.Next() {
		var item ImportEvent
		if err := rows.Scan(&item.ID, &item.TargetGroupID, &item.ImportedAssetID, &item.SourceAssetID, &item.EventType, &item.ActorUserID, &item.Detail, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *MySQLRepository) assetWithBinding(ctx context.Context, q queryer, groupID, assetID uint64) (*Asset, Binding, error) {
	var item Asset
	var binding Binding
	var source sql.NullInt64
	var imported sql.NullTime
	err := q.QueryRowContext(ctx, `SELECT a.id,a.group_id,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256,a.visibility,
	       b.resource_key,b.asset_kind,b.source_asset_id,b.imported_at,a.created_at,a.updated_at
	  FROM assets a JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
	 WHERE a.id=? AND a.group_id=? AND b.deleted_at IS NULL AND a.storage_path LIKE ?`, assetID, groupID, newResourceStorageSQLPattern).Scan(
		&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath,
		&item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility,
		&binding.ResourceKey, &binding.AssetKind, &source, &imported, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, Binding{}, err
	}
	binding.AssetID, binding.GroupID = item.ID, item.GroupID
	if source.Valid && source.Int64 > 0 {
		binding.SourceAssetID = uint64(source.Int64)
	}
	if imported.Valid {
		binding.ImportedAt = &imported.Time
	}
	item.ResourceKey = binding.ResourceKey
	item.AssetKind = binding.AssetKind
	item.SourceAssetID = binding.SourceAssetID
	item.ImportedAt = binding.ImportedAt
	return &item, binding, nil
}

func (r *MySQLRepository) importedSource(ctx context.Context, groupID, importedAssetID uint64) (*Asset, error) {
	var item Asset
	err := r.db.QueryRowContext(ctx, `SELECT source.id,source.group_id,source.category,source.title,source.original_name,
	       source.storage_path,source.mime_type,source.file_size,source.checksum_sha256,source.visibility,source.created_at,source.updated_at
	  FROM asset_dependencies d
	  JOIN assets source ON source.id=d.provider_asset_id
	  JOIN asset_bindings source_b ON source_b.asset_id=source.id AND source_b.asset_kind=? AND source_b.deleted_at IS NULL
	  JOIN asset_share_grants g ON g.asset_id=source.id AND g.permission=? AND g.status=?
	   AND (g.consumer_group_id IS NULL OR g.consumer_group_id=d.consumer_group_id)
	 WHERE d.consumer_group_id=? AND d.consumer_asset_id=? AND d.status=? AND source.storage_path LIKE ?`,
		AssetKindOwned, sharePermissionImport, shareStatusActive, groupID, importedAssetID, shareStatusActive, newResourceStorageSQLPattern).
		Scan(&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath,
			&item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility, &item.CreatedAt, &item.UpdatedAt)
	return &item, err
}

func upsertShareGrant(ctx context.Context, tx *sql.Tx, assetID, ownerGroupID uint64, consumerGroupID *uint64, actorID uint64, at time.Time) error {
	var consumer any
	if consumerGroupID != nil {
		consumer = *consumerGroupID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO asset_share_grants
		(asset_id,owner_group_id,consumer_group_id,permission,status,created_by,created_at,revoked_by,revoked_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE status=VALUES(status),created_by=VALUES(created_by),created_at=VALUES(created_at),revoked_by=NULL,revoked_at=NULL`,
		assetID, ownerGroupID, consumer, sharePermissionImport, shareStatusActive, actorID, at, nil, nil)
	return err
}

func (r *MySQLRepository) sharedSource(ctx context.Context, targetGroupID, sourceAssetID uint64) (SharedResource, error) {
	return r.sharedSourceTx(ctx, nil, targetGroupID, sourceAssetID)
}

func (r *MySQLRepository) sharedSourceTx(ctx context.Context, tx *sql.Tx, targetGroupID, sourceAssetID uint64) (SharedResource, error) {
	q := querySource(r.db, tx)
	row := q.QueryRowContext(ctx, `SELECT a.id,a.group_id,sg.code,sg.name,a.category,a.title,a.original_name,a.storage_path,a.mime_type,a.file_size,a.checksum_sha256,a.updated_at,
	       COALESCE(imported.asset_id,0),imported.imported_at
	  FROM assets a
	  JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id AND b.asset_kind=? AND b.deleted_at IS NULL
	  JOIN study_groups sg ON sg.id=a.group_id
	  JOIN asset_share_grants g ON g.asset_id=a.id
	  LEFT JOIN asset_bindings imported ON imported.group_id=? AND imported.source_asset_id=a.id AND imported.deleted_at IS NULL
	 WHERE g.permission=? AND g.status=? AND a.id=? AND a.group_id<>?
	   AND (g.consumer_group_id IS NULL OR g.consumer_group_id=?)
	   AND a.storage_path LIKE ? LIMIT 1`,
		AssetKindOwned, targetGroupID, sharePermissionImport, shareStatusActive, sourceAssetID, targetGroupID, targetGroupID, newResourceStorageSQLPattern)
	return scanSharedResource(row)
}

type rowScanner interface {
	Scan(...any) error
}

func scanSharedResource(row rowScanner) (SharedResource, error) {
	var item SharedResource
	var importedAssetID uint64
	var importedAt sql.NullTime
	if err := row.Scan(
		&item.AssetID, &item.OwnerGroup.ID, &item.OwnerGroup.Code, &item.OwnerGroup.Name,
		&item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.UpdatedAt,
		&importedAssetID, &importedAt,
	); err != nil {
		return SharedResource{}, err
	}
	if importedAssetID > 0 {
		item.Imported = true
		item.ImportedAssetID = importedAssetID
	}
	if importedAt.Valid {
		item.ImportedAt = &importedAt.Time
	}
	return item, nil
}

type sourceQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func querySource(db *sql.DB, tx *sql.Tx) sourceQuerier {
	if tx != nil {
		return tx
	}
	return db
}

func randomResourceKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate resource key: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
