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
	_, err := r.db.ExecContext(ctx, `INSERT INTO audit_logs (group_id,actor_user_id,action,target_type,target_id,before_json,after_json,ip,user_agent,log_id,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		nullableID(log.GroupID), log.ActorID, log.Action, log.TargetType, nullableID(log.TargetID), nullableString(log.BeforeJSON), nullableString(log.AfterJSON), log.IP, log.UserAgent, log.LogID, log.CreatedAt)
	return err
}

func (r *MySQLRepository) ListByGroup(ctx context.Context, groupID uint64, limit int) ([]Log, error) {
	return r.list(ctx, `SELECT a.id,a.group_id,a.actor_user_id,
		COALESCE(u.username,''),COALESCE(u.display_name,''),
		a.action,a.target_type,a.target_id,a.before_json,a.after_json,a.log_id,a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id=a.actor_user_id
		WHERE a.group_id=? ORDER BY a.id DESC LIMIT ?`, groupID, limit)
}

func (r *MySQLRepository) ListAll(ctx context.Context, limit int) ([]Log, error) {
	return r.list(ctx, `SELECT a.id,a.group_id,a.actor_user_id,
		COALESCE(u.username,''),COALESCE(u.display_name,''),
		a.action,a.target_type,a.target_id,a.before_json,a.after_json,a.log_id,a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id=a.actor_user_id
		ORDER BY a.id DESC LIMIT ?`, limit)
}

func (r *MySQLRepository) list(ctx context.Context, query string, args ...any) ([]Log, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Log
	for rows.Next() {
		var item Log
		var groupID, targetID sql.NullInt64
		var beforeJSON, afterJSON sql.NullString
		var created time.Time
		if err := rows.Scan(
			&item.ID,
			&groupID,
			&item.ActorID,
			&item.ActorUsername,
			&item.ActorDisplayName,
			&item.Action,
			&item.TargetType,
			&targetID,
			&beforeJSON,
			&afterJSON,
			&item.LogID,
			&created,
		); err != nil {
			return nil, err
		}
		if groupID.Valid && groupID.Int64 > 0 {
			item.GroupID = uint64(groupID.Int64)
		}
		if targetID.Valid && targetID.Int64 > 0 {
			item.TargetID = uint64(targetID.Int64)
		}
		if beforeJSON.Valid {
			item.BeforeJSON = beforeJSON.String
		}
		if afterJSON.Valid {
			item.AfterJSON = afterJSON.String
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
