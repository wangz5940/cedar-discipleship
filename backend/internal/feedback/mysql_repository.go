package feedback

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) AutomaticSettings(ctx context.Context) (AutomaticSettings, error) {
	settings := AutomaticSettings{Enabled: true, MutedErrorTypes: []string{}}
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled FROM feedback_automatic_settings WHERE id=1`,
	).Scan(&settings.Enabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AutomaticSettings{}, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT error_type FROM feedback_muted_error_types ORDER BY error_type`,
	)
	if err != nil {
		return AutomaticSettings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var errorType string
		if err := rows.Scan(&errorType); err != nil {
			return AutomaticSettings{}, err
		}
		settings.MutedErrorTypes = append(settings.MutedErrorTypes, errorType)
	}
	return settings, rows.Err()
}

func (r *MySQLRepository) SaveAutomaticSettings(
	ctx context.Context,
	settings AutomaticSettings,
	adminUserID uint64,
	updatedAt time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO feedback_automatic_settings
		(id,enabled,updated_by,updated_at) VALUES (1,?,?,?)
		ON DUPLICATE KEY UPDATE enabled=VALUES(enabled),updated_by=VALUES(updated_by),updated_at=VALUES(updated_at)`,
		settings.Enabled, adminUserID, updatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM feedback_muted_error_types`); err != nil {
		return err
	}
	for _, errorType := range settings.MutedErrorTypes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO feedback_muted_error_types
			(error_type,muted_by,created_at) VALUES (?,?,?)`,
			errorType, adminUserID, updatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *MySQLRepository) Create(ctx context.Context, item Feedback, attachments []Attachment) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `INSERT INTO feedbacks
		(group_id,user_id,name,contact,message,source,status,page,user_agent,log_id,
		 diagnostics_json,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		nullableID(item.GroupID), item.UserID, "", "", item.Message, item.Source, item.Status, "", "", item.LogID,
		nullableDiagnostics(item.DiagnosticsJSON), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return 0, err
	}
	rawID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	feedbackID := uint64(rawID)
	for index, attachment := range attachments {
		attachmentResult, err := tx.ExecContext(ctx, `INSERT INTO feedback_attachments
			(feedback_id,original_name,storage_path,mime_type,file_size,width,height,created_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			feedbackID, attachment.OriginalName, attachment.StoragePath, attachment.MimeType,
			attachment.FileSize, attachment.Width, attachment.Height, attachment.CreatedAt)
		if err != nil {
			return 0, err
		}
		attachmentID, err := attachmentResult.LastInsertId()
		if err != nil {
			return 0, err
		}
		attachments[index].ID = uint64(attachmentID)
		attachments[index].FeedbackID = feedbackID
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return feedbackID, nil
}

func (r *MySQLRepository) ListByUser(ctx context.Context, userID uint64, limit int) ([]Feedback, error) {
	return r.list(ctx, `WHERE f.user_id=? ORDER BY f.id DESC LIMIT ?`, userID, limit)
}

func (r *MySQLRepository) FindByUser(ctx context.Context, userID, feedbackID uint64) (*Feedback, error) {
	item, err := r.find(ctx, `WHERE f.id=? AND f.user_id=?`, feedbackID, userID)
	if err != nil {
		return nil, err
	}
	if err := r.loadRelations(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *MySQLRepository) ListAll(ctx context.Context, status Status, limit int) ([]Feedback, error) {
	if status == "" {
		return r.list(ctx, `ORDER BY f.id DESC LIMIT ?`, limit)
	}
	return r.list(ctx, `WHERE f.status=? ORDER BY f.id DESC LIMIT ?`, status, limit)
}

func (r *MySQLRepository) FindByID(ctx context.Context, feedbackID uint64) (*Feedback, error) {
	item, err := r.find(ctx, `WHERE f.id=?`, feedbackID)
	if err != nil {
		return nil, err
	}
	if err := r.loadRelations(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *MySQLRepository) UpdateStatus(ctx context.Context, feedbackID uint64, status Status, updatedAt time.Time) (bool, error) {
	var result sql.Result
	var err error
	if status == StatusClosed {
		result, err = r.db.ExecContext(ctx, `UPDATE feedbacks
			SET status=?,diagnostics_json=NULL,updated_at=?
			WHERE id=?`, status, updatedAt, feedbackID)
	} else {
		result, err = r.db.ExecContext(ctx, `UPDATE feedbacks SET status=?,updated_at=? WHERE id=?`,
			status, updatedAt, feedbackID)
	}
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected > 0 {
		return affected > 0, err
	}
	var exists int
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedbacks WHERE id=?`, feedbackID).Scan(&exists)
	return exists > 0, err
}

