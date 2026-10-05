//go:build integration

package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agp/backend/internal/testdb"
	"agp/backend/internal/user"
)

func passwordSessionFixture(t *testing.T) (*app, *sql.DB) {
	t.Helper()
	db := testdb.Open(t)
	testdb.Apply(t, db, "011_refresh_sessions.sql")
	testdb.Apply(t, db, "016_refresh_session_group_version.sql")
	testdb.Apply(t, db, "016_refresh_session_group_version.sql")
	testdb.Apply(t, db, "013_member_personal_settings.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,password_hash,is_super_admin,created_at,updated_at)
		VALUES (1,'single','Single','single','old',0,NOW(),NOW()),(2,'both','Both','both','old',0,NOW(),NOW()),
		       (3,'leader','Leader','leader','old',0,NOW(),NOW()),(4,'super','Super','super','old',1,NOW(),NOW()),
		       (5,'left','Left','left','old',0,NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
		VALUES (1,1,'Single',1,NOW(),NOW(),NOW()),(1,2,'Both',1,NOW(),NOW(),NOW()),
		       (2,2,'Both',1,NOW(),NOW(),NOW()),(1,3,'Leader',1,NOW(),NOW(),NOW()),
		       (1,4,'Super',1,NOW(),NOW(),NOW()),(1,5,'Left',0,NOW(),NOW(),NOW()),
		       (2,5,'Left',1,NOW(),NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at) VALUES (1,3,'group_leader',NOW())`)
	testdb.Apply(t, db, "015_tenants.sql")
	for id := 1; id <= 5; id++ {
		for device := 0; device < 2; device++ {
			token := fmt.Sprintf("user-%d-device-%d", id, device)
			testdb.Exec(t, db, `INSERT INTO refresh_sessions(user_id,token_hash,csrf_hash,expires_at,created_at,updated_at)
				VALUES (?,?,?,DATE_ADD(NOW(),INTERVAL 1 DAY),NOW(),NOW())`, id, tokenHash(token), tokenHash("csrf"))
		}
	}
	return &app{db: db, users: user.NewService(user.NewMySQLRepository(db)), secret: []byte("test-secret")}, db
}

func TestPasswordResetRevokesOnlyAffectedSessions(t *testing.T) {
	for _, scope := range []string{"group", "all"} {
		t.Run(scope, func(t *testing.T) {
			a, db := passwordSessionFixture(t)
			var err error
			if scope == "group" {
				_, err = a.users.SetGroupDefaultPassword(t.Context(), 1, "new", time.Now())
			} else {
				_, err = a.users.ResetNonSuperPasswords(t.Context(), "new", time.Now())
			}
			if err != nil {
				t.Fatal(err)
			}
			for id := 1; id <= 5; id++ {
				revoked := id == 1 || (scope == "all" && id != 4)
				var hash string
				if err := db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, id).Scan(&hash); err != nil {
					t.Fatal(err)
				}
				if (hash == "new") != revoked {
					t.Errorf("user %d hash reset=%v, want %v", id, hash == "new", revoked)
				}
				for device := 0; device < 2; device++ {
					_, err := a.refreshSession(t.Context(), fmt.Sprintf("user-%d-device-%d", id, device), "csrf")
					if (err != nil) != revoked {
						t.Errorf("user %d device %d refresh err=%v, revoked=%v", id, device, err, revoked)
					}
					active, err := a.activeRefreshSession(t.Context(), uint64((id-1)*2+device+1), uint64(id), 0, 0)
					if err != nil || active == revoked {
						t.Errorf("user %d device %d active=%v err=%v", id, device, active, err)
					}
				}
			}
		})
	}
}

func TestResetSelectedMemberPassword(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		target, group                   uint64
		super, ordinary, missingDefault bool
		want                            int
	}{
		{name: "member with multiple groups", target: 2, group: 1, want: 200},
		{name: "wrong group", target: 1, group: 2, want: 404},
		{name: "inactive member", target: 5, group: 1, want: 404},
		{name: "ordinary actor", target: 1, group: 1, ordinary: true, want: 403},
		{name: "protected administrator", target: 3, group: 1, want: 403},
		{name: "super resets administrator", target: 3, group: 1, super: true, want: 200},
		{name: "protected super", target: 4, group: 1, super: true, want: 403},
		{name: "missing default", target: 1, group: 1, missingDefault: true, want: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, db := passwordSessionFixture(t)
			if !tc.missingDefault {
				testdb.Exec(t, db, `UPDATE study_groups SET default_password_hash='group-default'`)
			}
			var memberID uint64
			if err := db.QueryRow(`SELECT id FROM group_members WHERE group_id=1 AND user_id=?`, tc.target).Scan(&memberID); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.SetPathValue("id", fmt.Sprint(memberID))
			actor := currentUser{ID: 3, CurrentGroupID: tc.group, IsSuperAdmin: tc.super}
			if !tc.ordinary {
				actor.Roles = []string{roleGroupAdmin}
			}
			req = req.WithContext(context.WithValue(req.Context(), currentUserKey, actor))
			response := httptest.NewRecorder()
			a.requireRole(roleGroupAdmin, a.handleAdminResetMemberPassword)(response, req)
			if response.Code != tc.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			for id := uint64(1); id <= 5; id++ {
				changed := tc.want == 200 && id == tc.target
				var hash string
				var mustChange bool
				var revoked int
				if err := db.QueryRow(`SELECT password_hash,must_change_password FROM users WHERE id=?`, id).Scan(&hash, &mustChange); err != nil {
					t.Fatal(err)
				}
				if err := db.QueryRow(`SELECT COUNT(*) FROM refresh_sessions WHERE user_id=? AND revoked_at IS NOT NULL`, id).Scan(&revoked); err != nil {
					t.Fatal(err)
				}
				if (hash == "group-default") != changed || mustChange != changed || (revoked == 2) != changed {
					t.Errorf("user=%d hash_changed=%v must_change=%v revoked=%d", id, hash == "group-default", mustChange, revoked)
				}
			}
		})
	}
}

func TestChangePasswordRejectsOldCookiesAndAccessToken(t *testing.T) {
	a, db := passwordSessionFixture(t)
	hash, err := hashPassword("old-password")
	if err != nil {
		t.Fatal(err)
	}
	testdb.Exec(t, db, `UPDATE users SET password_hash=? WHERE id=1`, hash)
	legacyToken, err := a.signToken(tokenClaims{UserID: 1, CurrentGroupID: 1})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1}))
	response := httptest.NewRecorder()
	a.handleChangePassword(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", response.Code, response.Body)
	}
	refresh := httptest.NewRequest(http.MethodPost, "/", nil)
	refresh.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "user-1-device-0"})
	refresh.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "csrf"})
	refresh.Header.Set(csrfHeaderName, "csrf")
	response = httptest.NewRecorder()
	a.handleRefreshSession(response, refresh)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("old refresh: %d", response.Code)
	}
	token, err := a.signToken(tokenClaims{UserID: 1, SessionID: 1})
	if err != nil {
		t.Fatal(err)
	}
	access := httptest.NewRequest(http.MethodGet, "/", nil)
	access.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	a.auth(a.handleMe)(response, access)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("old access: %d", response.Code)
	}
	legacyAccess := httptest.NewRequest(http.MethodGet, "/", nil)
	legacyAccess.Header.Set("Authorization", "Bearer "+legacyToken)
	response = httptest.NewRecorder()
	a.auth(a.handleMe)(response, legacyAccess)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("legacy access without session: %d", response.Code)
	}
}

func TestRefreshSessionGroupRejectsStaleCurrentGroup(t *testing.T) {
	a, db := passwordSessionFixture(t)
	var sessionID uint64
	if err := db.QueryRow(`SELECT id FROM refresh_sessions WHERE user_id=2 ORDER BY id LIMIT 1`).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	staleToken, err := a.signToken(tokenClaims{UserID: 2, SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	version, err := a.updateRefreshSessionGroup(t.Context(), sessionID, 2, 0, 2)
	if err != nil {
		t.Fatalf("current switch failed: %v", err)
	}
	if version != 1 {
		t.Fatalf("session version = %d, want 1", version)
	}
	if _, err := a.updateRefreshSessionGroup(t.Context(), sessionID, 2, 0, 1); !errors.Is(err, errRefreshSessionGroupChanged) {
		t.Fatalf("stale switch error = %v, want %v", err, errRefreshSessionGroupChanged)
	}
	var groupID, groupVersion uint64
	if err := db.QueryRow(`SELECT current_group_id,group_version FROM refresh_sessions WHERE id=?`, sessionID).
		Scan(&groupID, &groupVersion); err != nil {
		t.Fatal(err)
	}
	if groupID != 2 || groupVersion != 1 {
		t.Fatalf("session group/version = %d/%d, want 2/1", groupID, groupVersion)
	}
	for _, test := range []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "stale token", token: staleToken, wantStatus: http.StatusUnauthorized},
		{
			name: "current token",
			token: func() string {
				token, err := a.signToken(tokenClaims{
					UserID: 2, CurrentGroupID: 2, SessionID: sessionID, GroupVersion: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				return token
			}(),
			wantStatus: http.StatusOK,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("Authorization", "Bearer "+test.token)
			response := httptest.NewRecorder()
			a.auth(a.handleMe)(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestPasswordResetRollsBackIfRevocationFails(t *testing.T) {
	a, db := passwordSessionFixture(t)
	testdb.Exec(t, db, `CREATE TRIGGER reject_revocation BEFORE UPDATE ON refresh_sessions
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test revocation failure'`)
	if _, err := a.users.SetGroupDefaultPassword(t.Context(), 1, "new", time.Now()); err == nil {
		t.Fatal("reset succeeded despite failed revocation")
	}
	var hash, groupHash string
	if err := db.QueryRow(`SELECT u.password_hash,g.default_password_hash
		FROM users u JOIN study_groups g ON g.id=1 WHERE u.id=1`).Scan(&hash, &groupHash); err != nil {
		t.Fatal(err)
	}
	if hash != "old" || groupHash != "" {
		t.Fatalf("partial reset committed: user=%q group=%q", hash, groupHash)
	}
}

func TestLoginCannotCreateSessionWithSupersededPassword(t *testing.T) {
	a, db := passwordSessionFixture(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), `UPDATE users SET password_hash='new' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		response := httptest.NewRecorder()
		_, err := a.issueRefreshSession(ctx, response, httptest.NewRequest(http.MethodPost, "/", nil), 1, 1, "old")
		done <- err
	}()
	// The update is uncommitted while the login attempts to create its session.
	testdb.WaitForLockWait(t, db)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != user.ErrPasswordChanged {
		t.Fatalf("stale login error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM refresh_sessions WHERE user_id=1`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("stale login created a session: count=%d", count)
	}
	response := httptest.NewRecorder()
	if _, err := a.issueRefreshSession(t.Context(), response, httptest.NewRequest(http.MethodPost, "/", nil), 1, 1, "new"); err != nil {
		t.Fatalf("current password cannot create session: %v", err)
	}
	if len(response.Result().Cookies()) != 3 {
		t.Fatal("new login missing refresh/playback/CSRF cookies")
	}
	if err := a.users.UpdatePassword(t.Context(), 1, "old", "stale-change", time.Now()); err != user.ErrPasswordChanged {
		t.Fatalf("stale password change error = %v", err)
	}
}
