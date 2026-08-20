package ministry

import (
	"context"
	"time"
)

type Repository interface {
	EnsureCatalog(ctx context.Context, studyGroupID uint64, at time.Time) error
	ListGroups(ctx context.Context, studyGroupID, userID uint64) ([]GroupSummary, error)
	Group(ctx context.Context, studyGroupID, groupID, userID uint64) (*GroupSummary, error)
	CreateGroup(ctx context.Context, studyGroupID uint64, input GroupInput, at time.Time) (uint64, error)
	UpdateGroup(ctx context.Context, studyGroupID, groupID uint64, input GroupInput, at time.Time) error
	DeleteGroup(ctx context.Context, studyGroupID, groupID uint64, at time.Time) error
	Access(ctx context.Context, studyGroupID, groupID, userID uint64) (Access, error)
	ListMembers(ctx context.Context, studyGroupID, groupID uint64) ([]Member, error)

	RequestJoin(ctx context.Context, studyGroupID, groupID, userID uint64, message string, autoApprove bool, at time.Time) (uint64, error)
	Leave(ctx context.Context, studyGroupID, groupID, userID uint64, at time.Time) error
	SetIdentityPublic(ctx context.Context, studyGroupID, groupID, userID uint64, public bool, at time.Time) error
	SetMemberRole(ctx context.Context, studyGroupID, groupID, userID uint64, role MemberRole, at time.Time) error
	UpdateSettings(ctx context.Context, studyGroupID, groupID uint64, input GroupSettingsInput, at time.Time) error
	ListPendingRequests(ctx context.Context, studyGroupID uint64) ([]Request, error)
	Request(ctx context.Context, studyGroupID, requestID uint64) (*Request, error)
	DecideRequest(ctx context.Context, studyGroupID, requestID, reviewerID uint64, decision Status, at time.Time) error

	ListNotifications(ctx context.Context, studyGroupID, userID uint64, limit int) ([]Notification, error)
	ReadNotification(ctx context.Context, studyGroupID, notificationID, userID uint64, at time.Time) error

	ListShares(ctx context.Context, studyGroupID, groupID, userID uint64, canModerate bool) ([]Share, error)
	CreateShare(ctx context.Context, studyGroupID, groupID, authorID uint64, input ShareInput, status Status, at time.Time) (uint64, error)
	UpdateShare(ctx context.Context, studyGroupID, groupID, shareID, actorID uint64, input ShareInput, status Status, canManage bool, at time.Time) error
	DecideShare(ctx context.Context, studyGroupID, groupID, shareID, reviewerID uint64, decision Status, at time.Time) error
	SetSharePinned(ctx context.Context, studyGroupID, groupID, shareID, actorID uint64, pinned bool, at time.Time) error

	ListProgress(ctx context.Context, studyGroupID, groupID uint64, limit int) ([]Progress, error)
	CreateProgress(ctx context.Context, studyGroupID, groupID, authorID uint64, input ProgressInput, at time.Time) (uint64, error)

	AttendanceSettings(ctx context.Context, studyGroupID, groupID uint64) ([]int, []string, error)
	AttendanceGroupCode(ctx context.Context, studyGroupID, groupID uint64) (string, error)
	SaveAttendanceSettings(ctx context.Context, studyGroupID, groupID, actorID uint64, weekdays []int, extraDates []string, at time.Time) error
	AttendanceMembers(ctx context.Context, studyGroupID uint64) ([]AttendanceMember, error)
	AttendanceRecords(ctx context.Context, studyGroupID, groupID uint64, from, to string) ([]AttendanceRecord, error)
	IsActiveStudyGroupMember(ctx context.Context, studyGroupID, userID uint64) (bool, error)
	SetAttendance(ctx context.Context, studyGroupID, groupID, userID, actorID uint64, date string, present bool, at time.Time) error
}
