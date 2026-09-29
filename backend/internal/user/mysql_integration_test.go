//go:build integration

package user

import (
	"context"
	"slices"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestMemberQueriesAndPasswordScope(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "011_refresh_sessions.sql")
	testdb.Apply(t, db, "016_refresh_session_group_version.sql")
	testdb.Apply(t, db, "013_member_personal_settings.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'a','A',NOW(),NOW()),(2,'b','B',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,password_hash,is_super_admin,created_at,updated_at)
		VALUES (1,'left','Left','left','old',0,NOW(),NOW()),
		       (2,'both','Both','both','old',0,NOW(),NOW()),
		       (3,'single','Single','single','old',0,NOW(),NOW()),
		       (4,'leader','Leader','leader','old',0,NOW(),NOW()),
		       (5,'super','Super','super','old',1,NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
		VALUES (1,1,'Left',0,NOW(),NOW(),NOW()),(2,1,'Left',1,NOW(),NOW(),NOW()),
		       (1,2,'Both',1,NOW(),NOW(),NOW()),(2,2,'Both',1,NOW(),NOW(),NOW()),
		       (1,3,'Single',1,NOW(),NOW(),NOW()),(1,4,'Leader',1,NOW(),NOW(),NOW()),
		       (1,5,'Super',1,NOW(),NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at)
		VALUES (1,4,'group_leader',NOW()),(2,3,'group_admin',NOW())`)
	testdb.Apply(t, db, "015_tenants.sql")
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at)
		VALUES(2,'Other Tenant',1,NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES(2,3,'admin',1,NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	t.Run("personal settings stay within membership", func(t *testing.T) {
		settings := PersonalSettings{
			MemberName:     "Only In A",
			MobileViewMode: MobileViewStacked,
		}
		if err := repo.UpdatePersonalSettings(t.Context(), 2, 1, settings, time.Now()); err != nil {
			t.Fatal(err)
		}
		got, err := repo.PersonalSettings(t.Context(), 2, 1)
		if err != nil {
			t.Fatal(err)
		}
		if got != settings {
			t.Fatalf("personal settings = %+v, want %+v", got, settings)
		}
		var username, displayName, otherGroupName string
		if err := db.QueryRow(`SELECT u.username,u.display_name,gm.member_name
			FROM users u JOIN group_members gm ON gm.user_id=u.id AND gm.group_id=2
			WHERE u.id=2`).Scan(&username, &displayName, &otherGroupName); err != nil {
			t.Fatal(err)
		}
		if username != "both" || displayName != "Both" || otherGroupName != "Both" {
			t.Fatalf("account or other group changed: username=%q display=%q other_group=%q", username, displayName, otherGroupName)
		}
	})
	t.Run("active membership reset boundary", func(t *testing.T) {
		count, err := repo.SetGroupDefaultPassword(t.Context(), 1, "new", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("reset %d users, want only the single active A member", count)
		}
		for id := 1; id <= 5; id++ {
			var hash string
			if err := db.QueryRow("SELECT password_hash FROM users WHERE id=?", id).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			want := "old"
			if id == 3 {
				want = "new"
			}
			if hash != want {
				t.Errorf("user %d hash %q, want %q", id, hash, want)
			}
		}
	})
	t.Run("members with one connection", func(t *testing.T) {
		db.SetMaxOpenConns(1)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		members, err := repo.ListMembers(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(members) != 4 {
			t.Fatalf("members = %v", members)
		}
		for i, member := range members {
			if member.UserID != uint64(i+2) || !slices.Contains(member.Roles, RoleMember) {
				t.Errorf("order/default role: %+v", member)
			}
			if member.UserID == 3 && slices.Contains(member.Roles, RoleGroupAdmin) {
				t.Error("roles from another group leaked")
			}
			if member.UserID == 4 && !slices.Contains(member.Roles, RoleGroupLeader) {
				t.Error("leader role missing")
			}
		}
	})
}
