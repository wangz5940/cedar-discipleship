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

	"agp/backend/internal/audit"
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
	a := &app{db: db, users: user.NewService(user.NewMySQLRepository(db)), audits: audit.NewService(audit.NewMySQLRepository(db)), secret: []byte("tenant-test-secret")}
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
		status, data := call(http.MethodPost, "/api/super-admin/tenants", 1, 0, fmt.Sprintf(`{"name":%q}`, name))
		if status != http.StatusCreated {
			t.Fatalf("create tenant %s: %d %v", name, status, data)
		}
		tenantID := uint64(data["id"].(float64))
		var groupID uint64
		if group != "" {
			status, data = call(http.MethodPost, fmt.Sprintf("/api/tenants/%d/groups", tenantID), 1, 0, fmt.Sprintf(`{"name":%q}`, group))
			if status != http.StatusCreated {
				t.Fatalf("create group %s: %d %v", group, status, data)
			}
			groupID = uint64(data["id"].(float64))
		}
		if adminID != 0 {
			testdb.Exec(t, db, `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at) VALUES(?,?,'admin',1,NOW(),NOW())`, tenantID, adminID)
		}
		return tenantID, groupID
	}
	tenantA, groupA := create("主体 A", "共同组名", 2)
	tenantB, groupB := create("主体 B", "另一组", 3)
	testdb.Exec(t, db, `INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES(4,'membera','Member A','membera',NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at) VALUES(?,4,'member',1,NOW(),NOW())`, tenantA)
	testdb.Exec(t, db, `INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at) VALUES(?,4,'Member A',NOW(),NOW(),NOW())`, groupA)
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
	if status, data := call(http.MethodGet, path(tenantA, "admins"), 2, groupA, ""); status != http.StatusOK || len(data["admins"].([]any)) != 1 {
		t.Fatalf("ordinary member included among admins: %d %v", status, data)
	}
	if status, _ := call(http.MethodGet, path(tenantB, "admins"), 2, groupA, ""); status != http.StatusForbidden {
		t.Fatalf("other tenant admins visible: %d", status)
	}
	if status, _ := call(http.MethodPost, fmt.Sprintf("/api/super-admin/tenants/%d/admins", tenantA), 2, groupA, `{"username":"forbidden","display_name":"Forbidden"}`); status != http.StatusForbidden {
		t.Fatalf("tenant admin created another admin: %d", status)
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
	if status, _ := call(http.MethodDelete, path(tenantA, "members/3"), 1, 0, ""); status != http.StatusNotFound {
		t.Fatalf("obsolete member removal endpoint remains: %d", status)
	}
	if status, _ := call(http.MethodPut, path(tenantA, "members"), 1, 0, `{"user_id":4,"role":"admin"}`); status != http.StatusNotFound {
		t.Fatalf("obsolete member assignment endpoint remains: %d", status)
	}
	if status, _ := call(http.MethodGet, path(tenantA, "groups"), 3, groupB, ""); status != http.StatusForbidden {
		t.Fatalf("other tenant admin sees A: %d", status)
	}
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/tenants/%d", tenantA), 2, groupA, `{"name":"非法修改"}`); status != http.StatusForbidden {
		t.Fatalf("tenant admin updated tenant: %d", status)
	}
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/tenants/%d", tenantA), 1, 0, `{"name":"主体 A 新名"}`); status != http.StatusOK {
		t.Fatalf("rename tenant: %d", status)
	}
	if status, data := call(http.MethodGet, "/api/tenants", 1, 0, ""); status != http.StatusOK || data["tenants"] == nil {
		t.Fatalf("list renamed tenants: %d %v", status, data)
	}
	if status, _ := call(http.MethodDelete, fmt.Sprintf("/api/super-admin/tenants/%d", tenantA), 1, 0, ""); status != http.StatusConflict {
		t.Fatalf("deleted nonempty tenant: %d", status)
	}
	if status, _ := call(http.MethodDelete, "/api/super-admin/tenants/1", 1, 0, ""); status != http.StatusConflict {
		t.Fatalf("deleted original tenant: %d", status)
	}
	if status, data := call(http.MethodPost, fmt.Sprintf("/api/super-admin/tenants/%d/admins", tenantB), 1, 0, `{"username":"newadmin","display_name":"New Admin"}`); status != http.StatusCreated || data["initial_password"] == nil {
		t.Fatalf("create virtual tenant admin: %d %v", status, data)
	} else {
		newAdminID := uint64(data["id"].(float64))
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE user_id=?`, newAdminID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("virtual admin became check-in member: %d %v", count, err)
		}
		if status, _ := call(http.MethodPost, "/api/admin/members", 3, groupB, fmt.Sprintf(`{"create_user":false,"user_id":%d,"display_name":"New Admin"}`, newAdminID)); status != http.StatusConflict {
			t.Fatalf("tenant admin added as check-in member: %d", status)
		}
		if status, _ := call(http.MethodPost, fmt.Sprintf("/api/super-admin/groups/%d/members", groupA), 1, 0, fmt.Sprintf(`{"user_id":%d,"member_name":"New Admin"}`, newAdminID)); status != http.StatusConflict {
			t.Fatalf("super admin added virtual admin as check-in member: %d", status)
		}
		if status, data := call(http.MethodGet, path(tenantB, "admins"), 1, 0, ""); status != http.StatusOK || len(data["admins"].([]any)) != 2 {
			t.Fatalf("admin-only listing: %d %v", status, data)
		}
		if status, _ := call(http.MethodGet, path(tenantA, "groups"), newAdminID, groupB, ""); status != http.StatusForbidden {
			t.Fatalf("new admin accessed other tenant: %d", status)
		}
		if current, err := a.users.CurrentUser(t.Context(), newAdminID, groupB); err != nil || !current.IsTenantAdmin || current.CurrentTenantID != tenantB {
			t.Fatalf("virtual admin cannot manage own tenant: %+v %v", current, err)
		}
		if status, _ := call(http.MethodPost, fmt.Sprintf("/api/super-admin/tenants/%d/admins", tenantA), 1, 0, `{"username":"newadmin","display_name":"Duplicate"}`); status != http.StatusConflict {
			t.Fatalf("existing account promoted through create endpoint: %d", status)
		}
	}
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/groups/%d/tenant", groupA), 3, groupB, fmt.Sprintf(`{"tenant_id":%d}`, tenantB)); status != http.StatusForbidden {
		t.Fatalf("tenant admin moved group: %d", status)
	}
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/groups/%d/tenant", groupA), 1, 0, fmt.Sprintf(`{"tenant_id":%d}`, tenantB)); status != http.StatusConflict {
		t.Fatalf("group name collision on transfer: %d", status)
	}
	if status, _ := call(http.MethodPut, path(tenantA, fmt.Sprintf("groups/%d", groupA)), 1, 0, `{"name":"待转移组"}`); status != http.StatusOK {
		t.Fatalf("rename group before transfer: %d", status)
	}
	testdb.Exec(t, db, `INSERT INTO asset_dependencies(consumer_group_id,consumer_asset_id,provider_group_id,provider_asset_id,dependency_type,status,created_at,updated_at)
		VALUES(?,9001,?,9002,'import','active',NOW(),NOW())`, groupA, groupB)
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/groups/%d/tenant", groupA), 1, 0, fmt.Sprintf(`{"tenant_id":%d}`, tenantB)); status != http.StatusConflict {
		t.Fatalf("moved group with cross-group resource dependency: %d", status)
	}
	testdb.Exec(t, db, `DELETE FROM asset_dependencies WHERE consumer_asset_id=9001`)
	if status, _ := call(http.MethodPut, fmt.Sprintf("/api/super-admin/groups/%d/tenant", groupA), 1, 0, fmt.Sprintf(`{"tenant_id":%d}`, tenantB)); status != http.StatusOK {
		t.Fatalf("move group: %d", status)
	}
	if status, _ := call(http.MethodGet, path(tenantA, "groups"), 2, groupA, ""); status != http.StatusForbidden {
		t.Fatalf("source admin retained moved group: %d", status)
	}
	if status, data := call(http.MethodGet, path(tenantB, "groups"), 4, groupA, ""); status != http.StatusForbidden {
		t.Fatalf("regular member accessed admin endpoint: %d %v", status, data)
	}
	if moved, err := a.users.CurrentUser(t.Context(), 4, groupA); err != nil || moved.CurrentTenantID != tenantB {
		t.Fatalf("moved member lost group: %+v %v", moved, err)
	}
	if status, data := call(http.MethodGet, path(tenantB, "groups"), 3, groupB, ""); status != http.StatusOK || len(data["study_groups"].([]any)) < 2 {
		t.Fatalf("moved member cannot access target tenant: %d %v", status, data)
	}
	if status, _ := call(http.MethodDelete, fmt.Sprintf("/api/super-admin/tenants/%d", tenantA), 1, 0, ""); status != http.StatusConflict {
		t.Fatalf("deleted tenant with another group: %d", status)
	}
	if status, _ := call(http.MethodDelete, fmt.Sprintf("/api/super-admin/tenants/%d", tenantB), 3, groupB, ""); status != http.StatusForbidden {
		t.Fatalf("tenant admin deleted tenant: %d", status)
	}
	tenantC, _ := create("空主体", "", 0)
	if status, _ := call(http.MethodDelete, fmt.Sprintf("/api/super-admin/tenants/%d", tenantC), 1, 0, ""); status != http.StatusOK {
		t.Fatalf("delete empty tenant: %d", status)
	}
	if status, data := call(http.MethodGet, "/api/tenants", 1, 0, ""); status != http.StatusOK || len(data["tenants"].([]any)) != 3 {
		t.Fatalf("deleted tenant remains visible: %d %v", status, data)
	}
	if status, _ := call(http.MethodPost, "/api/super-admin/tenants", 1, 0, `{"name":"  "}`); status != http.StatusBadRequest {
		t.Fatalf("created tenant without name: %d", status)
	}
	status, emptyTenant := call(http.MethodPost, "/api/super-admin/tenants", 1, 0, `{"name":"仅主体名称"}`)
	if status != http.StatusCreated || emptyTenant["group_id"] != nil || emptyTenant["default_password"] != nil {
		t.Fatalf("create tenant without group: %d %v", status, emptyTenant)
	}
	emptyTenantID := uint64(emptyTenant["id"].(float64))
	if status, data := call(http.MethodGet, path(emptyTenantID, "groups"), 1, 0, ""); status != http.StatusOK || len(data["study_groups"].([]any)) != 0 {
		t.Fatalf("new tenant unexpectedly has a group: %d %v", status, data)
	}
	if status, data := call(http.MethodPost, fmt.Sprintf("/api/super-admin/tenants/%d/admins", emptyTenantID), 1, 0, `{"username":"emptyadmin","display_name":"Empty Admin"}`); status != http.StatusCreated || data["initial_password"] == nil {
		t.Fatalf("create admin before group: %d %v", status, data)
	}
	if status, data := call(http.MethodGet, path(emptyTenantID, "admins"), 1, 0, ""); status != http.StatusOK || len(data["admins"].([]any)) != 1 {
		t.Fatalf("new tenant admin missing: %d %v", status, data)
	}
	if status, data := call(http.MethodPost, path(emptyTenantID, "groups"), 1, 0, `{"name":"稍后创建的小组"}`); status != http.StatusCreated || data["default_password"] == nil {
		t.Fatalf("create group after tenant: %d %v", status, data)
	}
}
