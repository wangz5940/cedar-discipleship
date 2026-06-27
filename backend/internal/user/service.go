package user

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
)

var ErrUserNotFound = errors.New("user_not_found")
var ErrUsernameDisplayNameRequired = errors.New("username_display_name_required")
var ErrUserIDRequired = errors.New("user_id_required")
var ErrMemberNotFound = errors.New("member_not_found")
var ErrCannotRemoveSelf = errors.New("cannot_remove_self")
var ErrCannotRemoveSuperAdmin = errors.New("cannot_remove_super_admin")
var ErrCannotRemoveGroupLeader = errors.New("cannot_remove_group_leader")
var ErrCannotManageSelf = errors.New("cannot_manage_self")
var ErrCannotManageSuperAdmin = errors.New("cannot_manage_super_admin")
var ErrCannotManageGroupLeader = errors.New("cannot_manage_group_leader")
var ErrGroupDefaultPasswordMissing = errors.New("group_default_password_missing")
var ErrUserCreateFailed = errors.New("user_create_failed")
var ErrMemberAddFailed = errors.New("member_add_failed")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CurrentUser(ctx context.Context, userID, currentGroupID uint64) (UserVO, error) {
	item, err := s.repo.FindByID(ctx, userID)
	if err != nil || item.Status != 1 {
		return UserVO{}, ErrUserNotFound
	}
	groups, err := s.repo.ListGroups(ctx, userID, item.IsSuperAdmin)
	if err != nil {
		return UserVO{}, err
	}
	vo := toUserVO(*item)
	vo.Groups = groups
	if currentGroupID > 0 && ContainsGroup(groups, currentGroupID) {
		vo.CurrentGroupID = currentGroupID
	}
	if vo.CurrentGroupID > 0 {
		roles, err := s.repo.ListRoles(ctx, userID, vo.CurrentGroupID)
		if err != nil {
			return UserVO{}, err
		}
		vo.Roles = roles
	}
	return vo, nil
}

func (s *Service) LoginUser(ctx context.Context, username string) (*User, []Group, uint64, error) {
	item, err := s.repo.FindByUsername(ctx, username)
	if err != nil || item.Status != 1 {
		return nil, nil, 0, ErrUserNotFound
	}
	groups, err := s.repo.ListGroups(ctx, item.ID, item.IsSuperAdmin)
	if err != nil {
		return nil, nil, 0, err
	}
	var currentGroupID uint64
	if item.DefaultGroupID > 0 && ContainsGroup(groups, item.DefaultGroupID) {
		currentGroupID = item.DefaultGroupID
	} else if len(groups) == 1 {
		currentGroupID = groups[0].ID
	}
	return item, groups, currentGroupID, nil
}

func (s *Service) VisibleGroups(ctx context.Context, userID uint64, isSuperAdmin bool) ([]Group, error) {
	return s.repo.ListGroups(ctx, userID, isSuperAdmin)
}

func (s *Service) Roles(ctx context.Context, userID, groupID uint64) ([]string, error) {
	return s.repo.ListRoles(ctx, userID, groupID)
}

func (s *Service) Members(ctx context.Context, groupID uint64) ([]MemberVO, error) {
	members, err := s.repo.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]MemberVO, 0, len(members))
	for _, member := range members {
		out = append(out, MemberVO{
			MemberID:     member.MemberID,
			UserID:       member.UserID,
			Username:     member.Username,
			DisplayName:  member.DisplayName,
			MemberName:   firstNonEmpty(member.MemberName, member.DisplayName),
			IsSuperAdmin: member.IsSuperAdmin,
			Roles:        member.Roles,
		})
	}
	return out, nil
}

func (s *Service) AllGroups(ctx context.Context) ([]Group, error) {
	return s.repo.ListAllGroups(ctx)
}

func (s *Service) CreateGroup(ctx context.Context, code, name, description, passwordHash string, actorID uint64, at time.Time) (uint64, error) {
	return s.repo.CreateGroup(ctx, code, name, description, passwordHash, actorID, at)
}

func (s *Service) ListUsers(ctx context.Context, limit int) ([]UserListItemVO, error) {
	items, err := s.repo.ListUsers(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]UserListItemVO, 0, len(items))
	for _, item := range items {
		out = append(out, UserListItemVO{
			ID:           item.ID,
			Username:     item.Username,
			DisplayName:  item.DisplayName,
			IsSuperAdmin: item.IsSuperAdmin,
			Status:       item.Status,
		})
	}
	return out, nil
}

func (s *Service) CreateMember(ctx context.Context, groupID, actorID uint64, input CreateMemberInput) (uint64, error) {
	input.Username = normalizeUsername(firstNonEmpty(input.Username, input.NamePinyin, input.DisplayName))
	if input.CreateUser && (input.Username == "" || strings.TrimSpace(input.DisplayName) == "") {
		return 0, ErrUsernameDisplayNameRequired
	}
	if !input.CreateUser && input.UserID == 0 {
		return 0, ErrUserIDRequired
	}
	return s.repo.CreateMember(ctx, groupID, actorID, input)
}

