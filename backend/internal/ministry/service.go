package ministry

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrGroupNotFound          = errors.New("ministry_group_not_found")
	ErrForbidden              = errors.New("ministry_forbidden")
	ErrAlreadyMember          = errors.New("ministry_already_member")
	ErrNotMember              = errors.New("ministry_not_member")
	ErrLeaderCannotLeave      = errors.New("ministry_leader_cannot_leave")
	ErrInvalidDecision        = errors.New("ministry_invalid_decision")
	ErrRequestAlreadyReviewed = errors.New("ministry_request_already_reviewed")
	ErrRequestNotFound        = errors.New("ministry_request_not_found")
	ErrShareNotFound          = errors.New("ministry_share_not_found")
	ErrProgressNotFound       = errors.New("ministry_progress_not_found")
	ErrShareAlreadyReviewed   = errors.New("ministry_share_already_reviewed")
	ErrContentRequired        = errors.New("ministry_content_required")
	ErrInvalidVisibility      = errors.New("ministry_invalid_visibility")
	ErrInvalidRole            = errors.New("ministry_invalid_role")
	ErrInvalidAttachment      = errors.New("ministry_invalid_attachment")
	ErrInvalidGroupName       = errors.New("ministry_invalid_group_name")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Groups(ctx context.Context, studyGroupID uint64, actor Actor) ([]GroupSummary, error) {
	if err := s.repo.EnsureCatalog(ctx, studyGroupID, time.Now().UTC()); err != nil {
		return nil, err
	}
	groups, err := s.repo.ListGroups(ctx, studyGroupID, actor.UserID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].CanManage = actor.IsSuperAdmin || actor.IsStudyAdmin || groups[i].IsLeader || groups[i].Role == MemberRoleAdmin
		groups[i].CanReviewShares = actor.IsSuperAdmin || actor.IsStudyAdmin || groups[i].Role == MemberRoleAdmin
	}
	return groups, nil
}

func (s *Service) Detail(ctx context.Context, studyGroupID, groupID uint64, actor Actor) (*GroupDetail, error) {
	group, err := s.repo.Group(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return nil, ErrGroupNotFound
	}
	group.CanManage = canManage(actor, access)
	group.CanReviewShares = canReviewShares(actor, access)

	members, err := s.repo.ListMembers(ctx, studyGroupID, groupID)
	if err != nil {
		return nil, err
	}
	memberVOs := make([]MemberVO, 0, len(members))
	for _, member := range members {
		isSelf := member.UserID == actor.UserID
		isVisible := group.MemberVisibility == VisibilityAll || member.IdentityPublic || isSelf || group.CanManage
		displayName := "已加入成员"
		userID := uint64(0)
		if isVisible {
			displayName = member.DisplayName
			userID = member.UserID
		}
		memberVOs = append(memberVOs, MemberVO{
			UserID:         userID,
			DisplayName:    displayName,
			Role:           member.Role,
			IsLeader:       member.IsLeader,
			IsSelf:         isSelf,
			IdentityPublic: member.IdentityPublic,
			IsVisible:      isVisible,
		})
	}

	shares, err := s.repo.ListShares(ctx, studyGroupID, groupID, actor.UserID, group.CanReviewShares)
	if err != nil {
		return nil, err
	}
	progress, err := s.repo.ListProgress(ctx, studyGroupID, groupID, 200)
	if err != nil {
		return nil, err
	}
	deletedShares, err := s.repo.ListDeletedShares(
		ctx,
		studyGroupID,
		groupID,
		actor.UserID,
		group.CanManage,
		100,
	)
	if err != nil {
		return nil, err
	}
	deletedProgress, err := s.repo.ListDeletedProgress(
		ctx,
		studyGroupID,
		groupID,
		actor.UserID,
		group.CanManage,
		100,
	)
	if err != nil {
		return nil, err
	}
	return &GroupDetail{
		Group:           *group,
		Members:         memberVOs,
		Shares:          shareVOs(shares, actor, access),
		Progress:        progressVOs(progress, actor, access),
		DeletedShares:   shareVOs(deletedShares, actor, access),
		DeletedProgress: progressVOs(deletedProgress, actor, access),
	}, nil
}

