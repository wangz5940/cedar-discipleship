package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultPasswordEncryption(t *testing.T) {
	a := &app{secret: []byte("group-password-test-secret")}
	encrypted, err := a.encryptDefaultPassword("密码12345678")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := a.decryptDefaultPassword(encrypted)
	if err != nil || plain != "密码12345678" {
		t.Fatalf("round trip failed: %v", err)
	}
	other, err := a.encryptDefaultPassword(plain)
	if err != nil || other == encrypted {
		t.Fatal("encryption must use a fresh nonce")
	}
	wrongKey := &app{secret: []byte("different-secret")}
	if _, err := wrongKey.decryptDefaultPassword(encrypted); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, err := a.decryptDefaultPassword("invalid"); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
}

func TestDefaultPasswordAccessUsesGroupManagementPermissions(t *testing.T) {
	for _, item := range []struct {
		name    string
		user    currentUser
		allowed bool
	}{
		{name: "member"},
		{name: "admin", user: currentUser{Roles: []string{roleGroupAdmin}}, allowed: true},
		{name: "leader", user: currentUser{Roles: []string{roleGroupLeader}}, allowed: true},
		{name: "super", user: currentUser{IsSuperAdmin: true}, allowed: true},
		{name: "tenant admin", user: currentUser{IsTenantAdmin: true, CurrentGroupID: 1}, allowed: true},
		{name: "tenant admin without group", user: currentUser{IsTenantAdmin: true}},
	} {
		t.Run(item.name, func(t *testing.T) {
			a := &app{}
			called := false
			handler := a.requireRole(roleGroupAdmin, func(http.ResponseWriter, *http.Request) { called = true })
			req := httptest.NewRequest(http.MethodGet, "/api/admin/group/default-password", nil)
			req = req.WithContext(context.WithValue(req.Context(), currentUserKey, item.user))
			response := httptest.NewRecorder()
			handler(response, req)
			if called != item.allowed {
				t.Fatalf("allowed = %v, want %v", called, item.allowed)
			}
			if !item.allowed && response.Code != http.StatusForbidden {
				t.Fatal("member must receive forbidden")
			}
		})
	}
}
