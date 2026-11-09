//go:build integration

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func TestGroupDefaultPasswordEncryptedStorage(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "012_ministry_catalog_seed_policy.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,default_password_hash,created_at,updated_at)
		VALUES(1,'password-legacy','Legacy group','legacy-hash',NOW(),NOW())`)
	a := &app{db: db, secret: []byte("default-password-integration-secret"), users: user.NewService(user.NewMySQLRepository(db))}
	read := func(groupID uint64) (bool, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/admin/group/default-password", nil)
		req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{CurrentGroupID: groupID, Roles: []string{roleGroupAdmin}}))
		response := httptest.NewRecorder()
		a.requireRole(roleGroupAdmin, a.handleAdminGetGroupDefaultPassword)(response, req)
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected password response status or cache policy: %d", response.Code)
		}
		var result struct {
			Available bool   `json:"available"`
			Password  string `json:"password"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Available, result.Password
	}
	if available, _ := read(1); available {
		t.Fatal("legacy hash must not be exposed as a recoverable password")
	}
	password := "new-default-password"
	req := httptest.NewRequest(http.MethodPut, "/api/admin/group/default-password", nil)
	if _, err := a.setGroupDefaultPassword(1, password, false, 1, req); err != nil {
		t.Fatal(err)
	}
	if available, plain := read(1); !available || plain != password {
		t.Fatal("updated default password was not available to the group administrator")
	}
	var encrypted string
	if err := db.QueryRow(`SELECT default_password_encrypted FROM study_groups WHERE id=1`).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == "" || encrypted == password {
		t.Fatal("default password must be stored encrypted")
	}
	if _, err := a.users.SetGroupDefaultPassword(t.Context(), 1, "replacement-hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	if available, _ := read(1); available {
		t.Fatal("a hash-only update must clear the previous encrypted password")
	}
	id, err := a.users.CreateGroup(t.Context(), "Encrypted group", "new-group-hash", 1, time.Now(), encrypted)
	if err != nil {
		t.Fatal(err)
	}
	if available, plain := read(id); !available || plain != password {
		t.Fatal("new group did not preserve its encrypted default password")
	}
}