func (s *Service) CreateGroup(
	ctx context.Context,
	studyGroupID uint64,
	actor Actor,
	input GroupInput,
	at time.Time,
) (uint64, error) {
	if !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return 0, ErrForbidden
	}
	input, err := validGroupInput(input)
	if err != nil {
		return 0, err
	}
	return s.repo.CreateGroup(ctx, studyGroupID, input, at)
}

func (s *Service) UpdateGroup(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	input GroupInput,
	at time.Time,
) error {
	if !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return ErrForbidden
	}
	input, err := validGroupInput(input)
	if err != nil {
		return err
	}
	return s.repo.UpdateGroup(ctx, studyGroupID, groupID, input, at)
}

func (s *Service) DeleteGroup(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	at time.Time,
) error {
	if !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return ErrForbidden
	}
	return s.repo.DeleteGroup(ctx, studyGroupID, groupID, at)
}

func (s *Service) RequestJoin(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	message string,
	at time.Time,
) (uint64, error) {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return 0, ErrGroupNotFound
	}
	if access.IsMember {
		return 0, ErrAlreadyMember
	}
	return s.repo.RequestJoin(ctx, studyGroupID, groupID, actor.UserID, strings.TrimSpace(message), access.ShareAutoApprove, at)
}

func (s *Service) Leave(ctx context.Context, studyGroupID, groupID uint64, actor Actor, at time.Time) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !access.IsMember {
		return ErrNotMember
	}
	if access.IsLeader {
		return ErrLeaderCannotLeave
	}
	return s.repo.Leave(ctx, studyGroupID, groupID, actor.UserID, at)
}

func (s *Service) SetIdentityPublic(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	public bool,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !access.IsMember {
		return ErrNotMember
	}
	return s.repo.SetIdentityPublic(ctx, studyGroupID, groupID, actor.UserID, public, at)
}

