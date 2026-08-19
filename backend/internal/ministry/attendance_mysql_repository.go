package ministry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r *MySQLRepository) AttendanceGroupCode(
	ctx context.Context,
	studyGroupID, groupID uint64,
) (string, error) {
	var code string
	err := r.db.QueryRowContext(
		ctx,
		`SELECT code FROM ministry_groups WHERE id=? AND study_group_id=? AND status=1`,
		groupID,
		studyGroupID,
	).Scan(&code)
	return code, err
}

func (r *MySQLRepository) AttendanceSettings(
	ctx context.Context,
	studyGroupID, groupID uint64,
) ([]int, []string, error) {
	var mask uint
	err := r.db.QueryRowContext(
		ctx,
		`SELECT weekday_mask FROM ministry_attendance_settings
		  WHERE study_group_id=? AND ministry_group_id=?`,
		studyGroupID,
		groupID,
	).Scan(&mask)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		mask = weekdayMask(defaultAttendanceWeekdays)
	default:
		return nil, nil, fmt.Errorf("loading attendance settings: %w", err)
	}
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT DATE_FORMAT(attendance_date,'%Y-%m-%d')
		   FROM ministry_attendance_dates
		  WHERE study_group_id=? AND ministry_group_id=?
		  ORDER BY attendance_date`,
		studyGroupID,
		groupID,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("listing attendance dates: %w", err)
	}
	defer rows.Close()
	extraDates := []string{}
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, nil, fmt.Errorf("scanning attendance date: %w", err)
		}
		extraDates = append(extraDates, date)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating attendance dates: %w", err)
	}
	return weekdaysFromMask(mask), extraDates, nil
}

func (r *MySQLRepository) SaveAttendanceSettings(
	ctx context.Context,
	studyGroupID, groupID, actorID uint64,
	weekdays []int,
	extraDates []string,
	at time.Time,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning attendance settings: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO ministry_attendance_settings
			(study_group_id,ministry_group_id,weekday_mask,updated_by,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE weekday_mask=VALUES(weekday_mask),
		   updated_by=VALUES(updated_by),updated_at=VALUES(updated_at)`,
		studyGroupID,
		groupID,
		weekdayMask(weekdays),
		actorID,
		at,
		at,
	); err != nil {
		return fmt.Errorf("saving attendance settings: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM ministry_attendance_dates
		  WHERE study_group_id=? AND ministry_group_id=?`,
		studyGroupID,
		groupID,
	); err != nil {
		return fmt.Errorf("clearing attendance dates: %w", err)
	}
	for _, date := range extraDates {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO ministry_attendance_dates
				(study_group_id,ministry_group_id,attendance_date,created_by,created_at)
			 VALUES (?,?,?,?,?)`,
			studyGroupID,
			groupID,
			date,
			actorID,
			at,
		); err != nil {
			return fmt.Errorf("saving attendance date: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing attendance settings: %w", err)
	}
	return nil
}

func (r *MySQLRepository) AttendanceMembers(
	ctx context.Context,
	studyGroupID uint64,
) ([]AttendanceMember, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT u.id,u.username,COALESCE(NULLIF(gm.member_name,''),u.display_name)
		   FROM group_members gm
		   JOIN users u ON u.id=gm.user_id AND u.status=1
		  WHERE gm.group_id=? AND gm.status=1
		  ORDER BY COALESCE(NULLIF(gm.member_name,''),u.display_name),u.id`,
		studyGroupID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing attendance members: %w", err)
	}
	defer rows.Close()
	members := []AttendanceMember{}
	for rows.Next() {
		var member AttendanceMember
		if err := rows.Scan(&member.UserID, &member.Username, &member.DisplayName); err != nil {
			return nil, fmt.Errorf("scanning attendance member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating attendance members: %w", err)
	}
	return members, nil
}

func (r *MySQLRepository) AttendanceRecords(
	ctx context.Context,
	studyGroupID, groupID uint64,
	from, to string,
) ([]AttendanceRecord, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT user_id,DATE_FORMAT(attendance_date,'%Y-%m-%d')
		   FROM ministry_attendance_records
		  WHERE study_group_id=? AND ministry_group_id=?
		    AND attendance_date BETWEEN ? AND ? AND present=1
		  ORDER BY attendance_date,user_id`,
		studyGroupID,
		groupID,
		from,
		to,
	)
	if err != nil {
		return nil, fmt.Errorf("listing attendance records: %w", err)
	}
	defer rows.Close()
	records := []AttendanceRecord{}
	for rows.Next() {
		var record AttendanceRecord
		if err := rows.Scan(&record.UserID, &record.Date); err != nil {
			return nil, fmt.Errorf("scanning attendance record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating attendance records: %w", err)
	}
	return records, nil
}

func (r *MySQLRepository) IsActiveStudyGroupMember(
	ctx context.Context,
	studyGroupID, userID uint64,
) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM group_members gm
		   JOIN users u ON u.id=gm.user_id AND u.status=1
		  WHERE gm.group_id=? AND gm.user_id=? AND gm.status=1`,
		studyGroupID,
		userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking attendance member: %w", err)
	}
	return count > 0, nil
}

func (r *MySQLRepository) SetAttendance(
	ctx context.Context,
	studyGroupID, groupID, userID, actorID uint64,
	date string,
	present bool,
	at time.Time,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO ministry_attendance_records
			(study_group_id,ministry_group_id,attendance_date,user_id,present,marked_by,marked_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE present=VALUES(present),marked_by=VALUES(marked_by),
		   marked_at=VALUES(marked_at),updated_at=VALUES(updated_at)`,
		studyGroupID,
		groupID,
		date,
		userID,
		present,
		actorID,
		at,
		at,
		at,
	)
	if err != nil {
		return fmt.Errorf("saving attendance record: %w", err)
	}
	return nil
}

func weekdayMask(weekdays []int) uint {
	var mask uint
	for _, weekday := range weekdays {
		mask |= 1 << (weekday - 1)
	}
	return mask
}

func weekdaysFromMask(mask uint) []int {
	weekdays := []int{}
	for weekday := 1; weekday <= 7; weekday++ {
		if mask&(1<<(weekday-1)) != 0 {
			weekdays = append(weekdays, weekday)
		}
	}
	return weekdays
}
