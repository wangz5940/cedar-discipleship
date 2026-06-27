package asset

import (
	"context"
	"database/sql"
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
	err := r.db.QueryRowContext(ctx, `SELECT id,group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility
		FROM assets WHERE id=? AND group_id=?`, id, groupID).
		Scan(&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) List(ctx context.Context, groupID uint64, limit int) ([]Asset, error) {
	query := `SELECT id,group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility
		FROM assets WHERE group_id=? ORDER BY category,title,id`
	args := []any{groupID}
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
		if err := rows.Scan(&item.ID, &item.GroupID, &item.Category, &item.Title, &item.OriginalName, &item.StoragePath, &item.MimeType, &item.FileSize, &item.ChecksumSHA256, &item.Visibility); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) Create(ctx context.Context, item *Asset, actorID uint64) (uint64, error) {
	now := nowSQL()
	visibility := firstNonEmpty(item.Visibility, "group")
	res, err := r.db.ExecContext(ctx, `INSERT INTO assets (group_id,category,title,original_name,storage_path,mime_type,file_size,checksum_sha256,visibility,created_by,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.GroupID, item.Category, item.Title, item.OriginalName, item.StoragePath, item.MimeType, item.FileSize, item.ChecksumSHA256, visibility, actorID, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return uint64(id), nil
}

func (r *MySQLRepository) Delete(ctx context.Context, groupID, id uint64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM assets WHERE id=? AND group_id=?`, id, groupID)
	return err
}

func nowSQL() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000")
}
