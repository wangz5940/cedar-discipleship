package learning

import (
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A separate marker for every verse avoids guessing boundaries from punctuation.
var verseLineMarker = regexp.MustCompile(`(?m)^[ \t]*(?:【|\[)?([1-3]?[\p{Han}A-Za-z]+)[ \t]*([0-9]{1,3})[ \t]*[:：][ \t]*([0-9]{1,3})(?:】|\])?[ \t]*`)

type verseSegment struct{ ref, text string }

func splitVerseLines(text string) ([]verseSegment, bool) {
	matches := verseLineMarker.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 || strings.TrimSpace(text[:matches[0][0]]) != "" {
		return nil, false
	}
	segments := make([]verseSegment, 0, len(matches))
	seen := map[string]bool{}
	for index, match := range matches {
		end := len(text)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		body := strings.TrimSpace(text[match[1]:end])
		chapter, _ := strconv.Atoi(text[match[4]:match[5]])
		verse, _ := strconv.Atoi(text[match[6]:match[7]])
		ref := text[match[2]:match[3]] + strconv.Itoa(chapter) + ":" + strconv.Itoa(verse)
		if body == "" || strings.ContainsRune("-–—－:：0123456789", []rune(body)[0]) || chapter < 1 || verse < 1 || seen[ref] {
			return nil, false
		}
		seen[ref] = true
		segments = append(segments, verseSegment{ref: ref, text: strings.TrimSpace(text[match[0]:end])})
	}
	return segments, true
}

func resolveVerseProgression(plan map[string]any, date string) (map[string]any, bool) {
	if sources, exists := plan["verse_sources"].([]any); exists && !WeeklyVersePlan(plan) {
		refs, texts := []string{}, []string{}
		for _, item := range sources {
			source, ok := item.(map[string]any)
			if !ok {
				continue
			}
			part := map[string]any{"date": plan["date"], "completion_mode": "daily", "verse_ref": source["verse_ref"], "recite_text": source["recite_text"], "verses_per_day": source["verses_per_day"], "progression_start_date": source["progression_start_date"]}
			if resolved, active := resolveVerseProgression(part, date); active {
				refs = append(refs, asString(resolved["verse_ref"]))
				texts = append(texts, asString(resolved["recite_text"]))
			}
		}
		if len(texts) == 0 {
			return nil, false
		}
		resolved := maps.Clone(plan)
		resolved["verse_ref"], resolved["recite_text"] = strings.Join(refs, "，"), strings.Join(texts, "\n")
		return resolved, true
	}
	value, exists := plan["verses_per_day"]
	if WeeklyVersePlan(plan) || !exists {
		return plan, true
	}
	count, ok := value.(float64)
	segments, valid := splitVerseLines(asString(plan["recite_text"]))
	start, err := time.Parse("2006-01-02", firstNonEmpty(asString(plan["progression_start_date"]), asString(plan["date"])))
	day, dayErr := time.Parse("2006-01-02", date)
	if !ok || count < 1 || count > 100 || !valid || err != nil || dayErr != nil {
		return nil, false
	}
	offset := int(day.Sub(start)/(24*time.Hour)) * int(count)
	if offset < 0 || offset >= len(segments) {
		return nil, false
	}
	end := min(offset+int(count), len(segments))
	refs, texts := []string{}, []string{}
	for _, segment := range segments[offset:end] {
		refs = append(refs, segment.ref)
		texts = append(texts, segment.text)
	}
	resolved := maps.Clone(plan)
	resolved["verse_ref"], resolved["recite_text"] = strings.Join(refs, "，"), strings.Join(texts, "\n")
	return resolved, true
}
