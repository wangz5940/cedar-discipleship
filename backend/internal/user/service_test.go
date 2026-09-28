package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

type createMemberTestRepository struct {
	Repository
	existingUser *User
	groups       []Group
	findErr      error
	createdInput CreateMemberInput
	created      bool
	createErr    error
}

type personalSettingsTestRepository struct {
	Repository
	user            *User
	groups          []Group
	settings        PersonalSettings
	settingsErr     error
	loadedUserID    uint64
	loadedGroupID   uint64
	updatedSettings PersonalSettings
	updateAt        time.Time
	updateErr       error
}

func (r *personalSettingsTestRepository) FindByID(context.Context, uint64) (*User, error) {
	return r.user, nil
}

func (r *personalSettingsTestRepository) ListGroups(context.Context, uint64, bool) ([]Group, error) {
	return r.groups, nil
}

func (r *personalSettingsTestRepository) ListRoles(context.Context, uint64, uint64) ([]string, error) {
	return []string{RoleMember}, nil
}

func (r *personalSettingsTestRepository) PersonalSettings(
	_ context.Context,
	userID, groupID uint64,
) (PersonalSettings, error) {
	r.loadedUserID = userID
	r.loadedGroupID = groupID
	return r.settings, r.settingsErr
}

func (r *personalSettingsTestRepository) UpdatePersonalSettings(
	_ context.Context,
	userID, groupID uint64,
	settings PersonalSettings,
	at time.Time,
) error {
	r.loadedUserID = userID
	r.loadedGroupID = groupID
	r.updatedSettings = settings
	r.updateAt = at
	return r.updateErr
}

func TestServiceCurrentUserIncludesCurrentGroupPersonalSettings(t *testing.T) {
	t.Parallel()

	repo := &personalSettingsTestRepository{
		user: &User{
			ID:          23,
			Username:    "account-name",
			DisplayName: "全局名称",
			Status:      1,
		},
		groups: []Group{{ID: 7, Code: "alpha", Name: "甲组"}},
		settings: PersonalSettings{
			MemberName:     "甲组名称",
			MobileViewMode: MobileViewStacked,
		},
	}

	got, err := NewService(repo).CurrentUser(context.Background(), 23, 7)
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	if got.Username != "account-name" || got.MemberName != "甲组名称" || got.MobileViewMode != MobileViewStacked {
		t.Fatalf("CurrentUser() = %+v", got)
	}
	if repo.loadedUserID != 23 || repo.loadedGroupID != 7 {
		t.Fatalf("settings scope = user %d group %d, want user 23 group 7", repo.loadedUserID, repo.loadedGroupID)
	}
}

func TestServiceUpdatePersonalSettingsValidatesAndScopesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings PersonalSettings
		wantErr  error
	}{
		{
			name: "blank member name",
			settings: PersonalSettings{
				MemberName:     "   ",
				MobileViewMode: MobileViewMasonry,
			},
			wantErr: ErrMemberNameRequired,
		},
		{
			name: "member name too long",
			settings: PersonalSettings{
				MemberName:     strings.Repeat("名", 129),
				MobileViewMode: MobileViewMasonry,
			},
			wantErr: ErrMemberNameTooLong,
		},
		{
			name: "unknown mobile view",
			settings: PersonalSettings{
				MemberName:     "组内名称",
				MobileViewMode: "carousel",
			},
			wantErr: ErrInvalidMobileViewMode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repo := &personalSettingsTestRepository{}
			_, err := NewService(repo).UpdatePersonalSettings(
				context.Background(),
				23,
				7,
				test.settings,
				time.Now(),
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("UpdatePersonalSettings() error = %v, want %v", err, test.wantErr)
			}
			if repo.loadedUserID != 0 {
				t.Fatal("repository called for invalid settings")
			}
		})
	}

	repo := &personalSettingsTestRepository{}
	at := time.Now()
	got, err := NewService(repo).UpdatePersonalSettings(
		context.Background(),
		23,
		7,
		PersonalSettings{MemberName: "  组内名称  ", MobileViewMode: MobileViewStacked},
		at,
	)
	if err != nil {
		t.Fatalf("UpdatePersonalSettings() error = %v", err)
	}
	if got.MemberName != "组内名称" || got.MobileViewMode != MobileViewStacked {
		t.Fatalf("UpdatePersonalSettings() = %+v", got)
	}
	if repo.loadedUserID != 23 || repo.loadedGroupID != 7 {
		t.Fatalf("update scope = user %d group %d, want user 23 group 7", repo.loadedUserID, repo.loadedGroupID)
	}
	if repo.updatedSettings != got || !repo.updateAt.Equal(at) {
		t.Fatalf("repository update = %+v at %v, want %+v at %v", repo.updatedSettings, repo.updateAt, got, at)
	}
}

