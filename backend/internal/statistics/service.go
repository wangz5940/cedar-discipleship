package statistics

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

var ErrInvalidMonth = errors.New("invalid_month")

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

func (s *Service) MonthlyRanking(ctx context.Context, groupID uint64, month string, loc *time.Location) (MonthlyRankingVO, error) {
	month = strings.TrimSpace(month)
	now := time.Now().In(loc)
	if month == "" {
		month = now.Format("2006-01")
	}
	start, err := time.ParseInLocation("2006-01-02", month+"-01", loc)
	if err != nil {
		return MonthlyRankingVO{}, ErrInvalidMonth
	}
	end := start.AddDate(0, 1, -1)
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
				"weekly_verse":   0,
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
		if items[i].Counts["weekly_verse"] != items[j].Counts["weekly_verse"] {
			return items[i].Counts["weekly_verse"] > items[j].Counts["weekly_verse"]
		}
		return items[i].UserID < items[j].UserID
	})
	return MonthlyRankingVO{Month: month, From: from, To: to, Items: items}, nil
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
		out = append(out, CalendarItemVO{Date: item.Date, TaskType: item.TaskType, Part: item.Part})
	}
	return MemberCalendarVO{Items: out}, nil
}
