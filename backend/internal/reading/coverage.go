package reading

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var pageRange = regexp.MustCompile(`([0-9]{1,4})\s*(?:[-~—–至到]\s*([0-9]{1,4}))?\s*页`)

// Covers compares explicit page ranges. Callers must first verify the same
// group, week and resource; titles alone do not identify a document.
func Covers(completedTitle, completedContent, targetTitle, targetContent string) bool {
	start, end := pages(completedTitle, completedContent)
	targetStart, targetEnd := pages(targetTitle, targetContent)
	return start > 0 && targetStart > 0 && start <= targetStart && end >= targetEnd
}

func pages(title, content string) (int, int) {
	if match := pageRange.FindStringSubmatch(title); match != nil {
		start, _ := strconv.Atoi(match[1])
		end := start
		if match[2] != "" {
			end, _ = strconv.Atoi(match[2])
		}
		if start > 0 && end >= start {
			return start, end
		}
		return 0, 0
	}
	var metadata struct {
		Start int    `json:"page_start"`
		End   int    `json:"page_end"`
		Title string `json:"source_title"`
	}
	if !strings.HasPrefix(strings.TrimSpace(content), "{") || json.Unmarshal([]byte(content), &metadata) != nil {
		return 0, 0
	}
	if metadata.Title != "" && pageRange.MatchString(metadata.Title) {
		return pages(metadata.Title, "")
	}
	if metadata.Start > 0 && metadata.End >= metadata.Start {
		return metadata.Start, metadata.End
	}
	return 0, 0
}