func (r *createMemberTestRepository) FindByUsername(context.Context, string) (*User, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if r.existingUser == nil {
		return nil, ErrUserNotFound
	}
	return r.existingUser, nil
}

func (r *createMemberTestRepository) ListMembershipGroups(context.Context, uint64) ([]Group, error) {
	return r.groups, nil
}

func (r *createMemberTestRepository) CreateMember(
	_ context.Context,
	_, _ uint64,
	input CreateMemberInput,
) (uint64, error) {
	r.created = true
	r.createdInput = input
	if r.createErr != nil {
		return 0, r.createErr
	}
	return 42, nil
}

func TestServiceCreateMemberRequiresNameAndUsername(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input CreateMemberInput
	}{
		{
			name:  "missing username",
			input: CreateMemberInput{CreateUser: true, DisplayName: "张三"},
		},
		{
			name:  "missing display name",
			input: CreateMemberInput{CreateUser: true, Username: "zhangsan"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			repo := &createMemberTestRepository{}
			_, err := NewService(repo).CreateMember(context.Background(), 7, 9, test.input)
			if !errors.Is(err, ErrUsernameDisplayNameRequired) {
				t.Fatalf("CreateMember() error = %v, want %v", err, ErrUsernameDisplayNameRequired)
			}
			if repo.created {
				t.Fatal("repository CreateMember called for incomplete input")
			}
		})
	}
}

func TestServiceCreateMemberReturnsExistingAccountInformation(t *testing.T) {
	t.Parallel()

	repo := &createMemberTestRepository{
		existingUser: &User{
			ID:          23,
			Username:    "existing",
			DisplayName: "已有成员",
			Status:      1,
		},
		groups: []Group{
			{ID: 2, Code: "alpha", Name: "甲组", TenantID: 1},
			{ID: 5, Code: "beta", Name: "乙组", TenantID: 1},
		},
	}

	_, err := NewService(repo).CreateMember(context.Background(), 7, 9, CreateMemberInput{
		CreateUser:  true,
		TenantID:    1,
		DisplayName: "新成员",
		Username:    "Existing",
	})

	var conflict *UsernameConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("CreateMember() error = %v, want UsernameConflictError", err)
	}
	if conflict.ExistingUser.ID != 23 ||
		conflict.ExistingUser.Username != "existing" ||
		conflict.ExistingUser.DisplayName != "已有成员" {
		t.Fatalf("existing user = %+v", conflict.ExistingUser)
	}
	if len(conflict.ExistingUser.Groups) != 2 ||
		conflict.ExistingUser.Groups[0].Name != "甲组" ||
		conflict.ExistingUser.Groups[1].Name != "乙组" {
		t.Fatalf("existing user groups = %+v", conflict.ExistingUser.Groups)
	}
	if repo.created {
		t.Fatal("repository CreateMember called after username conflict")
	}
}

func TestServiceCreateMemberDoesNotRevealAnotherTenant(t *testing.T) {
	repo := &createMemberTestRepository{
		existingUser: &User{ID: 23, Username: "existing", Status: 1},
		groups:       []Group{{ID: 5, Name: "乙组", TenantID: 2}},
	}
	_, err := NewService(repo).CreateMember(context.Background(), 7, 9, CreateMemberInput{
		CreateUser: true, TenantID: 1, DisplayName: "新成员", Username: "existing",
	})
	if !errors.Is(err, ErrUsernameExists) {
		t.Fatalf("error = %v, want username conflict", err)
	}
	var details *UsernameConflictError
	if errors.As(err, &details) {
		t.Fatal("cross-tenant account details were exposed")
	}
}

func TestServiceCreateMemberPreservesExistingUserMembershipPath(t *testing.T) {
	t.Parallel()

	repo := &createMemberTestRepository{}
	userID, err := NewService(repo).CreateMember(context.Background(), 7, 9, CreateMemberInput{
		UserID:      23,
		DisplayName: "已有成员",
	})
	if err != nil {
		t.Fatalf("CreateMember() error = %v", err)
	}
	if userID != 42 {
		t.Fatalf("CreateMember() user ID = %d, want 42", userID)
	}
	if !repo.created || repo.createdInput.CreateUser || repo.createdInput.UserID != 23 {
		t.Fatalf("repository input = %+v", repo.createdInput)
	}
}

func TestIsDuplicateKeyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "username unique constraint",
			err:  &mysql.MySQLError{Number: 1062, Message: "duplicate entry"},
			want: true,
		},
		{
			name: "wrapped duplicate",
			err:  fmt.Errorf("insert user: %w", &mysql.MySQLError{Number: 1062, Message: "duplicate entry"}),
			want: true,
		},
		{
			name: "other database error",
			err:  &mysql.MySQLError{Number: 1205, Message: "lock wait timeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isDuplicateKeyError(test.err); got != test.want {
				t.Fatalf("isDuplicateKeyError() = %v, want %v", got, test.want)
			}
		})
	}
}
