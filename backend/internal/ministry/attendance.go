package ministry

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrAttendanceUnsupported = errors.New("ministry_attendance_unsupported")
	ErrInvalidAttendanceDate = errors.New("ministry_invalid_attendance_date")
	ErrInvalidAttendanceRule = errors.New("ministry_invalid_attendance_rule")
)

const countingGroupCode = "counting"

var defaultAttendanceWeekdays = []int{1, 7}

func (s *Service) AttendanceSheet(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	month string,
	loc *time.Location,
) (AttendanceSheetVO, error) {
	access, err := s.attendanceAccess(ctx, studyGroupID, groupID, actor)
	if err != nil {
		return AttendanceSheetVO{}, err
	}
	if !access.IsMember && !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return AttendanceSheetVO{}, ErrForbidden
	}
	start, end, err := attendanceMonthRange(month, loc)
	if err != nil {
		return AttendanceSheetVO{}, err
	}
	weekdays, extraDates, err := s.repo.AttendanceSettings(ctx, studyGroupID, groupID)
	if err != nil {
		return AttendanceSheetVO{}, err
	}
	dates := attendanceDates(start, end, weekdays, extraDates)
	members, err := s.repo.AttendanceMembers(ctx, studyGroupID)
	if err != nil {
		return AttendanceSheetVO{}, err
	}
	records, err := s.repo.AttendanceRecords(
		ctx,
		studyGroupID,
		groupID,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	if err != nil {
		return AttendanceSheetVO{}, err
	}
	for _, record := range records {
		if !containsString(dates, record.Date) {
			dates = append(dates, record.Date)
		}
	}
	sort.Strings(dates)
	present := make(map[uint64]map[string]bool)
	for _, record := range records {
		if present[record.UserID] == nil {
			present[record.UserID] = make(map[string]bool)
		}
		present[record.UserID][record.Date] = true
	}
	memberVOs := make([]AttendanceMemberVO, 0, len(members))
	for _, member := range members {
		memberPresent := present[member.UserID]
		if memberPresent == nil {
			memberPresent = map[string]bool{}
		}
		memberVOs = append(memberVOs, AttendanceMemberVO{
			UserID:       member.UserID,
			Username:     member.Username,
			DisplayName:  member.DisplayName,
			Present:      memberPresent,
			PresentCount: len(memberPresent),
		})
	}
	return AttendanceSheetVO{
		Month:     start.Format("2006-01"),
		Dates:     dates,
		Members:   memberVOs,
		Settings:  AttendanceSettingsVO{Weekdays: weekdays, ExtraDates: extraDates},
		CanMark:   access.IsMember || actor.IsSuperAdmin || actor.IsStudyAdmin,
		CanManage: actor.IsSuperAdmin || actor.IsStudyAdmin || access.IsAdmin,
	}, nil
}

func (s *Service) SaveAttendanceSettings(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	input AttendanceSettingsInput,
	at time.Time,
) error {
	access, err := s.attendanceAccess(ctx, studyGroupID, groupID, actor)
	if err != nil {
		return err
	}
	if !actor.IsSuperAdmin && !actor.IsStudyAdmin && !access.IsAdmin {
		return ErrForbidden
	}
	weekdays, err := normalizeWeekdays(input.Weekdays)
	if err != nil {
		return err
	}
	extraDates, err := normalizeAttendanceDates(input.ExtraDates)
	if err != nil {
		return err
	}
	return s.repo.SaveAttendanceSettings(ctx, studyGroupID, groupID, actor.UserID, weekdays, extraDates, at)
}

func (s *Service) SetAttendance(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	actor Actor,
	date string,
	present bool,
	at time.Time,
) error {
	access, err := s.attendanceAccess(ctx, studyGroupID, groupID, actor)
	if err != nil {
		return err
	}
	if !access.IsMember && !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return ErrForbidden
	}
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return ErrInvalidAttendanceDate
	}
	weekdays, extraDates, err := s.repo.AttendanceSettings(ctx, studyGroupID, groupID)
	if err != nil {
		return err
	}
	if present && !isRequiredAttendanceDate(parsed, weekdays, extraDates) {
		return ErrInvalidAttendanceDate
	}
	active, err := s.repo.IsActiveStudyGroupMember(ctx, studyGroupID, userID)
	if err != nil {
		return err
	}
	if !active {
		return ErrNotMember
	}
	return s.repo.SetAttendance(ctx, studyGroupID, groupID, userID, actor.UserID, parsed.Format("2006-01-02"), present, at)
}

func (s *Service) attendanceAccess(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
) (Access, error) {
	code, err := s.repo.AttendanceGroupCode(ctx, studyGroupID, groupID)
	if err != nil {
		return Access{}, ErrGroupNotFound
	}
	if code != countingGroupCode {
		return Access{}, ErrAttendanceUnsupported
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return Access{}, ErrGroupNotFound
	}
	return access, nil
}

func attendanceMonthRange(month string, loc *time.Location) (time.Time, time.Time, error) {
	month = strings.TrimSpace(month)
	if month == "" {
		month = time.Now().In(loc).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01-02", month+"-01", loc)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidAttendanceDate
	}
	return start, start.AddDate(0, 1, -1), nil
}

func normalizeWeekdays(values []int) ([]int, error) {
	seen := make(map[int]bool, len(values))
	for _, value := range values {
		if value < 1 || value > 7 {
			return nil, ErrInvalidAttendanceRule
		}
		seen[value] = true
	}
	out := make([]int, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Ints(out)
	return out, nil
}

func normalizeAttendanceDates(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		date, err := time.Parse("2006-01-02", strings.TrimSpace(value))
		if err != nil {
			return nil, ErrInvalidAttendanceDate
		}
		seen[date.Format("2006-01-02")] = true
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func attendanceDates(start, end time.Time, weekdays []int, extraDates []string) []string {
	seen := make(map[string]bool)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if containsWeekday(weekdays, weekdayNumber(date)) {
			seen[date.Format("2006-01-02")] = true
		}
	}
	for _, value := range extraDates {
		date, err := time.Parse("2006-01-02", value)
		if err == nil && !date.Before(start) && !date.After(end) {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isRequiredAttendanceDate(date time.Time, weekdays []int, extraDates []string) bool {
	if containsWeekday(weekdays, weekdayNumber(date)) {
		return true
	}
	target := date.Format("2006-01-02")
	for _, value := range extraDates {
		if value == target {
			return true
		}
	}
	return false
}

func weekdayNumber(date time.Time) int {
	if date.Weekday() == time.Sunday {
		return 7
	}
	return int(date.Weekday())
}

func containsWeekday(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
