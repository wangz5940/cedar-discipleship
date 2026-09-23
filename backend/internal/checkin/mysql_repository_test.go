package checkin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type weeklyLookupStep struct {
	contains []string
	excludes []string
	args     []any
	rows     [][]driver.Value
}

type weeklyLookupConnector struct {
	t     *testing.T
	steps []weeklyLookupStep
}

func (c *weeklyLookupConnector) Connect(context.Context) (driver.Conn, error) {
	return &weeklyLookupConn{connector: c}, nil
}

func (c *weeklyLookupConnector) Driver() driver.Driver { return weeklyLookupDriver{} }

type weeklyLookupDriver struct{}

func (weeklyLookupDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type weeklyLookupConn struct{ connector *weeklyLookupConnector }

func (*weeklyLookupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (*weeklyLookupConn) Close() error { return nil }

func (*weeklyLookupConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}

func (c *weeklyLookupConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.connector.t.Helper()
	if len(c.connector.steps) == 0 {
		return nil, errors.New("unexpected query")
	}
	step := c.connector.steps[0]
	c.connector.steps = c.connector.steps[1:]
	for _, fragment := range step.contains {
		if !strings.Contains(query, fragment) {
			c.connector.t.Errorf("query missing %q", fragment)
		}
	}
	for _, fragment := range step.excludes {
		if strings.Contains(query, fragment) {
			c.connector.t.Errorf("query unexpectedly contains %q", fragment)
		}
	}
	values := make([]any, len(args))
	for index := range args {
		values[index] = args[index].Value
	}
	if !reflect.DeepEqual(values, step.args) {
		c.connector.t.Errorf("query args=%#v, want %#v", values, step.args)
	}
	return &weeklyLookupRows{rows: step.rows}, nil
}

type weeklyLookupRows struct{ rows [][]driver.Value }

func (*weeklyLookupRows) Columns() []string { return []string{"id"} }
func (*weeklyLookupRows) Close() error      { return nil }
func (r *weeklyLookupRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func TestMySQLRepositoryFindExistingWeeklyTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		taskID   uint64
		weekID   uint64
		taskType string
		steps    []weeklyLookupStep
		wantID   uint64
		wantErr  error
	}{
		{
			name:     "same task remains complete across weeks",
			taskID:   890,
			weekID:   91,
			taskType: "weekly_verse",
			steps: []weeklyLookupStep{{
				contains: []string{"task_id=?", "task_type=?"},
				excludes: []string{"week_id=?", "logical_date=?", "logical_date BETWEEN"},
				args:     []any{int64(5), int64(95), int64(890), "weekly_verse"},
				rows:     [][]driver.Value{{int64(42)}},
			}},
			wantID: 42,
		},
		{
			name:     "different task in reused week is not a match",
			taskID:   890,
			weekID:   90,
			taskType: "weekly_verse",
			steps: []weeklyLookupStep{
				{
					contains: []string{"task_id=?", "task_type=?"},
					args:     []any{int64(5), int64(95), int64(890), "weekly_verse"},
				},
				{
					contains: []string{"week_id=?", "task_type=?", "task_id IS NULL"},
					args:     []any{int64(5), int64(95), int64(90), "weekly_verse"},
				},
			},
			wantErr: sql.ErrNoRows,
		},
		{
			name:     "legacy record without task id remains compatible",
			taskID:   890,
			weekID:   90,
			taskType: "weekly_verse",
			steps: []weeklyLookupStep{
				{
					contains: []string{"task_id=?", "task_type=?"},
					args:     []any{int64(5), int64(95), int64(890), "weekly_verse"},
				},
				{
					contains: []string{"week_id=?", "task_type=?", "task_id IS NULL"},
					args:     []any{int64(5), int64(95), int64(90), "weekly_verse"},
					rows:     [][]driver.Value{{int64(43)}},
				},
			},
			wantID: 43,
		},
		{
			name:     "taskless lookup retains week fallback",
			weekID:   90,
			taskType: "weekly_outline",
			steps: []weeklyLookupStep{{
				contains: []string{"week_id=?", "task_type=?"},
				args:     []any{int64(5), int64(95), int64(90), "weekly_outline"},
				rows:     [][]driver.Value{{int64(44)}},
			}},
			wantID: 44,
		},
		{
			name:     "video completion follows the same asset",
			taskID:   891,
			weekID:   90,
			taskType: "weekly_video",
			steps: []weeklyLookupStep{
				{
					contains: []string{"task_id=?", "task_type=?"},
					args:     []any{int64(5), int64(95), int64(891), "weekly_video"},
				},
				{
					contains: []string{"target_ta.task_id=?", "target_ta.asset_id=checked_ta.asset_id"},
					args:     []any{int64(5), int64(95), int64(891)},
					rows:     [][]driver.Value{{int64(45)}},
				},
			},
			wantID: 45,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			connector := &weeklyLookupConnector{t: t, steps: test.steps}
			db := sql.OpenDB(connector)
			defer db.Close()

			got, err := NewMySQLRepository(db).FindExistingWeeklyTask(
				t.Context(),
				5,
				95,
				test.taskID,
				test.weekID,
				test.taskType,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("FindExistingWeeklyTask() error = %v, want %v", err, test.wantErr)
			}
			if got != test.wantID {
				t.Fatalf("FindExistingWeeklyTask() = %d, want %d", got, test.wantID)
			}
			if len(connector.steps) != 0 {
				t.Fatalf("%d queries were not executed", len(connector.steps))
			}
		})
	}
}
