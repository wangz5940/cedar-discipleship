//go:build integration

package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Open creates a disposable database on the explicitly configured local test server.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	addr := os.Getenv("CEDAR_TEST_MYSQL_ADDR")
	if addr == "" {
		t.Skip("set CEDAR_TEST_MYSQL_ADDR to an isolated local MySQL server")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatal("integration database must be local")
	}
	admin, err := sql.Open("mysql", DSN(""))
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("cedar_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", DSN(name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
		if _, err := admin.Exec("DROP DATABASE " + name); err != nil {
			t.Error(err)
		}
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, migration := range []string{"001_init.sql", "002_checkin_partitions.sql", "007_resource_sharing.sql", "015_tenants.sql", "017_audit_log_id.sql", "018_feedback_workflow.sql", "023_recite_paper.sql", "024_learning_reminders.sql", "028_daily_devotion_reminders.sql"} {
		Apply(t, db, migration)
	}
	return db
}

func DSN(database string) string {
	credentials := "root"
	if password := os.Getenv("CEDAR_TEST_MYSQL_ROOT_PASSWORD"); password != "" {
		credentials += ":" + password
	}
	return credentials + "@tcp(" + os.Getenv("CEDAR_TEST_MYSQL_ADDR") + ")/" +
		database + "?parseTime=true&multiStatements=true"
}

func Apply(t *testing.T, db *sql.DB, migration string) {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(source), "../../migrations", migration))
	if err != nil {
		t.Fatal(err)
	}
	Exec(t, db, string(data))
}

func Exec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

// WaitForLockWait waits until another connection is blocked in this test's schema.
func WaitForLockWait(t *testing.T, db *sql.DB) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		var count int
		err := db.QueryRowContext(t.Context(), `SELECT COUNT(*)
			FROM performance_schema.data_lock_waits w
			JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID
			WHERE l.OBJECT_SCHEMA=DATABASE()`).Scan(&count)
		if err != nil {
			t.Fatal(err)
		}
		if count > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("no database lock wait observed")
		}
	}
}
