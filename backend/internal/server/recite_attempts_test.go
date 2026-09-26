package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestedReciteUserID(t *testing.T) {
	user := currentUser{ID: 7}
	for _, test := range []struct {
		name       string
		raw        string
		user       currentUser
		wantID     uint64
		wantStatus int
	}{
		{name: "old default remains own account", raw: "", user: user, wantID: 7},
		{name: "explicit own account", raw: "7", user: user, wantID: 7},
		{name: "member cannot select another account", raw: "8", user: user, wantStatus: http.StatusForbidden},
		{name: "super admin may select another account", raw: "8", user: currentUser{ID: 7, IsSuperAdmin: true}, wantID: 8},
		{name: "invalid id", raw: "abc", user: user, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			got, ok := requestedReciteUserID(response, test.user, test.raw)
			if test.wantStatus == 0 {
				if !ok || got != test.wantID {
					t.Fatalf("result = (%d,%t), want (%d,true)", got, ok, test.wantID)
				}
				return
			}
			if ok || response.Code != test.wantStatus {
				t.Fatalf("result = (%d,%t), status = %d, want %d", got, ok, response.Code, test.wantStatus)
			}
		})
	}
}

func TestReciteAdministrationRejectsRegularMembersBeforeDatabase(t *testing.T) {
	a := &app{}
	for _, test := range []struct {
		name    string
		method  string
		path    string
		body    string
		handler http.HandlerFunc
	}{
		{"list management", http.MethodGet, "/api/super-admin/recite-attempts", "", a.handleSuperListReciteAttempts},
		{"delete management", http.MethodDelete, "/api/super-admin/recite-attempts/5", "", a.handleSuperDeleteReciteAttempt},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{ID: 7, CurrentGroupID: 2}))
			response := httptest.NewRecorder()
			a.requireSuper(test.handler).ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", response.Code)
			}
		})
	}
}

func TestReciteAdminAttemptJSONIncludesIdentity(t *testing.T) {
	item := reciteAdminAttempt{reciteAttempt: reciteAttempt{ID: 5, Score: 90}, GroupID: 2, UserID: 7, UserName: "甲", VerseRef: "罗马书"}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["id"] != float64(5) || fields["score"] != float64(90) || fields["group_id"] != float64(2) || fields["user_name"] != "甲" {
		t.Fatalf("unexpected admin JSON: %s", data)
	}
}

func TestReciteScore(t *testing.T) {
	for _, test := range []struct {
		correct int
		total   int
		rate    int
		want    int
	}{
		{0, 0, 50, 0},
		{0, 4, 50, 0},
		{1, 3, 30, 10},
		{2, 3, 90, 60},
		{4, 4, 90, 90},
	} {
		if got := reciteScore(test.correct, test.total, test.rate); got != test.want {
			t.Errorf("reciteScore(%d, %d, %d) = %d, want %d", test.correct, test.total, test.rate, got, test.want)
		}
	}
}

func TestReciteAccuracy(t *testing.T) {
	if got := reciteAccuracy(2, 3); got != 67 {
		t.Fatalf("accuracy = %d, want 67", got)
	}
	if got := reciteAccuracy(0, 0); got != 0 {
		t.Fatalf("zero-blank accuracy = %d, want 0", got)
	}
}

func TestBuildReciteLeaderboard(t *testing.T) {
	date := func(day int) time.Time { return time.Date(2026, 9, day, 8, 0, 0, 0, time.UTC) }
	entries := buildReciteLeaderboard([]reciteLeaderboardAttempt{
		{UserID: 1, Name: "甲", Ref: "罗马书", Score: 30, BlankPercent: 50, Accuracy: 60, At: date(1)},
		{UserID: 1, Name: "甲", Ref: "罗马书", Score: 30, BlankPercent: 60, Accuracy: 50, At: date(2)},
		{UserID: 2, Name: "乙", Ref: "罗马书", Score: 30, BlankPercent: 50, Accuracy: 60, At: date(3)},
	}, time.UTC)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	first := entries[0]
	if first.Name != "乙" || first.RankScore != 30 || first.Attempts != 1 || first.BestAccuracy != 60 {
		t.Fatalf("first entry = %+v", first)
	}
	second := entries[1]
	if second.Name != "甲" || second.BestBlankPercent != 60 || second.BestAccuracy != 50 || second.AverageAccuracy != 55 || second.LatestAt != date(2).Format(time.RFC3339) {
		t.Fatalf("second entry = %+v", second)
	}
}
