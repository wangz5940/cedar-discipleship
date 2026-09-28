//go:build integration

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func TestTenantHTTPIsolationAndAdministration(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "011_refresh_sessions.sql")
	testdb.Apply(t, db, "012_ministry_catalog_seed_policy.sql")
	testdb.Apply(t, db, "013_member_personal_settings.sql")
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,is_super_admin,created_at,updated_at)
		VALUES(1,'super','Super','super',1,NOW(),NOW()),(2,'admina','Admin A','admina',0,NOW(),NOW()),
		(3,'adminb','Admin B','adminb',0,NOW(),NOW())`)
	a := &app{db: db, users: user.NewService(user.NewMySQLRepository(db)), secret: []byte("tenant-test-secret")}
	mux := http.NewServeMux()
	a.routes(mux)
	call := func(method, path string, userID, groupID uint64, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		token, err := a.signToken(tokenClaims{UserID: userID, CurrentGroupID: groupID})
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, req)
		var data map[string]any
		_ = json.Unmarshal(res.Body.Bytes(), &data)
		return res.Code, data
	}
	create := func(name, group string, adminID uint64) (uint64, uint64) {
		t.Helper()
		body := fmt.Sprintf(`{"name":%q,"group_name":%q,"admin_user_id":%d}`, name, group, adminID)
		status, data := call(http.MethodPost, "/api/super-admin/tenants", 1, 0, body)
		if status != http.StatusCreated {
			t.Fatalf("create tenant %s: %d %v", name, status, data)
		}
		return uint64(data["id"].(float64)), uint64(data["group_id"].(float64))
	}
	tenantA, groupA := create("主体 A", "共同组名", 2)
	tenantB, groupB := create("主体 B", "另一组", 3)
	testdb.Exec(t, db, `INSERT INTO refresh_sessions(user_id,token_hash,csrf_hash,current_group_id,expires_at,created_at,updated_at)
		VALUES(2,?,?,?,DATE_ADD(NOW(),INTERVAL 1 DAY),NOW(),NOW())`, tokenHash("playback-token"), tokenHash("csrf"), groupA)
	playbackRequest := httptest.NewRequest(http.MethodGet, "/api/assets/1/stream", nil)
	playbackRequest.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "playback-token"})
	if !a.playbackSessionAllowed(playbackRequest, groupA) || a.playbackSessionAllowed(playbackRequest, groupB) {
		t.Fatal("playback session ignored selected group or tenant")
	}
	expires := time.Now().Add(time.Hour).Unix()
	url := fmt.Sprintf("/api/assets/1/stream?group_id=%d&expires=%d&signature=%s", groupA, expires, a.signAssetPlayback(1, groupA, expires))
	unauthenticatedStream := httptest.NewRequest(http.MethodGet, url, nil)
	unauthenticatedStream.SetPathValue("id", "1")
	streamResponse := httptest.NewRecorder()
	a.handleStreamAsset(streamResponse, unauthenticatedStream)
	if streamResponse.Code != http.StatusForbidden {
		t.Fatalf("signed stream without session: %d", streamResponse.Code)
	}
	testdb.Exec(t, db, `UPDATE refresh_sessions SET revoked_at=NOW() WHERE user_id=2`)
	if a.playbackSessionAllowed(playbackRequest, groupA) {
		t.Fatal("revoked session still streams assets")
	}
	path := func(tenantID uint64, suffix string) string {
		return fmt.Sprintf("/api/tenants/%d/%s", tenantID, suffix)
	}
	if status, data := call(http.MethodGet, path(tenantA, "groups"), 2, groupA, ""); status != http.StatusOK || len(data["study_groups"].([]any)) != 1 {
		t.Fatalf("own groups: %d %v", status, data)
	}
	if status, _ := call(http.MethodGet, path(tenantB, "groups"), 2, groupA, ""); status != http.StatusForbidden {
		t.Fatalf("other tenant groups: %d", status)
	}
	if status, _ := call(http.MethodPut, path(tenantB, "members"), 2, groupA, `{"user_id":2,"role":"admin"}`); status != http.StatusForbidden {
		t.Fatalf("other tenant members: %d", status)
	}
	if status, _ := call(http.MethodPut, path(tenantA, "members"), 2, groupA, `{"user_id":3,"role":"admin"}`); status != http.StatusForbidden {
		t.Fatalf("tenant admin invited external account: %d", status)
	}
	if status, _ := call(http.MethodPut, path(tenantA, "members"), 2, groupA, `{"user_id":2,"role":"member"}`); status != http.StatusConflict {
		t.Fatalf("last admin demotion: %d", status)
	}
	if status, data := call(http.MethodPost, path(tenantA, "groups"), 2, groupA, `{"name":"第二组"}`); status != http.StatusCreated {
		t.Fatalf("tenant admin create group: %d %v", status, data)
	}
	if status, data := call(http.MethodPost, path(tenantB, "groups"), 3, groupB, `{"name":"第二组"}`); status != http.StatusCreated {
		t.Fatalf("same group name in other tenant blocked: %d %v", status, data)
	}
	if status, data := call(http.MethodPut, path(tenantA, fmt.Sprintf("groups/%d", groupA)), 2, groupA, `{"name":"另一组"}`); status != http.StatusOK {
		t.Fatalf("same name in other tenant blocked: %d %v", status, data)
	}
	if status, _ := call(http.MethodPut, path(tenantA, "members"), 1, 0, `{"user_id":3,"role":"member"}`); status != http.StatusOK {
		t.Fatalf("super add shared account: %d", status)
	}
	if status, _ := call(http.MethodDelete, path(tenantA, "members/3"), 2, groupA, ""); status != http.StatusOK {
		t.Fatalf("tenant admin remove member: %d", status)
	}
	shared, err := a.users.CurrentUser(t.Context(), 3, groupB)
	if err != nil || shared.CurrentGroupID != groupB || shared.CurrentTenantID != tenantB {
		t.Fatalf("other tenant after removal: %+v %v", shared, err)
	}
	if status, _ := call(http.MethodGet, path(tenantA, "groups"), 3, groupB, ""); status != http.StatusForbidden {
		t.Fatalf("removed member still sees A: %d", status)
	}
}
