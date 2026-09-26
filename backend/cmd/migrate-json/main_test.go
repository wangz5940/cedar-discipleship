package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

type existingGroupConnector struct {
	t *testing.T
}

func (c *existingGroupConnector) Connect(context.Context) (driver.Conn, error) {
	return &existingGroupConn{t: c.t}, nil
}

func (*existingGroupConnector) Driver() driver.Driver { return existingGroupDriver{} }

type existingGroupDriver struct{}

func (existingGroupDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type existingGroupConn struct {
	t *testing.T
}

func (*existingGroupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (*existingGroupConn) Close() error { return nil }

func (*existingGroupConn) Begin() (driver.Tx, error) { return existingGroupTx{}, nil }

func (c *existingGroupConn) QueryContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT id FROM study_groups WHERE code=?") {
		c.t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 1 || args[0].Value != "existing" {
		c.t.Fatalf("query args = %#v", args)
	}
	return &existingGroupRows{rows: [][]driver.Value{{int64(42)}}}, nil
}

func (c *existingGroupConn) ExecContext(
	_ context.Context,
	query string,
	args []driver.NamedValue,
) (driver.Result, error) {
	if !strings.Contains(query, "UPDATE study_groups SET name=?, updated_at=? WHERE id=?") ||
		strings.Contains(strings.ToLower(query), "status") {
		c.t.Fatalf("existing group update changes status: %s", query)
	}
	if len(args) != 3 || args[0].Value != "replacement" || args[2].Value != int64(42) {
		c.t.Fatalf("exec args = %#v", args)
	}
	return driver.RowsAffected(1), nil
}

type existingGroupTx struct{}

func (existingGroupTx) Commit() error   { return nil }
func (existingGroupTx) Rollback() error { return nil }

type existingGroupRows struct {
	rows [][]driver.Value
}

func (*existingGroupRows) Columns() []string { return []string{"id"} }
func (*existingGroupRows) Close() error      { return nil }

func (r *existingGroupRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

func TestEnsureGroupPreservesExistingGroupStatus(t *testing.T) {
	t.Parallel()
	db := sql.OpenDB(&existingGroupConnector{t: t})
	defer db.Close()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	id, created, err := ensureGroup(t.Context(), tx, "existing", "replacement", "hash", "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 || created {
		t.Fatalf("ensureGroup() = (%d,%v), want (42,false)", id, created)
	}
}

type memberLookupConnector struct {
	t       *testing.T
	userIDs []int64
}

func (c *memberLookupConnector) Connect(context.Context) (driver.Conn, error) {
	return &memberLookupConn{memberLookupConnector: c}, nil
}

func (*memberLookupConnector) Driver() driver.Driver { return existingGroupDriver{} }

type memberLookupConn struct{ *memberLookupConnector }

func (*memberLookupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (*memberLookupConn) Close() error { return nil }

func (*memberLookupConn) Begin() (driver.Tx, error) { return existingGroupTx{}, nil }

func (c *memberLookupConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT user_id FROM group_members") ||
		!strings.Contains(query, "group_id=? AND status=1 AND member_name=? LIMIT 2") ||
		len(args) != 2 || args[0].Value != int64(42) || args[1].Value != "same name" {
		c.t.Fatalf("unexpected member lookup: %q, %#v", query, args)
	}
	rows := make([][]driver.Value, 0, len(c.userIDs))
	for _, id := range c.userIDs {
		rows = append(rows, []driver.Value{id})
	}
	return &existingGroupRows{rows: rows}, nil
}

func TestReuseGroupMemberByNameOnlyWhenEnabledAndUnique(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ids     []int64
		enabled bool
		wantID  uint64
		wantHit bool
		wantErr bool
	}{
		{name: "disabled", ids: []int64{12}, enabled: false},
		{name: "absent", enabled: true},
		{name: "unique", ids: []int64{12}, enabled: true, wantID: 12, wantHit: true},
		{name: "ambiguous", ids: []int64{12, 13}, enabled: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := sql.OpenDB(&memberLookupConnector{t: t, userIDs: tc.ids})
			defer db.Close()
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			id, hit, err := reuseGroupMemberByName(t.Context(), tx, 42, "same name", tc.enabled)
			if id != tc.wantID || hit != tc.wantHit || (err != nil) != tc.wantErr {
				t.Fatalf("reuseGroupMemberByName() = (%d,%v,%v)", id, hit, err)
			}
		})
	}
}

func TestNamespacedGeneratedUsernameKeepsExplicitMappings(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  options
		mapa map[string]string
		want string
		gen  bool
	}{
		{name: "legacy default", want: "member003", gen: true},
		{name: "opted in", opt: options{groupCode: "ZW1", namespaceGeneratedUsernames: true}, want: "zw1-member003", gen: true},
		{name: "explicit map", opt: options{groupCode: "zw1", namespaceGeneratedUsernames: true}, mapa: map[string]string{"张三": "Existing_123"}, want: "existing_123"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, generated := usernameForImport("张三", 3, tc.mapa, tc.opt)
			if got != tc.want || generated != tc.gen {
				t.Fatalf("usernameForImport() = (%q,%v)", got, generated)
			}
		})
	}
}