func (s *Service) RemoveMember(ctx context.Context, groupID, memberID, actorID uint64, actorIsSuperAdmin bool, at time.Time) (uint64, error) {
	member, err := s.repo.AdminMember(ctx, groupID, memberID)
	if err != nil {
		return 0, ErrMemberNotFound
	}
	if member.UserID == actorID {
		return 0, ErrCannotRemoveSelf
	}
	if member.IsSuperAdmin && !actorIsSuperAdmin {
		return 0, ErrCannotRemoveSuperAdmin
	}
	if member.LeaderCount > 0 {
		return 0, ErrCannotRemoveGroupLeader
	}
	if err := s.repo.RemoveMember(ctx, groupID, memberID, member.UserID, at); err != nil {
		return 0, err
	}
	return member.UserID, nil
}

func (s *Service) SetRole(ctx context.Context, groupID, memberID, actorID uint64, actorIsSuperAdmin bool, role string, grant bool, at time.Time) error {
	member, err := s.repo.AdminMember(ctx, groupID, memberID)
	if err != nil {
		return ErrMemberNotFound
	}
	if member.UserID == actorID {
		return ErrCannotManageSelf
	}
	if member.IsSuperAdmin {
		return ErrCannotManageSuperAdmin
	}
	if member.LeaderCount > 0 {
		return ErrCannotManageGroupLeader
	}
	return s.repo.SetRole(ctx, groupID, member.UserID, role, grant, at)
}

func (s *Service) SetUserRole(ctx context.Context, groupID, userID uint64, role string, grant bool, at time.Time) error {
	return s.repo.SetRole(ctx, groupID, userID, role, grant, at)
}

func (s *Service) ResetNonSuperPasswords(ctx context.Context, passwordHash string, at time.Time) (int64, error) {
	return s.repo.ResetNonSuperPasswords(ctx, passwordHash, at)
}

func (s *Service) SetGroupDefaultPassword(ctx context.Context, groupID uint64, passwordHash string, at time.Time) (int64, error) {
	return s.repo.SetGroupDefaultPassword(ctx, groupID, passwordHash, at)
}

func (s *Service) GroupDefaultPasswordHash(ctx context.Context, groupID uint64) (string, error) {
	return s.repo.GroupDefaultPasswordHash(ctx, groupID)
}

func (s *Service) EnsureBootstrapSuperAdmin(ctx context.Context, username, displayName, passwordHash string, at time.Time) error {
	username = normalizeUsername(firstNonEmpty(username, displayName))
	displayName = firstNonEmpty(displayName, username)
	if username == "" || strings.TrimSpace(displayName) == "" {
		return ErrUsernameDisplayNameRequired
	}
	exists, err := s.repo.HasSuperAdmin(ctx)
	if err != nil || exists {
		return err
	}
	return s.repo.BootstrapSuperAdmin(ctx, username, displayName, username, passwordHash, at)
}

func (s *Service) CreateUserWithHash(ctx context.Context, username, displayName, namePinyin, passwordHash string, isSuperAdmin bool, actorID uint64, at time.Time) (uint64, error) {
	username = normalizeUsername(firstNonEmpty(username, namePinyin, displayName))
	if username == "" || strings.TrimSpace(displayName) == "" {
		return 0, ErrUsernameDisplayNameRequired
	}
	return s.repo.CreateUserWithHash(ctx, username, displayName, firstNonEmpty(namePinyin, username), passwordHash, isSuperAdmin, actorID, at)
}

func (s *Service) AddMember(ctx context.Context, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error {
	return s.repo.AddMember(ctx, groupID, userID, memberName, actorID, at)
}

func (s *Service) RecordLogin(ctx context.Context, userID uint64, at time.Time) error {
	return s.repo.UpdateLastLogin(ctx, userID, at)
}

func (s *Service) SetDefaultGroup(ctx context.Context, userID, groupID uint64, at time.Time) error {
	return s.repo.UpdateDefaultGroup(ctx, userID, groupID, at)
}

func (s *Service) PasswordHash(ctx context.Context, userID uint64) (string, error) {
	return s.repo.PasswordHash(ctx, userID)
}

func (s *Service) UpdatePassword(ctx context.Context, userID uint64, passwordHash string, at time.Time) error {
	return s.repo.UpdatePassword(ctx, userID, passwordHash, at)
}

func ContainsGroup(groups []Group, id uint64) bool {
	for _, group := range groups {
		if group.ID == id {
			return true
		}
	}
	return false
}

func toUserVO(item User) UserVO {
	return UserVO{
		ID:                 item.ID,
		Username:           item.Username,
		DisplayName:        item.DisplayName,
		IsSuperAdmin:       item.IsSuperAdmin,
		DefaultGroupID:     item.DefaultGroupID,
		MustChangePassword: item.MustChangePassword,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func normalizeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
