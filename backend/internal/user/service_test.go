package user

import (
	"context"
	"errors"
	"fmt"
	"testing"

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
			{ID: 2, Code: "alpha", Name: "甲组"},
			{ID: 5, Code: "beta", Name: "乙组"},
		},
	}

	_, err := NewService(repo).CreateMember(context.Background(), 7, 9, CreateMemberInput{
		CreateUser:  true,
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
