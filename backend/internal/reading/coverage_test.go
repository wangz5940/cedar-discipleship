package reading

import "testing"

func TestExplicitReadingCoverage(t *testing.T) {
	for _, tc := range []struct {
		title, content, target, targetContent string
		covered                               bool
	}{
		{"旧名称 13-27页", "", "新名称 13-15页", "", true},
		{"13至27页", "", "15页", "", true},
		{"阅读", `{"page_start":13,"page_end":27}`, "阅读", `{"page_start":13,"page_end":15}`, true},
		{"阅读", `{"source_title":"阅读 13-27页"}`, "阅读 13-15页", "", true},
		{"13-14页", "", "13-15页", "", false},
		{"14-27页", "", "13-15页", "", false},
		{"27-13页", "", "13-15页", "", false},
		{"阅读", "", "13-15页", "", false},
		{"13-27页", "", "阅读", "", false},
	} {
		if got := Covers(tc.title, tc.content, tc.target, tc.targetContent); got != tc.covered {
			t.Errorf("%q -> %q: %v, want %v", tc.title, tc.target, got, tc.covered)
		}
	}
}
