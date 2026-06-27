package utils

import "time"

func TodayIn(loc *time.Location) string {
	return time.Now().In(loc).Format("2006-01-02")
}
