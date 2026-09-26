package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestConvertAssetToImportWithUnchangedBinding(t *testing.T) {
	for _, test := range []struct {
		name        string
		binding     bool
		wantInserts int
	}{
		{name: "existing binding", binding: true, wantInserts: 0},
		{name: "missing binding", binding: false, wantInserts: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &bindingTestState{exists: test.binding}
			db := sql.OpenDB(bindingTestConnector{state})
			defer db.Close()
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err := convertAssetToImport(context.Background(), tx, 2, 486, "key", reusableAsset{ID: 20}, time.Now()); err != nil {
				t.Fatal(err)
			}
			if state.inserts != test.wantInserts {
				t.Fatalf("binding inserts = %d, want %d", state.inserts, test.wantInserts)
			}
		})
	}
}

type bindingTestState struct {
	exists  bool
	inserts int
}

type bindingTestConnector struct{ state *bindingTestState }

func (c bindingTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &bindingTestConn{state: c.state}, nil
}

func (bindingTestConnector) Driver() driver.Driver { return bindingTestDriver{} }

type bindingTestDriver struct{}

func (bindingTestDriver) Open(string) (driver.Conn, error) { return nil, errors.New("unused") }

type bindingTestConn struct{ state *bindingTestState }

func (*bindingTestConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (*bindingTestConn) Close() error                        { return nil }
func (*bindingTestConn) Begin() (driver.Tx, error)           { return bindingTestTx{}, nil }

func (c *bindingTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO asset_bindings") {
		if c.state.exists {
			return nil, errors.New("duplicate binding")
		}
		c.state.exists = true
		c.state.inserts++
	}
	return driver.RowsAffected(0), nil
}

func (c *bindingTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT 1 FROM asset_bindings") {
		return nil, errors.New("unexpected query")
	}
	return &bindingTestRows{exists: c.state.exists}, nil
}

type bindingTestTx struct{}

func (bindingTestTx) Commit() error   { return nil }
func (bindingTestTx) Rollback() error { return nil }

type bindingTestRows struct {
	exists bool
	read   bool
}

func (*bindingTestRows) Columns() []string { return []string{"exists"} }
func (*bindingTestRows) Close() error      { return nil }
func (r *bindingTestRows) Next(dest []driver.Value) error {
	if !r.exists || r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = int64(1)
	return nil
}