func (s *Service) SetMemberRole(
	ctx context.Context,
	studyGroupID, groupID, userID uint64,
	actor Actor,
	role MemberRole,
	at time.Time,
) error {
	if role != MemberRoleMember && role != MemberRoleAdmin {
		return ErrInvalidRole
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !canManage(actor, access) {
		return ErrForbidden
	}
	return s.repo.SetMemberRole(ctx, studyGroupID, groupID, userID, role, at)
}

func (s *Service) UpdateSettings(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	input GroupSettingsInput,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if input.MemberVisibility != nil {
		if *input.MemberVisibility != VisibilityAll && *input.MemberVisibility != VisibilitySelf {
			return ErrInvalidVisibility
		}
		if !canManage(actor, access) {
			return ErrForbidden
		}
	}
	if input.ShareAutoApprove != nil && !canReviewShares(actor, access) {
		return ErrForbidden
	}
	if input.LeaderUserID != nil && !actor.IsSuperAdmin && !actor.IsStudyAdmin {
		return ErrForbidden
	}
	return s.repo.UpdateSettings(ctx, studyGroupID, groupID, input, at)
}

func (s *Service) PendingRequests(ctx context.Context, studyGroupID uint64, actor Actor) ([]RequestVO, error) {
	items, err := s.repo.ListPendingRequests(ctx, studyGroupID)
	if err != nil {
		return nil, err
	}
	out := make([]RequestVO, 0, len(items))
	for _, item := range items {
		access, accessErr := s.repo.Access(ctx, studyGroupID, item.GroupID, actor.UserID)
		if accessErr != nil || !canReviewShares(actor, access) {
			continue
		}
		out = append(out, requestVO(item))
	}
	return out, nil
}

func (s *Service) DecideRequest(
	ctx context.Context,
	studyGroupID, requestID uint64,
	actor Actor,
	decision Status,
	at time.Time,
) error {
	if decision != StatusApproved && decision != StatusRejected {
		return ErrInvalidDecision
	}
	target, err := s.repo.Request(ctx, studyGroupID, requestID)
	if err != nil {
		return ErrRequestNotFound
	}
	if target.Status != StatusPending {
		return ErrRequestAlreadyReviewed
	}
	access, err := s.repo.Access(ctx, studyGroupID, target.GroupID, actor.UserID)
	if err != nil || !canReviewShares(actor, access) {
		return ErrForbidden
	}
	return s.repo.DecideRequest(ctx, studyGroupID, requestID, actor.UserID, decision, at)
}

func (s *Service) Notifications(
	ctx context.Context,
	studyGroupID uint64,
	actor Actor,
	limit int,
) ([]NotificationVO, error) {
	items, err := s.repo.ListNotifications(ctx, studyGroupID, actor.UserID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationVO, 0, len(items))
	for _, item := range items {
		out = append(out, NotificationVO{
			ID: item.ID, GroupID: item.GroupID, GroupName: item.GroupName, Type: item.Type,
			Title: item.Title, Body: item.Body, IsRead: item.IsRead, CreatedAt: item.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) ReadNotification(
	ctx context.Context,
	studyGroupID, notificationID uint64,
	actor Actor,
	at time.Time,
) error {
	return s.repo.ReadNotification(ctx, studyGroupID, notificationID, actor.UserID, at)
}

func (s *Service) CreateShare(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	input ShareInput,
	at time.Time,
) (uint64, error) {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" {
		return 0, ErrContentRequired
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return 0, ErrGroupNotFound
	}
	if !canContribute(actor, access) {
		return 0, ErrNotMember
	}
	status := StatusPending
	if access.ShareAutoApprove || canReviewShares(actor, access) {
		status = StatusPublished
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	return s.repo.CreateShare(ctx, studyGroupID, groupID, actor.UserID, input, status, at)
}

func (s *Service) UpdateShare(
	ctx context.Context,
	studyGroupID, groupID, shareID uint64,
	actor Actor,
	input ShareInput,
	at time.Time,
) error {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" {
		return ErrContentRequired
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !canContribute(actor, access) {
		return ErrNotMember
	}
	status := StatusPending
	if access.ShareAutoApprove || canReviewShares(actor, access) {
		status = StatusPublished
	}
	return s.repo.UpdateShare(
		ctx,
		studyGroupID,
		groupID,
		shareID,
		actor.UserID,
		ShareInput{Title: strings.TrimSpace(input.Title), Body: strings.TrimSpace(input.Body)},
		status,
		canManage(actor, access),
		at,
	)
}

func (s *Service) DecideShare(
	ctx context.Context,
	studyGroupID, groupID, shareID uint64,
	actor Actor,
	decision Status,
	at time.Time,
) error {
	if decision != StatusPublished && decision != StatusRejected {
		return ErrInvalidDecision
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil || !canReviewShares(actor, access) {
		return ErrForbidden
	}
	return s.repo.DecideShare(ctx, studyGroupID, groupID, shareID, actor.UserID, decision, at)
}

func (s *Service) SetSharePinned(
	ctx context.Context,
	studyGroupID, groupID, shareID uint64,
	actor Actor,
	pinned bool,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !canReviewShares(actor, access) {
		return ErrForbidden
	}
	return s.repo.SetSharePinned(ctx, studyGroupID, groupID, shareID, actor.UserID, pinned, at)
}

func (s *Service) DeleteShare(
	ctx context.Context,
	studyGroupID, groupID, shareID uint64,
	actor Actor,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	return s.repo.DeleteShare(
		ctx,
		studyGroupID,
		groupID,
		shareID,
		actor.UserID,
		canManage(actor, access),
		at,
	)
}

func (s *Service) RestoreShare(
	ctx context.Context,
	studyGroupID, groupID, shareID uint64,
	actor Actor,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	return s.repo.RestoreShare(
		ctx,
		studyGroupID,
		groupID,
		shareID,
		actor.UserID,
		canManage(actor, access),
		at,
	)
}

func (s *Service) CreateProgress(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
	input ProgressInput,
	at time.Time,
) (uint64, error) {
	if strings.TrimSpace(input.Content) == "" {
		return 0, ErrContentRequired
	}
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return 0, ErrGroupNotFound
	}
	if !canContribute(actor, access) {
		return 0, ErrNotMember
	}
	input.Content = strings.TrimSpace(input.Content)
	if input.OccurredAt.IsZero() {
		input.OccurredAt = at
	}
	return s.repo.CreateProgress(ctx, studyGroupID, groupID, actor.UserID, input, at)
}

func (s *Service) DeleteProgress(
	ctx context.Context,
	studyGroupID, groupID, progressID uint64,
	actor Actor,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	return s.repo.DeleteProgress(
		ctx,
		studyGroupID,
		groupID,
		progressID,
		actor.UserID,
		canManage(actor, access),
		at,
	)
}

func (s *Service) RestoreProgress(
	ctx context.Context,
	studyGroupID, groupID, progressID uint64,
	actor Actor,
	at time.Time,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	return s.repo.RestoreProgress(
		ctx,
		studyGroupID,
		groupID,
		progressID,
		actor.UserID,
		canManage(actor, access),
		at,
	)
}

func (s *Service) CanContribute(
	ctx context.Context,
	studyGroupID, groupID uint64,
	actor Actor,
) error {
	access, err := s.repo.Access(ctx, studyGroupID, groupID, actor.UserID)
	if err != nil {
		return ErrGroupNotFound
	}
	if !canContribute(actor, access) {
		return ErrNotMember
	}
	return nil
}

func canManage(actor Actor, access Access) bool {
	return actor.IsSuperAdmin || actor.IsStudyAdmin || access.IsLeader || access.IsAdmin
}

func canReviewShares(actor Actor, access Access) bool {
	return actor.IsSuperAdmin || actor.IsStudyAdmin || access.IsAdmin
}

func canContribute(actor Actor, access Access) bool {
	return actor.IsSuperAdmin || actor.IsStudyAdmin || access.IsMember
}

func validGroupInput(input GroupInput) (GroupInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || len([]rune(input.Name)) > 128 || len([]rune(input.Description)) > 512 {
		return GroupInput{}, ErrInvalidGroupName
	}
	return input, nil
}

func requestVO(item Request) RequestVO {
	return RequestVO{
		ID: item.ID, GroupID: item.GroupID, UserID: item.UserID,
		UserDisplayName: item.UserDisplayName, Message: item.Message,
		Status: item.Status, CreatedAt: item.CreatedAt,
	}
}

func shareVOs(items []Share, actor Actor, access Access) []ShareVO {
	out := make([]ShareVO, 0, len(items))
	for _, item := range items {
		out = append(out, ShareVO{
			ID: item.ID, GroupID: item.GroupID, AuthorID: item.AuthorID,
			AuthorName: item.AuthorName, Title: item.Title, Body: item.Body,
			Status: item.Status, IsPinned: item.IsPinned,
			CanEdit:     item.DeletedAt == nil && (item.AuthorID == actor.UserID || canManage(actor, access)),
			CanDelete:   item.DeletedAt == nil && (item.AuthorID == actor.UserID || canManage(actor, access)),
			CanRestore:  item.DeletedAt != nil && (item.AuthorID == actor.UserID || canManage(actor, access)),
			CanReview:   item.DeletedAt == nil && canReviewShares(actor, access) && item.Status == StatusPending,
			CanPin:      item.DeletedAt == nil && canReviewShares(actor, access) && item.Status == StatusPublished,
			PublishedAt: item.PublishedAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
			DeletedAt: item.DeletedAt,
		})
	}
	return out
}

func progressVOs(items []Progress, actor Actor, access Access) []ProgressVO {
	out := make([]ProgressVO, 0, len(items))
	for _, item := range items {
		attachments := make([]AttachmentVO, 0, len(item.Attachments))
		for _, attachment := range item.Attachments {
			attachments = append(attachments, AttachmentVO{
				ID: attachment.ID, Title: attachment.Title,
				OriginalName: attachment.OriginalName, MimeType: attachment.MimeType,
				URL: attachment.URL,
			})
		}
		out = append(out, ProgressVO{
			ID: item.ID, GroupID: item.GroupID, AuthorID: item.AuthorID,
			AuthorName: item.AuthorName, OccurredAt: item.OccurredAt,
			Content: item.Content, Attachments: attachments, CreatedAt: item.CreatedAt,
			CanDelete:  item.DeletedAt == nil && (item.AuthorID == actor.UserID || canManage(actor, access)),
			CanRestore: item.DeletedAt != nil && (item.AuthorID == actor.UserID || canManage(actor, access)),
			DeletedAt:  item.DeletedAt,
		})
	}
	return out
}