func (r *MySQLRepository) CreateReply(ctx context.Context, reply Reply) (uint64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var exists uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM feedbacks WHERE id=? FOR UPDATE`, reply.FeedbackID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO feedback_replies
		(feedback_id,admin_user_id,message,created_at) VALUES (?,?,?,?)`,
		reply.FeedbackID, reply.AdminUserID, reply.Message, reply.CreatedAt)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE feedbacks SET updated_at=? WHERE id=?`, reply.CreatedAt, reply.FeedbackID); err != nil {
		return 0, err
	}
	rawID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(rawID), nil
}

func (r *MySQLRepository) FindAttachment(ctx context.Context, feedbackID, attachmentID uint64) (*Attachment, error) {
	var item Attachment
	err := r.db.QueryRowContext(ctx, `SELECT id,feedback_id,original_name,storage_path,mime_type,
		file_size,width,height,created_at FROM feedback_attachments WHERE id=? AND feedback_id=?`,
		attachmentID, feedbackID).Scan(
		&item.ID, &item.FeedbackID, &item.OriginalName, &item.StoragePath, &item.MimeType,
		&item.FileSize, &item.Width, &item.Height, &item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *MySQLRepository) find(ctx context.Context, clause string, args ...any) (*Feedback, error) {
	items, err := r.list(ctx, clause+` LIMIT 1`, args...)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return &items[0], nil
}

func (r *MySQLRepository) list(ctx context.Context, clause string, args ...any) ([]Feedback, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		f.id,f.group_id,f.user_id,COALESCE(u.username,''),COALESCE(u.display_name,''),
		COALESCE(gm.member_name,''),COALESCE(g.name,''),
		f.name,f.contact,f.message,COALESCE(NULLIF(f.source,''),'manual'),
		COALESCE(NULLIF(f.status,''),'pending'),f.log_id,
		COALESCE(CAST(f.diagnostics_json AS CHAR),''),
		f.created_at,f.updated_at,
		(SELECT COUNT(*) FROM feedback_attachments fa WHERE fa.feedback_id=f.id),
		(SELECT COUNT(*) FROM feedback_replies fr WHERE fr.feedback_id=f.id)
		FROM feedbacks f
		LEFT JOIN users u ON u.id=f.user_id
		LEFT JOIN study_groups g ON g.id=f.group_id
		LEFT JOIN group_members gm ON gm.group_id=f.group_id AND gm.user_id=f.user_id
		`+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Feedback, 0)
	for rows.Next() {
		var item Feedback
		var groupID, userID sql.NullInt64
		if err := rows.Scan(
			&item.ID, &groupID, &userID, &item.Username, &item.DisplayName,
			&item.MemberName, &item.GroupName,
			&item.LegacyName, &item.LegacyContact, &item.Message, &item.Source, &item.Status, &item.LogID,
			&item.DiagnosticsJSON, &item.CreatedAt, &item.UpdatedAt,
			&item.AttachmentCount, &item.ReplyCount,
		); err != nil {
			return nil, err
		}
		if groupID.Valid && groupID.Int64 > 0 {
			item.GroupID = uint64(groupID.Int64)
		}
		if userID.Valid && userID.Int64 > 0 {
			item.UserID = uint64(userID.Int64)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) loadRelations(ctx context.Context, item *Feedback) error {
	attachments, err := r.attachments(ctx, item.ID)
	if err != nil {
		return err
	}
	replies, err := r.replies(ctx, item.ID)
	if err != nil {
		return err
	}
	item.Attachments = attachments
	item.Replies = replies
	item.AttachmentCount = len(attachments)
	item.ReplyCount = len(replies)
	return nil
}

func (r *MySQLRepository) attachments(ctx context.Context, feedbackID uint64) ([]Attachment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,feedback_id,original_name,storage_path,mime_type,
		file_size,width,height,created_at FROM feedback_attachments WHERE feedback_id=? ORDER BY id`, feedbackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Attachment, 0)
	for rows.Next() {
		var item Attachment
		if err := rows.Scan(
			&item.ID, &item.FeedbackID, &item.OriginalName, &item.StoragePath, &item.MimeType,
			&item.FileSize, &item.Width, &item.Height, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *MySQLRepository) replies(ctx context.Context, feedbackID uint64) ([]Reply, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT fr.id,fr.feedback_id,fr.admin_user_id,
		COALESCE(u.display_name,''),fr.message,fr.created_at
		FROM feedback_replies fr
		LEFT JOIN users u ON u.id=fr.admin_user_id
		WHERE fr.feedback_id=? ORDER BY fr.id`, feedbackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Reply, 0)
	for rows.Next() {
		var item Reply
		if err := rows.Scan(
			&item.ID, &item.FeedbackID, &item.AdminUserID,
			&item.AdminDisplayName, &item.Message, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
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

func nullableDiagnostics(value string) any {
	if value == "" {
		return nil
	}
	return value
}
