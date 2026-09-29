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
	if _, err := repo.CreateMember(t.Context(), 1, 1, CreateMemberInput{UserID: 3, DisplayName: "Only B"}); !errors.Is(err, ErrMemberAddFailed) {
		t.Fatalf("cross-tenant add member error = %v", err)
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
