//go:build integration

package user

import (
	"errors"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestTenantMembershipAndGroupSelection(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "013_member_personal_settings.sql")
	testdb.Apply(t, db, "003_ministry_groups.sql")
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at) VALUES(2,'主体 B',1,NOW(),NOW());
		INSERT INTO study_groups(id,tenant_id,code,name,created_at,updated_at)
		VALUES (1,1,'a1','A1',NOW(),NOW()),(2,1,'a2','A2',NOW(),NOW()),(3,2,'b1','B1',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at)
		VALUES (1,'admin','Admin','admin',NOW(),NOW()),(2,'both','Both','both',NOW(),NOW()),(3,'onlyb','Only B','onlyb',NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,created_at,updated_at)
		VALUES (1,1,'admin',NOW(),NOW()),(1,2,'member',NOW(),NOW()),(2,2,'member',NOW(),NOW()),(2,3,'member',NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,joined_at,created_at,updated_at)
		VALUES (1,2,'Both',NOW(),NOW(),NOW()),(3,2,'Both',NOW(),NOW(),NOW()),(3,3,'Only B',NOW(),NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	service := NewService(repo)

	adminGroups, err := repo.ListGroups(t.Context(), 1, false)
	if err != nil || len(adminGroups) != 2 || adminGroups[0].ID != 1 || adminGroups[1].ID != 2 || !adminGroups[0].TenantAdmin {
		t.Fatalf("tenant admin groups = %+v, err = %v", adminGroups, err)
	}
	admin, err := service.CurrentUser(t.Context(), 1, 2)
	if err != nil || !admin.IsTenantAdmin || admin.CurrentTenantID != 1 || admin.CurrentGroupID != 2 {
		t.Fatalf("tenant admin context = %+v, err = %v", admin, err)
	}
	other, err := service.CurrentUser(t.Context(), 1, 3)
	if err != nil || other.CurrentGroupID != 0 || other.CurrentTenantID != 0 {
		t.Fatalf("cross-tenant selection = %+v, err = %v", other, err)
	}

	bothA, err := service.CurrentUser(t.Context(), 2, 1)
	if err != nil || bothA.CurrentTenantID != 1 || bothA.IsTenantAdmin {
		t.Fatalf("shared account in A = %+v, err = %v", bothA, err)
	}
	bothB, err := service.CurrentUser(t.Context(), 2, 3)
	if err != nil || bothB.CurrentTenantID != 2 || bothB.IsTenantAdmin {
		t.Fatalf("shared account in B = %+v, err = %v", bothB, err)
	}
	if _, err := repo.CreateMember(t.Context(), 1, 1, CreateMemberInput{UserID: 3, DisplayName: "Only B"}); err != nil {
		t.Fatalf("cross-tenant add member: %v", err)
	}
	onlyBGroups, err := repo.ListMembershipGroups(t.Context(), 3)
	if err != nil || len(onlyBGroups) != 2 || onlyBGroups[0].ID != 1 || onlyBGroups[1].ID != 3 {
		t.Fatalf("cross-tenant member groups = %+v, err = %v", onlyBGroups, err)
	}
	if _, err := repo.CreateMember(t.Context(), 3, 1, CreateMemberInput{UserID: 1, DisplayName: "Admin"}); !errors.Is(err, ErrMemberAddFailed) {
		t.Fatalf("cross-tenant tenant admin add error = %v, want %v", err, ErrMemberAddFailed)
	}
	if _, err := repo.CreateMember(t.Context(), 2, 1, CreateMemberInput{UserID: 2, DisplayName: "Both"}); err != nil {
		t.Fatalf("same-tenant add member: %v", err)
	}
	if err := repo.RemoveMember(t.Context(), 1, 1, 2, 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, err := service.CurrentUser(t.Context(), 2, 3)
	if err != nil || after.CurrentTenantID != 2 || after.CurrentGroupID != 3 {
		t.Fatalf("other tenant membership after A removal = %+v, err = %v", after, err)
	}
}

func TestEmptyTenantAdminContext(t *testing.T) {
	db := testdb.Open(t)
	testdb.Apply(t, db, "013_member_personal_settings.sql")
	testdb.Exec(t, db, `INSERT INTO tenants(id,name,status,created_at,updated_at) VALUES
		(2,'空小家',1,NOW(),NOW()),(3,'另一空小家',1,NOW(),NOW()),(4,'已停用小家',0,NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,created_at,updated_at) VALUES
		(10,'emptyadmin','Empty Admin','emptyadmin',NOW(),NOW()),
		(11,'emptymember','Empty Member','emptymember',NOW(),NOW()),
		(12,'multiadmin','Multi Admin','multiadmin',NOW(),NOW()),
		(13,'inactiveadmin','Inactive Admin','inactiveadmin',NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at) VALUES
		(2,10,'admin',1,NOW(),NOW()),
		(2,11,'member',1,NOW(),NOW()),
		(2,12,'admin',1,NOW(),NOW()),(3,12,'admin',1,NOW(),NOW()),
		(4,13,'admin',1,NOW(),NOW())`)
	repo := NewMySQLRepository(db)
	service := NewService(repo)

	tests := []struct {
		name            string
		userID          uint64
		wantTenantID    uint64
		wantTenantAdmin bool
	}{
		{name: "sole active admin", userID: 10, wantTenantID: 2, wantTenantAdmin: true},
		{name: "ordinary member", userID: 11},
		{name: "admin of multiple tenants", userID: 12},
		{name: "admin of inactive tenant", userID: 13},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tenantID, ok, err := repo.FindSoleTenantAdmin(t.Context(), tc.userID)
			if err != nil {
				t.Fatal(err)
			}
			if tenantID != tc.wantTenantID || ok != tc.wantTenantAdmin {
				t.Fatalf("repository context = tenant %d admin %t, want tenant %d admin %t",
					tenantID, ok, tc.wantTenantID, tc.wantTenantAdmin)
			}
			got, err := service.CurrentUser(t.Context(), tc.userID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got.CurrentTenantID != tc.wantTenantID || got.IsTenantAdmin != tc.wantTenantAdmin ||
				got.CurrentGroupID != 0 || len(got.Groups) != 0 {
				t.Fatalf("current user = %+v", got)
			}
		})
	}
}

func TestDefaultTenantNameMigration(t *testing.T) {
	db := testdb.Open(t)
	var name string
	if err := db.QueryRow(`SELECT name FROM tenants WHERE id=1`).Scan(&name); err != nil || name != "原有小家" {
		t.Fatalf("new default name = %q, err = %v", name, err)
	}
	testdb.Exec(t, db, `UPDATE tenants SET name='原有主体' WHERE id=1`)
	testdb.Apply(t, db, "015_tenants.sql")
	if err := db.QueryRow(`SELECT name FROM tenants WHERE id=1`).Scan(&name); err != nil || name != "原有小家" {
		t.Fatalf("legacy name after migration = %q, err = %v", name, err)
	}
	testdb.Exec(t, db, `UPDATE tenants SET name='自定义小家' WHERE id=1`)
	testdb.Apply(t, db, "015_tenants.sql")
	if err := db.QueryRow(`SELECT name FROM tenants WHERE id=1`).Scan(&name); err != nil || name != "自定义小家" {
		t.Fatalf("custom name after migration = %q, err = %v", name, err)
	}
}
