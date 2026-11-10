//go:build integration

package server

import (
	"testing"

	"agp/backend/internal/testdb"
)

func TestGroupPasswordMigrationPreservesDataOnRestart(t *testing.T) {
	db := testdb.Open(t)
	testdb.Exec(t, db, `ALTER TABLE study_groups DROP COLUMN default_password_encrypted`)
	testdb.Apply(t, db, "031_group_password_encrypted.sql")
	testdb.Exec(t, db, `INSERT INTO study_groups(code,name,default_password_hash,default_password_encrypted,created_at,updated_at)
		VALUES ('restart-check','重启验收','existing-hash','existing-encrypted-value',NOW(),NOW())`)
	testdb.Apply(t, db, "031_group_password_encrypted.sql")
	var hash, encrypted string
	if err := db.QueryRow(`SELECT default_password_hash,default_password_encrypted FROM study_groups WHERE code='restart-check'`).Scan(&hash, &encrypted); err != nil {
		t.Fatal(err)
	}
	if hash != "existing-hash" || encrypted != "existing-encrypted-value" {
		t.Fatal("restart changed the existing group password")
	}
}
