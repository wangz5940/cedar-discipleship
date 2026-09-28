package asset

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RestoreReferenceTx resolves a backup path using server-owned registrations.
// Existing references retain their permissions, including revoked source access.
func (r *MySQLRepository) RestoreReferenceTx(
	ctx context.Context,
	tx *sql.Tx,
	groupID, actorID uint64,
	storagePath string,
	at time.Time,
) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, `SELECT a.id
		FROM assets a JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
		WHERE a.group_id=? AND BINARY a.storage_path=BINARY ? AND b.deleted_at IS NULL
		ORDER BY a.id LIMIT 1 FOR UPDATE`, groupID, storagePath).Scan(&id)
	if err == nil || !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	// A backup is not proof of ownership. New references must pass the same
	// source-grant validation as a normal resource import.
	err = tx.QueryRowContext(ctx, `SELECT a.id
		FROM assets a JOIN asset_bindings b ON b.asset_id=a.id AND b.group_id=a.group_id
		JOIN study_groups owner_group ON owner_group.id=a.group_id
		JOIN study_groups target_group ON target_group.id=? AND target_group.tenant_id=owner_group.tenant_id
		WHERE BINARY a.storage_path=BINARY ? AND b.asset_kind='owned' AND b.deleted_at IS NULL
		ORDER BY a.id LIMIT 1`, groupID, storagePath).Scan(&id)
	if err != nil {
		return 0, err
	}
	return r.importTx(ctx, tx, groupID, actorID, id, at, nil)
}
