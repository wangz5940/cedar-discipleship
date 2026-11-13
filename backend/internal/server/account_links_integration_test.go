//go:build integration

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/checkin"
	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func TestAccountLinkReviewSwitchAndCredentialScope(t *testing.T) {
	a, db := passwordSessionFixture(t)
	ctx := t.Context()
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at) VALUES(2,'Second tenant',1,NOW(),NOW());
		UPDATE study_groups SET tenant_id=2 WHERE id=2;
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at) VALUES(2,5,'member',1,NOW(),NOW())`)
	before, err := a.users.CurrentUser(ctx, 1, 1)
	if err != nil || len(before.Groups) != 1 || before.ID != 1 {
		t.Fatalf("original member = %+v, err %v", before, err)
	}
	review, err := a.users.CreateAccountLinkReview(ctx, "single", "left", 4)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := a.users.CurrentUser(ctx, 1, 1)
	if err != nil || len(pending.Groups) != 1 {
		t.Fatalf("pending review grants access: %+v %v", pending, err)
	}
	approvals := make(chan error, 2)
	for range 2 {
		go func() { approvals <- a.users.ReviewAccountLink(ctx, review.ID, 4, true) }()
	}
	for range 2 {
		if err := <-approvals; err != nil {
			t.Fatal(err)
		}
	}
	for _, loginID := range []uint64{1, 5} {
		for groupID, memberID := range map[uint64]uint64{1: 1, 2: 5} {
			current, err := a.users.CurrentUser(ctx, loginID, groupID)
			if err != nil || len(current.Groups) != 2 || current.ID != memberID || current.LoginAccountID() != loginID {
				t.Fatalf("login %d group %d = %+v %v", loginID, groupID, current, err)
			}
		}
	}
	var sessionID uint64
	if err := db.QueryRow(`SELECT id FROM refresh_sessions WHERE user_id=1 ORDER BY id LIMIT 1`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	token, err := a.signToken(tokenClaims{UserID: 1, SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/auth/switch-group", strings.NewReader(`{"group_id":2}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	a.auth(a.handleSwitchGroup)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("switch = %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Token string      `json:"token"`
		User  user.UserVO `json:"user"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.User.ID != 5 || result.User.LoginAccountID() != 1 || result.User.CurrentGroupID != 2 {
		t.Fatalf("switched user = %+v", result.User)
	}
	claims, err := a.verifyToken(result.Token)
	if err != nil || claims.UserID != 1 {
		t.Fatalf("credential owner changed: %+v %v", claims, err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	request.Header.Set("Authorization", "Bearer "+result.Token)
	response = httptest.NewRecorder()
	a.auth(a.handleMe)(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"account_id":1`) {
		t.Fatalf("me = %d %s", response.Code, response.Body.String())
	}
	a.location = time.UTC
	a.checkins = checkin.NewService(checkin.NewMySQLRepository(db))
	testdb.Exec(t, db, `INSERT INTO checkin_records(group_id,user_id,logical_date,checkin_time,task_type,detail,created_by,created_at,updated_at)
		VALUES(2,5,'2026-11-01',NOW(),'daily_bible','original history',5,NOW(),NOW())`)
	request = httptest.NewRequest(http.MethodGet, "/api/checkins?from=2026-11-01&to=2026-11-01&user_id=5", nil)
	request.Header.Set("Authorization", "Bearer "+result.Token)
	response = httptest.NewRecorder()
	a.auth(a.handleListCheckins)(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"user_id":5`) || !strings.Contains(response.Body.String(), "original history") {
		t.Fatalf("original history = %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/api/personal-settings", strings.NewReader(`{"member_name":"Linked group name","mobile_view_mode":"stacked"}`))
	request.Header.Set("Authorization", "Bearer "+result.Token)
	response = httptest.NewRecorder()
	a.auth(a.handleUpdatePersonalSettings)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("personal settings = %d %s", response.Code, response.Body.String())
	}
	var memberName string
	if err := db.QueryRow(`SELECT member_name FROM group_members WHERE group_id=2 AND user_id=5`).Scan(&memberName); err != nil || memberName != "Linked group name" {
		t.Fatalf("settings member scope = %q %v", memberName, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/auth/default-group", strings.NewReader(`{"group_id":2}`))
	request.Header.Set("Authorization", "Bearer "+result.Token)
	response = httptest.NewRecorder()
	a.auth(a.handleSetDefaultGroup)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("default group = %d %s", response.Code, response.Body.String())
	}
	var defaultGroup uint64
	if err := db.QueryRow(`SELECT default_group_id FROM users WHERE id=1`).Scan(&defaultGroup); err != nil || defaultGroup != 2 {
		t.Fatalf("login default = %d %v", defaultGroup, err)
	}
	refreshed, err := a.refreshSession(ctx, "user-1-device-0", "csrf")
	if err != nil || refreshed.UserID != 1 || refreshed.CurrentGroupID != 2 {
		t.Fatalf("refresh = %+v %v", refreshed, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	request.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "user-1-device-0"})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	request.Header.Set(csrfHeaderName, "csrf")
	response = httptest.NewRecorder()
	a.handleRefreshSession(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("refresh endpoint = %d %s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	claims, err = a.verifyToken(result.Token)
	if err != nil || claims.UserID != 1 || result.User.ID != 5 {
		t.Fatalf("refreshed identity = %+v %+v %v", claims, result.User, err)
	}
	if _, err := a.users.SetGroupDefaultPassword(ctx, 1, "changed", time.Now()); err != nil {
		t.Fatal(err)
	}
	if hash, err := a.users.PasswordHash(ctx, 1); err != nil || hash != "old" {
		t.Fatalf("linked password was overwritten: %q %v", hash, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM group_members WHERE (group_id=1 AND user_id=1) OR (group_id=2 AND user_id=5)`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("members changed: %d %v", count, err)
	}
	ownerHash, err := hashPassword("owner-old-password")
	if err != nil {
		t.Fatal(err)
	}
	otherHash, err := hashPassword("other-old-password")
	if err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, `UPDATE users SET password_hash=? WHERE id=1`, ownerHash)
	testdb.Exec(t, db, `UPDATE users SET password_hash=? WHERE id=5`, otherHash)
	request = httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{"old_password":"owner-old-password","new_password":"owner-new-password"}`))
	request.Header.Set("Authorization", "Bearer "+result.Token)
	response = httptest.NewRecorder()
	a.auth(a.handleChangePassword)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("credential password = %d %s", response.Code, response.Body.String())
	}
	ownerHash, err = a.users.PasswordHash(ctx, 1)
	if err != nil || !verifyPassword("owner-new-password", ownerHash) {
		t.Fatalf("login password scope failed: %v", err)
	}
	unchangedHash, err := a.users.PasswordHash(ctx, 5)
	if err != nil || unchangedHash != otherHash {
		t.Fatalf("other credential changed: %v", err)
	}
}

func TestAccountLinkRejectsStaleOverlapAndUnauthorizedReview(t *testing.T) {
	a, db := passwordSessionFixture(t)
	ctx := t.Context()
	for _, names := range [][2]string{{"single", "single"}, {"single", "both"}, {"single", "super"}, {"missing", "left"}} {
		if _, err := a.users.CreateAccountLinkReview(ctx, names[0], names[1], 4); err == nil {
			t.Fatalf("accepted invalid pair %v", names)
		}
	}
	review, err := a.users.CreateAccountLinkReview(ctx, "single", "left", 4)
	if err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, `UPDATE group_members SET member_name='Updated' WHERE user_id=5`)
	// A credential-name change invalidates the preview before approval.
	testdb.Exec(t, db, `UPDATE users SET display_name='Updated' WHERE id=5`)
	if err := a.users.ReviewAccountLink(ctx, review.ID, 4, true); err != user.ErrAccountLinkChanged {
		t.Fatalf("stale review = %v", err)
	}
	if err := a.users.ReviewAccountLink(ctx, review.ID, 4, false); err != nil {
		t.Fatal(err)
	}
	if err := a.users.ReviewAccountLink(ctx, review.ID, 4, true); err == nil {
		t.Fatal("approved rejected review")
	}
	var sessionID uint64
	if err := db.QueryRow(`SELECT id FROM refresh_sessions WHERE user_id=1 ORDER BY id LIMIT 1`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	token, err := a.signToken(tokenClaims{UserID: 1, SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/super-admin/account-links", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	a.auth(a.requireSuper(a.handleAccountLinkReviews))(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member accessed reviews: %d", response.Code)
	}
}
