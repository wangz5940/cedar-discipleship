package server

import (
	"context"
	"time"
)

// The narrow sending window prevents a restart or unmute from replaying missed reminder slots.
func devotionReminderDue(now time.Time) bool {
	at := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	return (at.Hour() == 7 || at.Hour() == 18) && at.Minute() == 30
}

func (a *app) enqueueDailyDevotionReminders(ctx context.Context, now time.Time) error {
	if !devotionReminderDue(now) {
		return nil
	}
	date := now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
	rows, err := a.db.QueryContext(ctx, `SELECT g.id FROM study_groups g
        LEFT JOIN group_settings s ON s.group_id=g.id
        WHERE g.status=1 AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(s.settings,'$.checkin_notifications.daily_enabled')),'true')='true'`)
	if err != nil {
		return err
	}
	var groups []uint64
	for rows.Next() {
		var id uint64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, groupID := range groups {
		if err = a.enqueueGroupDevotionReminder(ctx, groupID, date, now); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) enqueueGroupDevotionReminder(ctx context.Context, groupID uint64, date string, now time.Time) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT IGNORE INTO daily_devotion_reminder_runs(group_id,logical_date,reminder_hour) VALUES(?,?,?)`, groupID, date, now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Hour())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	// Manual reminders forbid self-reminding; self-sent rows identify this system reminder.
	// Muted members are skipped permanently for this slot, without affecting historical messages.
	_, err = tx.ExecContext(ctx, `INSERT INTO learning_reminders(group_id,sender_id,recipient_id,logical_date,created_at,daily_limit_slot)
        SELECT m.group_id,m.user_id,m.user_id,?,?,NULL FROM group_members m
        JOIN users u ON u.id=m.user_id AND u.status=1
        LEFT JOIN learning_reminder_preferences p ON p.group_id=m.group_id AND p.user_id=m.user_id
        WHERE m.group_id=? AND m.status=1 AND COALESCE(p.muted,FALSE)=FALSE`, date, now.UTC(), groupID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
