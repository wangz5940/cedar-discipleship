package statistics

import (
	"context"
	"testing"
	"time"
)

type serviceTestRepository struct {
	members []Member
	counts  []TaskCount
	from    string
	to      string
}

func (r *serviceTestRepository) DailySummary(context.Context, uint64, string, string) (map[string]int, error) {
	return nil, nil
}

func (r *serviceTestRepository) Members(context.Context, uint64) ([]Member, error) {
	return r.members, nil
}

func (r *serviceTestRepository) MonthlyTaskCounts(_ context.Context, _ uint64, from, to string) ([]TaskCount, error) {
	r.from = from
	r.to = to
	return r.counts, nil
}

func (r *serviceTestRepository) MemberCalendar(context.Context, uint64, uint64, string, string) ([]CalendarItem, error) {
	return nil, nil
}

func (r *serviceTestRepository) LearningTotals(context.Context, uint64, uint64) (*LearningTotals, error) {
	return nil, nil
}

func TestMonthlyRankingUsesExplicitDateRange(t *testing.T) {
	t.Parallel()

	repo := &serviceTestRepository{
		members: []Member{{MemberID: 1, UserID: 2, Username: "user", MemberName: "成员"}},
		counts:  []TaskCount{{UserID: 2, TaskType: "daily_devotion", Count: 3}},
	}
	service := NewService(repo)

	result, err := service.MonthlyRanking(
		context.Background(),
		1,
		"",
		"2026-08-03",
		"2026-08-19",
		time.FixedZone("CST", 8*60*60),
	)
	if err != nil {
		t.Fatalf("MonthlyRanking() error = %v", err)
	}
	if repo.from != "2026-08-03" || repo.to != "2026-08-19" {
		t.Fatalf("query range = %s..%s, want 2026-08-03..2026-08-19", repo.from, repo.to)
	}
	if result.From != "2026-08-03" || result.To != "2026-08-19" {
		t.Fatalf("result range = %s..%s", result.From, result.To)
	}
	if result.Items[0].Counts["daily_devotion"] != 3 {
		t.Fatalf("daily devotion count = %d, want 3", result.Items[0].Counts["daily_devotion"])
	}
}

func TestMonthlyRankingRejectsInvalidDateRange(t *testing.T) {
	t.Parallel()

	service := NewService(&serviceTestRepository{})
	_, err := service.MonthlyRanking(
		context.Background(),
		1,
		"",
		"2026-08-20",
		"2026-08-19",
		time.FixedZone("CST", 8*60*60),
	)
	if err != ErrInvalidDateRange {
		t.Fatalf("MonthlyRanking() error = %v, want %v", err, ErrInvalidDateRange)
	}
}
