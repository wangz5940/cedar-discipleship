package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecitePaperValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*recitePaper)
		valid  bool
	}{
		{"valid snapshot", func(*recitePaper) {}, true},
		{"protected references version", func(p *recitePaper) { p.Version = 2 }, true},
		{"unsupported version", func(p *recitePaper) { p.Version = 3 }, false},
		{"empty text", func(p *recitePaper) { p.Text = "" }, false},
		{"oversized text", func(p *recitePaper) { p.Text = strings.Repeat("神", 30001) }, false},
		{"missing indexes", func(p *recitePaper) { p.BlankIndexes = nil }, false},
		{"negative index", func(p *recitePaper) { p.BlankIndexes[0] = -1 }, false},
		{"index outside text", func(p *recitePaper) { p.BlankIndexes[0] = 100 }, false},
		{"unordered indexes", func(p *recitePaper) { p.BlankIndexes = []int{2, 1}; p.Answers = []string{"", ""} }, false},
		{"duplicate indexes", func(p *recitePaper) { p.BlankIndexes = []int{1, 1}; p.Answers = []string{"", ""} }, false},
		{"missing answer", func(p *recitePaper) { p.Answers = nil }, false},
		{"too many answers", func(p *recitePaper) { p.Answers = []string{"", ""} }, false},
		{"oversized answers", func(p *recitePaper) { p.Answers[0] = strings.Repeat("神", 30001) }, false},
		{"blank answer", func(p *recitePaper) { p.Answers[0] = "" }, true},
		{"Unicode snapshot", func(p *recitePaper) { p.Text = "𠮷神爱"; p.Answers[0] = "𠮷神爱" }, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			paper := &recitePaper{Version: 1, Text: "【弗1:16】感谢　神", BlankIndexes: []int{1}, Answers: []string{"感谢神"}}
			test.change(paper)
			if got := validRecitePaper(paper); got != test.valid {
				t.Fatalf("valid=%v want %v", got, test.valid)
			}
			if test.valid {
				return
			}
			body, err := json.Marshal(map[string]any{"task_id": 1, "blank_percent": 100, "blank_count": 3, "correct_count": 3, "paper": paper})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/recite-attempts", strings.NewReader(string(body)))
			req = req.WithContext(context.WithValue(req.Context(), currentUserKey, currentUser{ID: 1, CurrentGroupID: 1}))
			res := httptest.NewRecorder()
			(&app{}).handleCreateReciteAttempt(res, req)
			if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "invalid_recite_paper") {
				t.Fatalf("status=%d body=%s", res.Code, res.Body)
			}
		})
	}
	if !validRecitePaper(nil) {
		t.Fatal("older requests must remain valid without a snapshot")
	}
}
