package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	userdomain "agp/backend/internal/user"
)

type adminMemberTestRepository struct {
	userdomain.Repository
	existingUser *userdomain.User
	groups       []userdomain.Group
	createdInput userdomain.CreateMemberInput
	created      bool
}

func (r *adminMemberTestRepository) FindByUsername(context.Context, string) (*userdomain.User, error) {
	if r.existingUser == nil {
		return nil, userdomain.ErrUserNotFound
	}
	return r.existingUser, nil
}

func (r *adminMemberTestRepository) ListMembershipGroups(context.Context, uint64) ([]userdomain.Group, error) {
	return r.groups, nil
}

func (r *adminMemberTestRepository) CreateMember(
	_ context.Context,
	_, _ uint64,
	input userdomain.CreateMemberInput,
) (uint64, error) {
	r.created = true
	r.createdInput = input
	return input.UserID, nil
}

func TestHandleAdminCreateMemberReturnsExistingAccountDetails(t *testing.T) {
	t.Parallel()

	repo := &adminMemberTestRepository{
		existingUser: &userdomain.User{
			ID:           23,
			Username:     "existing",
			DisplayName:  "已有成员",
			PasswordHash: "must-not-leak",
			Status:       1,
		},
		groups: []userdomain.Group{
			{ID: 2, Code: "alpha", Name: "甲组"},
			{ID: 5, Code: "beta", Name: "乙组"},
		},
	}
	a := &app{users: userdomain.NewService(repo)}
	request := adminMemberRequest(t, `{
		"create_user": true,
		"display_name": "新成员",
		"username": "existing"
	}`)
	recorder := httptest.NewRecorder()

	a.handleAdminCreateMember(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var payload struct {
		Error        string                    `json:"error"`
		ExistingUser userdomain.ExistingUserVO `json:"existing_user"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != "username_exists" ||
		payload.ExistingUser.ID != 23 ||
		payload.ExistingUser.Username != "existing" ||
		payload.ExistingUser.DisplayName != "已有成员" ||
		len(payload.ExistingUser.Groups) != 2 ||
		payload.ExistingUser.Groups[0].Name != "甲组" ||
		payload.ExistingUser.Groups[1].Name != "乙组" {
		t.Fatalf("response = %+v", payload)
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("must-not-leak")) {
		t.Fatal("response leaked password hash")
	}
}

func TestHandleAdminCreateMemberConfirmsExistingAccountMembership(t *testing.T) {
	t.Parallel()

	repo := &adminMemberTestRepository{}
	a := &app{users: userdomain.NewService(repo)}
	request := adminMemberRequest(t, `{
		"create_user": false,
		"user_id": 23,
		"display_name": "已有成员"
	}`)
	recorder := httptest.NewRecorder()

	a.handleAdminCreateMember(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if !repo.created ||
		repo.createdInput.CreateUser ||
		repo.createdInput.UserID != 23 ||
		repo.createdInput.DisplayName != "已有成员" {
		t.Fatalf("repository input = %+v", repo.createdInput)
	}
}

func adminMemberRequest(t *testing.T, body string) *http.Request {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, "/api/admin/members", bytes.NewBufferString(body))
	user := currentUser{ID: 9, CurrentGroupID: 7}
	return request.WithContext(context.WithValue(request.Context(), currentUserKey, user))
}