func TestTasksForWeekSplitsMultipleReadingsIntoMultipleWeeklyBookTasks(t *testing.T) {
	titleJSON, err := json.Marshal([]string{
		"《基督是一切》48-52页",
		"《救赎史剧》109-118页",
	})
	if err != nil {
		t.Fatalf("marshal title json: %v", err)
	}
	week := oldWeek{
		Title: titleJSON,
		Readings: []oldAssetRef{
			{Title: "《基督是一切》48-52页", URL: "/api/assets/101/download", Type: "pdf"},
			{Title: "《救赎史剧》109-118页", URL: "/api/assets/102/download", Type: "pdf"},
		},
		Video: "本周视频",
		Verse: "背经",
	}

	tasks := tasksForWeek(week)
	var bookTasks []plannedTask
	for _, task := range tasks {
		if task.Type == "weekly_book" {
			bookTasks = append(bookTasks, task)
		}
	}

	if len(bookTasks) != 2 {
		t.Fatalf("expected 2 weekly_book tasks, got %d", len(bookTasks))
	}
	if bookTasks[0].Title != "《基督是一切》48-52页" {
		t.Fatalf("unexpected first title: %q", bookTasks[0].Title)
	}
	if bookTasks[1].Title != "《救赎史剧》109-118页" {
		t.Fatalf("unexpected second title: %q", bookTasks[1].Title)
	}
	if len(bookTasks[0].Assets) != 1 || bookTasks[0].Assets[0].Ref.URL != "/api/assets/101/download" {
		t.Fatalf("unexpected first assets: %+v", bookTasks[0].Assets)
	}
	if len(bookTasks[1].Assets) != 1 || bookTasks[1].Assets[0].Ref.URL != "/api/assets/102/download" {
		t.Fatalf("unexpected second assets: %+v", bookTasks[1].Assets)
	}
}

func TestReadingTasksForWeekFallsBackWhenTitleCountDiffersFromReadingCount(t *testing.T) {
	titleJSON, err := json.Marshal([]string{"《基督是一切》48-52页"})
	if err != nil {
		t.Fatalf("marshal title json: %v", err)
	}
	week := oldWeek{
		Title: titleJSON,
		Readings: []oldAssetRef{
			{Title: "", URL: "/api/assets/101/download", Type: "pdf"},
			{Title: "《救赎史剧》109-118页", URL: "/api/assets/102/download", Type: "pdf"},
		},
	}

	tasks := readingTasksForWeek(week)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 reading tasks, got %d", len(tasks))
	}
	if tasks[0].Title != "《基督是一切》48-52页" {
		t.Fatalf("unexpected first task title: %q", tasks[0].Title)
	}
	if tasks[1].Title != "《救赎史剧》109-118页" {
		t.Fatalf("unexpected second task title: %q", tasks[1].Title)
	}
}

