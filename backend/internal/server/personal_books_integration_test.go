//go:build integration

package server

import (
	"agp/backend/internal/asset"
	"agp/backend/internal/testdb"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPersonalBooksIsolationAndCompletion(t *testing.T) {
	a, db := passwordSessionFixture(t)
	testdb.Apply(t, db, "030_personal_daily_books.sql")
	root := t.TempDir()
	if err := os.MkdirAll(root+"/team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root+"/team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/book.md", []byte("# 1 First\nContent"), 0600); err != nil {
		t.Fatal(err)
	}
	a.assets = asset.NewService(asset.NewMySQLRepository(db), asset.NewLocalStorage(root), "")
	a.location = time.FixedZone("CST", 8*3600)
	testdb.Exec(t, db, `INSERT INTO assets(id,group_id,category,title,original_name,storage_path,mime_type,created_by,created_at,updated_at) VALUES(10,1,'markdown','Book','book.md','team-a-resources/objects/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/book.md','text/markdown',1,NOW(),NOW())`)
	testdb.Exec(t, db, `INSERT INTO asset_bindings(asset_id,group_id,resource_key,created_at,updated_at) VALUES(10,1,REPEAT('a',32),NOW(),NOW())`)
	call := func(method, body string, userID, groupID uint64, handler http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/personal-books", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: userID, CurrentGroupID: groupID}))
		req.SetPathValue("date", "2026-01-02")
		response := httptest.NewRecorder()
		handler(response, req)
		return response
	}
	plan := `{"date":"2026-01-02","title":"My book","path":"/api/assets/10/download","type":"markdown","section":1}`
	require := func(response *httptest.ResponseRecorder, status int, contains string) {
		t.Helper()
		if response.Code != status || !strings.Contains(response.Body.String(), contains) {
			t.Fatalf("status=%d body=%s want=%d contains=%s", response.Code, response.Body, status, contains)
		}
	}
	require(call("PUT", plan, 1, 1, a.handleSavePersonalBook), 200, `"ok":true`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"completed":false`)
	require(call("GET", "", 2, 1, a.handlePersonalBooks), 200, `"plans":[]`)
	require(call("PUT", strings.Replace(plan, "My book", "Other account", 1), 2, 1, a.handleSavePersonalBook), 200, `"ok":true`)
	require(call("GET", "", 2, 2, a.handlePersonalBooks), 200, `"plans":[]`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"title":"My book"`)
	require(call("GET", "", 5, 1, a.handlePersonalBooks), 403, "group_membership_required")
	require(call("PUT", "", 1, 1, a.handlePersonalBookCompletion), 200, `"ok":true`)
	require(call("PUT", "", 1, 1, a.handlePersonalBookCompletion), 200, `"ok":true`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"completed":true`)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM checkin_records`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("personal completion entered group checkins: %d, %v", count, err)
	}
	require(call("DELETE", "", 1, 1, a.handleDeletePersonalBook), 200, `"ok":true`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"plans":[]`)
	require(call("PUT", "", 1, 1, a.handlePersonalBookCompletion), 404, "personal_book_not_found")
	require(call("PUT", plan, 1, 1, a.handleSavePersonalBook), 200, `"ok":true`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"completed":true`)
	require(call("DELETE", "", 1, 1, a.handlePersonalBookCompletion), 200, `"ok":true`)
	require(call("GET", "", 1, 1, a.handlePersonalBooks), 200, `"completed":false`)
	for _, body := range []string{strings.Replace(plan, `"section":1`, `"section":0`, 1), strings.Replace(plan, "2026-01-02", "2026-02-30", 1), strings.Replace(plan, "/api/assets/10/download", "https://example.com/book.md", 1), `{"date":"2026-01-02","title":"PDF","path":"/api/assets/10/download","type":"pdf","page_start":4,"page_end":2}`} {
		require(call("PUT", body, 1, 1, a.handleSavePersonalBook), 400, "invalid_personal_book")
	}
	req := httptest.NewRequest("PUT", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
	req.SetPathValue("date", time.Now().In(a.location).AddDate(0, 0, 1).Format("2006-01-02"))
	response := httptest.NewRecorder()
	a.handlePersonalBookCompletion(response, req)
	require(response, 400, "future_date_forbidden")
}
