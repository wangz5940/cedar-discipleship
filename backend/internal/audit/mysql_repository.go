package audit

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

func (r *MySQLRepository) Create(ctx context.Context, log Log) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_logs (group_id,actor_user_id,action,target_type,target_id,before_json,after_json,ip,user_agent,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		nullableID(log.GroupID), log.ActorID, log.Action, log.TargetType, nullableID(log.TargetID), nullableString(log.BeforeJSON), nullableString(log.AfterJSON), log.IP, log.UserAgent, log.CreatedAt)
	return err
}

func (r *MySQLRepository) ListByGroup(ctx context.Context, groupID uint64, limit int) ([]Log, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,actor_user_id,action,target_type,target_id,created_at FROM audit_logs WHERE group_id=? ORDER BY id DESC LIMIT ?`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Log
	for rows.Next() {
		var item Log
		var targetID sql.NullInt64
		var created time.Time
		if err := rows.Scan(&item.ID, &item.ActorID, &item.Action, &item.TargetType, &targetID, &created); err != nil {
			return nil, err
		}
		if targetID.Valid && targetID.Int64 > 0 {
			item.TargetID = uint64(targetID.Int64)
		}
		item.CreatedAt = created.Format(time.RFC3339)
		items = append(items, item)
	}
	return items, rows.Err()
}

func nullableID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
