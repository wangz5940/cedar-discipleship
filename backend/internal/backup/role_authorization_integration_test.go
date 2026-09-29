//go:build integration

package backup

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"agp/backend/internal/testdb"
)

func TestImportLocalBackupLeaderRoleAuthorization(t *testing.T) {
	tests := []struct {
		name           string
		actorID        uint64
		leaderRoles    []string
		adminRoles     []string
		candidateRoles []string
		wantErr        bool
		wantLeaders    []string
		wantAdmins     []string
	}{
		{
			name:           "group admin cannot add leader",
			actorID:        2,
			leaderRoles:    []string{roleGroupLeader},
			adminRoles:     []string{roleGroupAdmin},
			candidateRoles: []string{roleGroupLeader},
			wantErr:        true,
			wantLeaders:    []string{"leader"},
			wantAdmins:     []string{"admin"},
		},
		{
			name:        "group admin cannot remove leader",
			actorID:     2,
			adminRoles:  []string{roleGroupAdmin},
			wantErr:     true,
			wantLeaders: []string{"leader"},
			wantAdmins:  []string{"admin"},
		},
		{
			name:           "group admin can restore unchanged leaders",
			actorID:        2,
			leaderRoles:    []string{roleGroupLeader},
			candidateRoles: []string{roleGroupAdmin},
			wantLeaders:    []string{"leader"},
			wantAdmins:     []string{"candidate"},
		},
		{
			name:           "group leader can transfer leader role",
			actorID:        1,
			adminRoles:     []string{roleGroupAdmin},
			candidateRoles: []string{roleGroupLeader},
			wantLeaders:    []string{"candidate"},
			wantAdmins:     []string{"admin"},
		},
		{
			name:           "tenant admin can transfer leader role",
			actorID:        4,
			adminRoles:     []string{roleGroupAdmin},
			candidateRoles: []string{roleGroupLeader},
			wantLeaders:    []string{"candidate"},
			wantAdmins:     []string{"admin"},
		},
		{
			name:           "super admin can transfer leader role",
			actorID:        5,
			adminRoles:     []string{roleGroupAdmin},
			candidateRoles: []string{roleGroupLeader},
			wantLeaders:    []string{"candidate"},
			wantAdmins:     []string{"admin"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := testdb.Open(t)
			seedBackupRoleAuthorization(t, db)
			payload := Payload{Members: []Member{
				{Username: "leader", DisplayName: "Leader", MemberName: "Leader", Roles: test.leaderRoles},
				{Username: "admin", DisplayName: "Admin", MemberName: "Admin", Roles: test.adminRoles},
				{Username: "candidate", DisplayName: "Candidate", MemberName: "Candidate", Roles: test.candidateRoles},
			}}

			err := NewMySQLRepository(db).ImportLocalBackup(t.Context(), 1, test.actorID, payload, time.Now())
			if test.wantErr {
				if !errors.Is(err, ErrBackupRoleChangeForbidden) {
					t.Fatalf("import error = %v, want %v", err, ErrBackupRoleChangeForbidden)
				}
			} else if err != nil {
				t.Fatalf("import failed: %v", err)
			}

			if got := backupRoleUsernames(t, db, roleGroupLeader); !reflect.DeepEqual(got, test.wantLeaders) {
				t.Errorf("leaders = %v, want %v", got, test.wantLeaders)
			}
			if got := backupRoleUsernames(t, db, roleGroupAdmin); !reflect.DeepEqual(got, test.wantAdmins) {
				t.Errorf("admins = %v, want %v", got, test.wantAdmins)
			}
		})
	}
}

func seedBackupRoleAuthorization(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.Exec(t, db, `INSERT INTO study_groups(id,code,name,created_at,updated_at)
		VALUES (1,'group-a','Group A',NOW(),NOW());
		INSERT INTO users(id,username,display_name,name_pinyin,is_super_admin,created_at,updated_at)
		VALUES (1,'leader','Leader','leader',0,NOW(),NOW()),
		       (2,'admin','Admin','admin',0,NOW(),NOW()),
		       (3,'candidate','Candidate','candidate',0,NOW(),NOW()),
		       (4,'tenant-admin','Tenant Admin','tenant-admin',0,NOW(),NOW()),
		       (5,'super-admin','Super Admin','super-admin',1,NOW(),NOW());
		INSERT INTO tenant_members(tenant_id,user_id,role,status,created_at,updated_at)
		VALUES (1,1,'member',1,NOW(),NOW()),
		       (1,2,'member',1,NOW(),NOW()),
		       (1,3,'member',1,NOW(),NOW()),
		       (1,4,'admin',1,NOW(),NOW());
		INSERT INTO group_members(group_id,user_id,member_name,status,joined_at,created_at,updated_at)
		VALUES (1,1,'Leader',1,NOW(),NOW(),NOW()),
		       (1,2,'Admin',1,NOW(),NOW(),NOW()),
		       (1,3,'Candidate',1,NOW(),NOW(),NOW());
		INSERT INTO user_group_roles(group_id,user_id,role,created_at)
		VALUES (1,1,'group_leader',NOW()),
		       (1,2,'group_admin',NOW())`)
}

func backupRoleUsernames(t *testing.T, db *sql.DB, role string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT u.username
		FROM user_group_roles r JOIN users u ON u.id=r.user_id
		WHERE r.group_id=1 AND r.role=? ORDER BY u.username`, role)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var usernames []string
	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			t.Fatal(err)
		}
		usernames = append(usernames, username)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return usernames
}
