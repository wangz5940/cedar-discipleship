package statistics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidMonth     = errors.New("invalid_month")
	ErrInvalidDateRange = errors.New("invalid_date_range")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Summary(ctx context.Context, groupID uint64, from, to string) (SummaryVO, error) {
	summary, err := s.repo.DailySummary(ctx, groupID, from, to)
	if err != nil {
		return SummaryVO{}, err
	}
	return SummaryVO{From: from, To: to, Summary: summary}, nil
}

func (s *Service) MonthlyRanking(ctx context.Context, groupID uint64, month, fromInput, toInput string, loc *time.Location) (MonthlyRankingVO, error) {
	start, end, err := rankingRange(month, fromInput, toInput, loc)
	if err != nil {
		return MonthlyRankingVO{}, err
	}
	from, to := start.Format("2006-01-02"), end.Format("2006-01-02")

	members, err := s.repo.Members(ctx, groupID)
	if err != nil {
		return MonthlyRankingVO{}, err
	}
	byUser := map[uint64]*MonthlyRankingItemVO{}
	for _, member := range members {
		byUser[member.UserID] = &MonthlyRankingItemVO{
			MemberID:    member.MemberID,
			UserID:      member.UserID,
			Username:    member.Username,
			DisplayName: member.DisplayName,
			MemberName:  member.MemberName,
			Counts: map[string]int{
				"daily_devotion": 0,
				"weekly_book":    0,
				"weekly_video":   0,
				"weekly_outline": 0,
			},
		}
	}
	counts, err := s.repo.MonthlyTaskCounts(ctx, groupID, from, to)
	if err != nil {
		return MonthlyRankingVO{}, err
	}
	for _, count := range counts {
		item, ok := byUser[count.UserID]
		if !ok {
			continue
		}
		item.Counts[count.TaskType] = count.Count
		item.Total += count.Count
	}
	items := make([]MonthlyRankingItemVO, 0, len(byUser))
	for _, item := range byUser {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Total != items[j].Total {
			return items[i].Total > items[j].Total
		}
		if items[i].Counts["daily_devotion"] != items[j].Counts["daily_devotion"] {
			return items[i].Counts["daily_devotion"] > items[j].Counts["daily_devotion"]
		}
		if items[i].Counts["weekly_book"] != items[j].Counts["weekly_book"] {
			return items[i].Counts["weekly_book"] > items[j].Counts["weekly_book"]
		}
		if items[i].Counts["weekly_outline"] != items[j].Counts["weekly_outline"] {
			return items[i].Counts["weekly_outline"] > items[j].Counts["weekly_outline"]
		}
		return items[i].UserID < items[j].UserID
	})
	return MonthlyRankingVO{Month: start.Format("2006-01"), From: from, To: to, Items: items}, nil
}

func rankingRange(month, fromInput, toInput string, loc *time.Location) (time.Time, time.Time, error) {
	month = strings.TrimSpace(month)
	fromInput = strings.TrimSpace(fromInput)
	toInput = strings.TrimSpace(toInput)
	now := time.Now().In(loc)
	if fromInput != "" || toInput != "" {
		if fromInput == "" {
			fromInput = formatMonthStart(now)
		}
		if toInput == "" {
			toInput = now.Format("2006-01-02")
		}
		start, err := time.ParseInLocation("2006-01-02", fromInput, loc)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidDateRange
		}
		end, err := time.ParseInLocation("2006-01-02", toInput, loc)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidDateRange
		}
		if end.Before(start) {
			return time.Time{}, time.Time{}, ErrInvalidDateRange
		}
		return start, end, nil
	}
	if month == "" {
		start, err := time.ParseInLocation("2006-01-02", formatMonthStart(now), loc)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidDateRange
		}
		return start, now, nil
	}
	start, err := time.ParseInLocation("2006-01-02", month+"-01", loc)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidMonth
	}
	end := start.AddDate(0, 1, -1)
	if start.Year() == now.Year() && start.Month() == now.Month() {
		end = now
	}
	return start, end, nil
}

func formatMonthStart(value time.Time) string {
	return value.Format("2006-01") + "-01"
}

func (s *Service) MemberCalendar(ctx context.Context, groupID, userID uint64, month string, loc *time.Location) (MemberCalendarVO, error) {
	month = strings.TrimSpace(month)
	if month == "" {
		month = time.Now().In(loc).Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01-02", month+"-01", loc)
	if err != nil {
		return MemberCalendarVO{}, ErrInvalidMonth
	}
	end := start.AddDate(0, 1, -1)
	items, err := s.repo.MemberCalendar(ctx, groupID, userID, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return MemberCalendarVO{}, err
	}
	out := make([]CalendarItemVO, 0, len(items))
	for _, item := range items {
		out = append(out, CalendarItemVO(item))
	}
	return MemberCalendarVO{Items: out}, nil
}
