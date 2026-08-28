package asset

import "strings"

func appendAssetCategoryFilter(query string, args []any, category string, columns ...string) (string, []any) {
	category = strings.TrimSpace(strings.ToLower(category))
	if category == "" || len(columns) == 0 {
		return query, args
	}

	parts := make([]string, 0, len(columns)*3)
	if category == "ministry_attachment" {
		for _, column := range columns {
			parts = append(parts, column+" LIKE ?")
			args = append(args, "ministry-%")
		}
		return query + " AND (" + strings.Join(parts, " OR ") + ")", args
	}

	for _, candidate := range backendCategoryCandidates(category) {
		for _, column := range columns {
			parts = append(parts, column+"=?")
			args = append(args, candidate)
		}
	}
	return query + " AND (" + strings.Join(parts, " OR ") + ")", args
}

func backendCategoryCandidates(category string) []string {
	switch category {
	case "share", "ppt", "handout":
		return []string{"handout", "share", "ppt"}
	case "pdf", "passage":
		return []string{"passage", "pdf"}
	default:
		return []string{category}
	}
}