func TestReadingTasksForWeekKeepsExternalHTMLAsContent(t *testing.T) {
	week := oldWeek{
		Readings: []oldAssetRef{
			{
				Title: "第一章 基督的血",
				URL:   "https://pages.uoregon.edu/fyin/book/001.htm",
				Type:  "iframe",
			},
		},
	}

	tasks := readingTasksForWeek(week)
	if len(tasks) != 1 {
		t.Fatalf("expected 1 reading task, got %d", len(tasks))
	}
	if tasks[0].Content != "https://pages.uoregon.edu/fyin/book/001.htm" {
		t.Fatalf("content = %q, want external URL", tasks[0].Content)
	}
	if shouldImportAssetRef(tasks[0].Assets[0].Ref.URL) {
		t.Fatal("external HTML should not be imported as an asset")
	}
}

func TestParseReadingMetadata(t *testing.T) {
	got := parseReadingMetadata("《基督是一切》基督是神的仆人--马可福音（22-25页，读到事奉的性质为止）")
	if got.BookName != "基督是一切" {
		t.Fatalf("BookName = %q, want %q", got.BookName, "基督是一切")
	}
	if got.PageStart != 22 || got.PageEnd != 25 {
		t.Fatalf("page range = %d-%d, want 22-25", got.PageStart, got.PageEnd)
	}
	if got.ReadingNote != "读到事奉的性质为止" {
		t.Fatalf("ReadingNote = %q, want %q", got.ReadingNote, "读到事奉的性质为止")
	}
}

func TestMigratedReadingContentIsStructuredJSON(t *testing.T) {
	content := migratedReadingContent("《救赎史剧》纵览 73-79页")
	var got migratedReadingMetadata
	if err := json.Unmarshal([]byte(content), &got); err != nil {
		t.Fatalf("unmarshal migrated content: %v", err)
	}
	if got.BookName != "救赎史剧" || got.PageStart != 73 || got.PageEnd != 79 {
		t.Fatalf("unexpected metadata: %+v", got)
	}
}

func TestNormalizeTaskSectionsBuildsCurrentScriptureAndDevotionShape(t *testing.T) {
	raw := json.RawMessage(`{
		"daily": {
			"path": "/api/assets/12/download",
			"devotion": {"start_date": "2026-05-27", "start_section": 43},
			"scripture": {
				"book": "路加福音",
				"book_id": "42",
				"max_chapters": 24,
				"start_date": "2026-05-27"
			}
		}
	}`)
	normalized, err := normalizeTaskSections(raw)
	if err != nil {
		t.Fatalf("normalize task sections: %v", err)
	}
	var sections map[string]any
	if err := json.Unmarshal(normalized, &sections); err != nil {
		t.Fatalf("unmarshal normalized sections: %v", err)
	}
	daily := sections["daily"].(map[string]any)
	devotion := daily["devotion"].(map[string]any)
	if devotion["path"] != "/api/assets/12/download" || devotion["numbered_start_date"] != "2026-05-27" || devotion["numbered_start"].(float64) != 43 {
		t.Fatalf("unexpected devotion config: %+v", devotion)
	}
	scripture := daily["scripture"].(map[string]any)
	sequence := scripture["sequence"].([]any)
	if len(sequence) != 25 {
		t.Fatalf("unexpected scripture sequence: %+v", sequence)
	}
	first := sequence[0].(map[string]any)
	last := sequence[len(sequence)-1].(map[string]any)
	if first["book"] != "路加福音" || first["book_id"] != "42" || first["chapters"].(float64) != 24 {
		t.Fatalf("unexpected first scripture book: %+v", first)
	}
	if last["book"] != "启示录" || last["book_id"] != "66" || last["chapters"].(float64) != 22 {
		t.Fatalf("unexpected last scripture book: %+v", last)
	}
	if scripture["book"] != "路加福音" || scripture["book_id"] != "42" || scripture["max_chapters"].(float64) != 24 {
		t.Fatalf("unexpected scripture start book: %+v", scripture)
	}
}

func TestDatabaseAssetDownloadURLCanonicalizesAbsoluteAPIURL(t *testing.T) {
	got := databaseAssetDownloadURL("https://mouss.synology.me:7399/api/assets/181/download")
	if got != "/api/assets/181/download" {
		t.Fatalf("databaseAssetDownloadURL() = %q, want canonical API path", got)
	}
}
