package notification

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"
)

type queryStep struct {
	contains []string
	args     []any
	rows     [][]driver.Value
	columns  int
	err      error
}

type sourceConnector struct {
	t     *testing.T
	steps []queryStep
}

func (c *sourceConnector) Connect(context.Context) (driver.Conn, error) {
	return &sourceConn{connector: c}, nil
}

func (c *sourceConnector) Driver() driver.Driver { return sourceDriver{} }

type sourceDriver struct{}

func (sourceDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type sourceConn struct{ connector *sourceConnector }

func (*sourceConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }
func (*sourceConn) Close() error                        { return nil }
func (*sourceConn) Begin() (driver.Tx, error)           { return nil, errors.New("unexpected transaction") }
func (c *sourceConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.connector.t.Helper()
	if len(c.connector.steps) == 0 {
		c.connector.t.Fatal("unexpected query")
	}
	step := c.connector.steps[0]
	c.connector.steps = c.connector.steps[1:]
	for _, fragment := range step.contains {
		if !strings.Contains(query, fragment) {
			c.connector.t.Errorf("query missing %q", fragment)
		}
	}
	values := make([]any, len(args))
	for i := range args {
		values[i] = args[i].Value
	}
	if !reflect.DeepEqual(values, step.args) {
		c.connector.t.Errorf("query args=%#v, want %#v", values, step.args)
	}
	return &sourceRows{count: step.columns, rows: step.rows}, step.err
}

type sourceRows struct {
	count int
	rows  [][]driver.Value
}

func (r *sourceRows) Columns() []string { return make([]string, r.count) }
func (*sourceRows) Close() error        { return nil }
func (r *sourceRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func TestCheckinSourceSnapshot(t *testing.T) {
	t.Parallel()
	date := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	// UTC September 8 is already September 9 in the application's timezone.
	checkedAt := time.Date(2026, 9, 8, 16, 5, 0, 0, time.UTC)
	start := date.AddDate(0, 0, -2)
	end := date.AddDate(0, 0, 4)
	tests := []struct {
		name     string
		taskType string
		date     time.Time
		weekID   driver.Value
		noRecord bool
		readErr  error
		want     string
	}{
		{"daily Shanghai midnight", "daily_devotion", date, nil, false, nil, "每日灵修\n1 【新】张三"},
		{"daily historical", "daily_devotion", date.AddDate(0, 0, -1), nil, false, nil, ""},
		{"weekly current", "weekly_book", start, int64(7), false, nil, "本周任务\n1 张三 【新】基督"},
		{"weekly video", "weekly_video", start, int64(7), false, nil, "本周任务\n1 张三 【新】视频"},
		{"weekly previous despite current date", "weekly_book", date, int64(6), false, nil, ""},
		{"deleted record", "", date, nil, true, nil, ""},
		{"database error", "", date, nil, false, errors.New("db unavailable"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{RecordID: 42, GroupID: 1, LogicalDate: tt.date.Format("2006-01-02")}
			first := queryStep{
				contains: []string{"group_id=? AND id=? AND logical_date=?", "deleted_at IS NULL", "status='done'", "source='web'"},
				args:     []any{int64(1), int64(42), event.LogicalDate},
				columns:  4, err: tt.readErr,
			}
			if !tt.noRecord {
				first.rows = [][]driver.Value{{tt.taskType, tt.date, checkedAt, tt.weekID}}
			}
			steps := []queryStep{first}
			if strings.HasPrefix(tt.taskType, "weekly_") {
				steps = append(steps, queryStep{
					contains: []string{"group_id=?", "start_date<=? AND end_date>=?", "ORDER BY start_date DESC"},
					args:     []any{int64(1), "2026-09-09", "2026-09-09"},
					columns:  3,
					rows:     [][]driver.Value{{int64(7), start, end}},
				})
			}
			if tt.want != "" {
				args := []any{int64(1), "2026-09-09", "2026-09-09", int64(42)}
				fragments := []string{
					"c.group_id=?", "c.logical_date BETWEEN ? AND ?", "c.id<=?",
					"m.group_id=c.group_id", "t.group_id=c.group_id",
					"c.deleted_at IS NULL", "ORDER BY c.checkin_time,c.id",
				}
				if tt.weekID != nil {
					args[1], args[2] = "2026-09-07", "2026-09-13"
					args = append(args[:3], int64(7), int64(7), int64(42))
					fragments = append(fragments, "c.week_id=?", "current_task.week_id=?",
						"current_ta.asset_id=checked_ta.asset_id", "checked_ta.group_id=c.group_id")
				}
				steps = append(steps, queryStep{
					contains: fragments, args: args, columns: 6,
					rows: [][]driver.Value{{int64(42), int64(2), "张三", tt.taskType, "title", `{"book_name":"基督是一切"}`}},
				})
			}
			connector := &sourceConnector{t: t, steps: steps}
			db := sql.OpenDB(connector)
			defer db.Close()
			source := NewCheckinSource(db, time.FixedZone("CST", 8*3600))
			got, err := source.Snapshot(t.Context(), event)
			if (err != nil) != (tt.readErr != nil) {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if got.Text != tt.want {
				t.Fatalf("Snapshot() = %q, want %q", got.Text, tt.want)
			}
			if tt.want != "" {
				wantEnd := "2026-09-10 00:00 +0800"
				if tt.weekID != nil {
					wantEnd = "2026-09-14 00:00 +0800"
				}
				if got.ExpiresAt.Format("2006-01-02 15:04 -0700") != wantEnd {
					t.Fatalf("expiration = %v, want %s", got.ExpiresAt, wantEnd)
				}
			}
			if len(connector.steps) != 0 {
				t.Fatalf("%d queries were not executed", len(connector.steps))
			}
		})
	}
}

func TestCheckinSourceEnabled(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		event   Event
		columns int
		rows    [][]driver.Value
		want    bool
		wantErr bool
	}{
		{
			name: "daily defaults enabled",
			event: Event{
				RecordID: 1, GroupID: 2, LogicalDate: "2026-09-09",
			},
			columns: 2,
			rows:    [][]driver.Value{{"daily_devotion", nil}},
			want:    true,
		},
		{
			name: "daily disabled",
			event: Event{
				RecordID: 1, GroupID: 2, LogicalDate: "2026-09-09",
			},
			columns: 2,
			rows: [][]driver.Value{{
				"daily_devotion", `{"checkin_notifications":{"daily_enabled":false,"weekly_enabled":true}}`,
			}},
		},
		{
			name: "weekly disabled",
			event: Event{
				RecordID: 1, GroupID: 2, LogicalDate: "2026-09-09",
			},
			columns: 2,
			rows: [][]driver.Value{{
				"weekly_video", `{"checkin_notifications":{"daily_enabled":true,"weekly_enabled":false}}`,
			}},
		},
		{
			name:    "initial defaults enabled without settings row",
			event:   Event{GroupID: 2, Initial: "weekly"},
			columns: 1,
			want:    true,
		},
		{
			name: "invalid settings fail closed",
			event: Event{
				RecordID: 1, GroupID: 2, LogicalDate: "2026-09-09",
			},
			columns: 2,
			rows:    [][]driver.Value{{"daily_devotion", `{"checkin_notifications":{"daily_enabled":"yes"}}`}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragments := []string{"group_id=?"}
			args := []any{int64(2)}
			if tt.event.Initial == "" {
				fragments = append(fragments, "c.id=?", "c.logical_date=?", "c.source='web'")
				args = append(args, int64(1), "2026-09-09")
			} else {
				fragments = append(fragments, "FROM group_settings")
			}
			connector := &sourceConnector{t: t, steps: []queryStep{{
				contains: fragments,
				args:     args,
				columns:  tt.columns,
				rows:     tt.rows,
			}}}
			db := sql.OpenDB(connector)
			defer db.Close()
			got, err := NewCheckinSource(db, time.UTC).Enabled(t.Context(), tt.event)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("Enabled() = %v, %v; want %v, error=%v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
