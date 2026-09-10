package notification

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Event struct {
	RecordID    uint64    `json:"record_id"`
	GroupID     uint64    `json:"group_id"`
	LogicalDate string    `json:"logical_date"`
	OccurredAt  time.Time `json:"occurred_at"`
	Initial     string    `json:"initial,omitempty"`
}

type Entry struct {
	RecordID uint64
	UserID   uint64
	Name     string
	TaskType string
	BookName string
}

type Snapshot struct {
	// Text is the complete current-period summary; diffs only decide whether to send it.
	Text      string
	ExpiresAt time.Time
	Topic     string
	Version   string
}

// Entries arrive in first-checkin order, independent of member names.
func FormatCheckins(entries []Entry, recordID uint64, daily bool) string {
	type content struct {
		key, label string
		isNew      bool
	}
	type member struct {
		name     string
		contents []content
	}
	var members []member
	positions := make(map[uint64]int)
	found := false
	for _, entry := range entries {
		if daily && entry.TaskType != "daily_devotion" {
			continue
		}
		var key, label string
		if !daily {
			switch entry.TaskType {
			case "weekly_book":
				key = cleanText(entry.BookName)
				if key == "" {
					continue
				}
				chars := []rune(strings.Trim(key, "《》「」"))
				if len(chars) > 2 {
					chars = chars[:2]
				}
				label = string(chars)
				key = "book:" + key
			case "weekly_video":
				key, label = "video", "视频"
			default:
				continue
			}
		}
		isNew := recordID != 0 && entry.RecordID == recordID
		found = found || isNew
		index, ok := positions[entry.UserID]
		if !ok {
			index = len(members)
			positions[entry.UserID] = index
			members = append(members, member{name: cleanText(entry.Name)})
		}
		m := &members[index]
		if daily {
			continue
		}
		exists := false
		for i := range m.contents {
			if m.contents[i].key == key {
				m.contents[i].isNew = m.contents[i].isNew || isNew
				exists = true
				break
			}
		}
		if !exists {
			m.contents = append(m.contents, content{key: key, label: label, isNew: isNew})
		}
	}
	if recordID != 0 && !found {
		return ""
	}
	title := "本周任务"
	if daily {
		title = "每日灵修"
	}
	lines := []string{title}
	if len(members) == 0 {
		lines = append(lines, "暂无打卡记录")
	}
	for i, m := range members {
		parts := []string{fmt.Sprintf("%d %s", i+1, m.name)}
		for _, c := range m.contents {
			label := c.label
			if c.isNew {
				label = "【新】" + label
			}
			parts = append(parts, label)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return strings.Join(lines, "\n")
}

func cleanText(text string) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

func eligible(taskType, logicalDate, today, weekStart, weekEnd string) bool {
	switch taskType {
	case "daily_devotion":
		return logicalDate == today
	case "weekly_book", "weekly_video":
		return weekStart != "" && weekStart <= today && today <= weekEnd &&
			weekStart <= logicalDate && logicalDate <= weekEnd
	default:
		return false
	}
}

func splitMessage(text string) []string {
	const maxBytes = 3500
	var chunks []string
	for len(text) > maxBytes {
		end := maxBytes
		for !utf8.RuneStart(text[end]) {
			end--
		}
		if newline := strings.LastIndexByte(text[:end], '\n'); newline > 0 {
			end = newline
		} else if space := strings.LastIndexByte(text[:end], ' '); space > 0 {
			end = space
		}
		chunks = append(chunks, text[:end])
		text = strings.TrimSpace(text[end:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}
