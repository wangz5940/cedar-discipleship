package user

import (
	"context"
	"time"
)

type Repository interface {
	FindByID(ctx context.Context, id uint64) (*User, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	ListAllGroups(ctx context.Context) ([]Group, error)
	CreateGroup(ctx context.Context, code, name, description, passwordHash string, actorID uint64, at time.Time) (uint64, error)
	ListUsers(ctx context.Context, limit int) ([]UserListItem, error)
	ListGroups(ctx context.Context, userID uint64, isSuperAdmin bool) ([]Group, error)
	ListRoles(ctx context.Context, userID, groupID uint64) ([]string, error)
	ListMembers(ctx context.Context, groupID uint64) ([]Member, error)
	CreateMember(ctx context.Context, groupID, actorID uint64, input CreateMemberInput) (uint64, error)
	AdminMember(ctx context.Context, groupID, memberID uint64) (*AdminMember, error)
	RemoveMember(ctx context.Context, groupID, memberID, userID uint64, at time.Time) error
	SetRole(ctx context.Context, groupID, userID uint64, role string, grant bool, at time.Time) error
	ResetNonSuperPasswords(ctx context.Context, passwordHash string, at time.Time) (int64, error)
	SetGroupDefaultPassword(ctx context.Context, groupID uint64, passwordHash string, at time.Time) (int64, error)
	GroupDefaultPasswordHash(ctx context.Context, groupID uint64) (string, error)
	HasSuperAdmin(ctx context.Context) (bool, error)
	BootstrapSuperAdmin(ctx context.Context, username, displayName, namePinyin, passwordHash string, at time.Time) error
	CreateUserWithHash(ctx context.Context, username, displayName, namePinyin, passwordHash string, isSuperAdmin bool, actorID uint64, at time.Time) (uint64, error)
	AddMember(ctx context.Context, groupID, userID uint64, memberName string, actorID uint64, at time.Time) error
	UpdateLastLogin(ctx context.Context, userID uint64, at time.Time) error
	UpdateDefaultGroup(ctx context.Context, userID uint64, groupID uint64, updatedAt time.Time) error
	PasswordHash(ctx context.Context, userID uint64) (string, error)
	UpdatePassword(ctx context.Context, userID uint64, passwordHash string, updatedAt time.Time) error
	Save(ctx context.Context, user *User) error
}
