package asset

import (
	"reflect"
	"strings"
	"testing"
)

func TestAppendAssetCategoryFilter(t *testing.T) {
	tests := []struct {
		name          string
		category      string
		columns       []string
		wantCondition string
		wantArgs      []any
	}{
		{
			name:          "handout aliases",
			category:      "handout",
			columns:       []string{"a.category"},
			wantCondition: "AND (a.category=? OR a.category=? OR a.category=?)",
			wantArgs:      []any{"handout", "share", "ppt"},
		},
		{
			name:          "passage aliases",
			category:      "pdf",
			columns:       []string{"a.category"},
			wantCondition: "AND (a.category=? OR a.category=?)",
			wantArgs:      []any{"passage", "pdf"},
		},
		{
			name:          "dynamic ministry attachments",
			category:      "ministry_attachment",
			columns:       []string{"ca.category", "pa.category"},
			wantCondition: "AND (ca.category LIKE ? OR pa.category LIKE ?)",
			wantArgs:      []any{"ministry-%", "ministry-%"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args := appendAssetCategoryFilter("SELECT 1", nil, tt.category, tt.columns...)
			if !strings.Contains(query, tt.wantCondition) {
				t.Fatalf("query = %q, want condition %q", query, tt.wantCondition)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Fatalf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}
