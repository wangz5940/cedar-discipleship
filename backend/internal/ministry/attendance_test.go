package ministry

import (
	"context"
	"errors"
	"testing"
	"time"
)

type attendanceTestRepository struct {
	Repository
	code            string
	access          Access
	weekdays        []int
	extraDates      []string
	members         []AttendanceMember
	records         []AttendanceRecord
	activeMember    bool
	savedWeekdays   []int
	savedExtraDates []string
	markedUserID    uint64
	markedDate      string
	markedPresent   bool
}

func (r *attendanceTestRepository) AttendanceGroupCode(context.Context, uint64, uint64) (string, error) {
	return r.code, nil
}

func (r *attendanceTestRepository) Access(context.Context, uint64, uint64, uint64) (Access, error) {
	return r.access, nil
}

func (r *attendanceTestRepository) AttendanceSettings(context.Context, uint64, uint64) ([]int, []string, error) {
	return r.weekdays, r.extraDates, nil
}

func (r *attendanceTestRepository) AttendanceMembers(context.Context, uint64) ([]AttendanceMember, error) {
	return r.members, nil
}

func (r *attendanceTestRepository) AttendanceRecords(context.Context, uint64, uint64, string, string) ([]AttendanceRecord, error) {
	return r.records, nil
}

func (r *attendanceTestRepository) SaveAttendanceSettings(
	_ context.Context,
	_, _, _ uint64,
	weekdays []int,
	extraDates []string,
	_ time.Time,
) error {
	r.savedWeekdays = weekdays
	r.savedExtraDates = extraDates
	return nil
}

func (r *attendanceTestRepository) IsActiveStudyGroupMember(context.Context, uint64, uint64) (bool, error) {
	return r.activeMember, nil
}

func (r *attendanceTestRepository) SetAttendance(
	_ context.Context,
	_, _ uint64,
	userID, _ uint64,
	date string,
	present bool,
	_ time.Time,
) error {
	r.markedUserID = userID
	r.markedDate = date
	r.markedPresent = present
	return nil
}

func TestServiceAttendanceSheetBuildsRequiredDates(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{
		code:       countingGroupCode,
		access:     Access{IsMember: true},
		weekdays:   []int{1, 7},
		extraDates: []string{"2026-08-05"},
		members: []AttendanceMember{{
			UserID:      9,
			Username:    "member",
			DisplayName: "成员甲",
		}},
		records: []AttendanceRecord{{UserID: 9, Date: "2026-08-02"}},
	}
	service := NewService(repo)
	loc := time.FixedZone("CST", 8*60*60)

	sheet, err := service.AttendanceSheet(context.Background(), 1, 2, Actor{UserID: 7}, "2026-08", loc)
	if err != nil {
		t.Fatalf("AttendanceSheet() error = %v", err)
	}
	if len(sheet.Dates) != 11 {
		t.Fatalf("attendance dates = %v, want 11 dates", sheet.Dates)
	}
	if !sheet.Members[0].Present["2026-08-02"] || sheet.Members[0].PresentCount != 1 {
		t.Fatalf("member attendance = %+v, want 2026-08-02 present", sheet.Members[0])
	}
	if !sheet.CanMark || sheet.CanManage {
		t.Fatalf("permissions = mark:%v manage:%v, want true,false", sheet.CanMark, sheet.CanManage)
	}
}

func TestServiceSaveAttendanceSettingsRequiresAdmin(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{
		code:   countingGroupCode,
		access: Access{IsMember: true},
	}
	service := NewService(repo)

	err := service.SaveAttendanceSettings(
		context.Background(),
		1,
		2,
		Actor{UserID: 7},
		AttendanceSettingsInput{Weekdays: []int{1, 7}},
		time.Now(),
	)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("SaveAttendanceSettings() error = %v, want %v", err, ErrForbidden)
	}
}

func TestServiceSaveAttendanceSettingsNormalizesValues(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{
		code:   countingGroupCode,
		access: Access{IsMember: true, IsAdmin: true},
	}
	service := NewService(repo)

	err := service.SaveAttendanceSettings(
		context.Background(),
		1,
		2,
		Actor{UserID: 7},
		AttendanceSettingsInput{
			Weekdays:   []int{7, 1, 7},
			ExtraDates: []string{"2026-08-09", "2026-08-05", "2026-08-09"},
		},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("SaveAttendanceSettings() error = %v", err)
	}
	if got := repo.savedWeekdays; len(got) != 2 || got[0] != 1 || got[1] != 7 {
		t.Fatalf("saved weekdays = %v, want [1 7]", got)
	}
	if got := repo.savedExtraDates; len(got) != 2 || got[0] != "2026-08-05" || got[1] != "2026-08-09" {
		t.Fatalf("saved extra dates = %v", got)
	}
}

func TestServiceSetAttendanceAllowsCountingMember(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{
		code:         countingGroupCode,
		access:       Access{IsMember: true},
		weekdays:     []int{7},
		activeMember: true,
	}
	service := NewService(repo)

	err := service.SetAttendance(
		context.Background(),
		1,
		2,
		9,
		Actor{UserID: 7},
		"2026-08-02",
		true,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("SetAttendance() error = %v", err)
	}
	if repo.markedUserID != 9 || repo.markedDate != "2026-08-02" || !repo.markedPresent {
		t.Fatalf("marked attendance = user:%d date:%s present:%v", repo.markedUserID, repo.markedDate, repo.markedPresent)
	}
}

func TestServiceSetAttendanceRejectsUnscheduledDate(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{
		code:         countingGroupCode,
		access:       Access{IsMember: true},
		weekdays:     []int{7},
		activeMember: true,
	}
	service := NewService(repo)

	err := service.SetAttendance(
		context.Background(),
		1,
		2,
		9,
		Actor{UserID: 7},
		"2026-08-03",
		true,
		time.Now(),
	)
	if !errors.Is(err, ErrInvalidAttendanceDate) {
		t.Fatalf("SetAttendance() error = %v, want %v", err, ErrInvalidAttendanceDate)
	}
}

func TestServiceAttendanceRejectsOtherMinistryGroup(t *testing.T) {
	t.Parallel()

	repo := &attendanceTestRepository{code: "hosting"}
	service := NewService(repo)

	_, err := service.AttendanceSheet(
		context.Background(),
		1,
		2,
		Actor{UserID: 7},
		"2026-08",
		time.UTC,
	)
	if !errors.Is(err, ErrAttendanceUnsupported) {
		t.Fatalf("AttendanceSheet() error = %v, want %v", err, ErrAttendanceUnsupported)
	}
}
