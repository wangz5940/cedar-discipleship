package statistics

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type serviceTestRepository struct {
	members     []Member
	counts      []TaskCount
	videoCounts []TaskCount
	summary     map[string]int
	from        string
	to          string
}

func (r *serviceTestRepository) DailySummary(context.Context, uint64, string, string) (map[string]int, error) {
	return r.summary, nil
}

func (r *serviceTestRepository) Members(context.Context, uint64) ([]Member, error) {
	return r.members, nil
}

func (r *serviceTestRepository) MonthlyNonVideoTaskCounts(_ context.Context, _ uint64, from, to string) ([]TaskCount, error) {
	r.from = from
	r.to = to
	return r.counts, nil
}

func (r *serviceTestRepository) MonthlyVideoCompletionCounts(_ context.Context, _ uint64, from, to string) ([]TaskCount, error) {
	r.from = from
	r.to = to
	return r.videoCounts, nil
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

func TestSummaryUsesVideoCompletionCounts(t *testing.T) {
	t.Parallel()

	service := NewService(&serviceTestRepository{
		summary: map[string]int{
			"daily_devotion":  4,
			"daily_scripture": 2,
			"weekly_checkin":  1,
			"weekly_video":    0,
		},
		videoCounts: []TaskCount{
			{UserID: 2, TaskType: "weekly_video", Count: 1},
			{UserID: 3, TaskType: "weekly_video", Count: 2},
		},
	})

	result, err := service.Summary(context.Background(), 1, "2026-09-01", "2026-09-06")
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if got := result.Summary["weekly_video"]; got != 3 {
		t.Fatalf("weekly video summary = %d, want 3", got)
	}
	if got := result.Summary["daily_devotion"]; got != 4 {
		t.Fatalf("daily devotion summary = %d, want 4", got)
	}
	if result.Summary["daily_scripture"] != 2 || result.Summary["weekly_checkin"] != 1 {
		t.Fatalf("summary lost independent task identities: %#v", result.Summary)
	}
}

func TestMonthlyRankingIncludesUniqueVideoResourceCompletions(t *testing.T) {
	t.Parallel()

	repo := &serviceTestRepository{
		members:     []Member{{MemberID: 1, UserID: 2, Username: "user", MemberName: "成员"}},
		counts:      []TaskCount{{UserID: 2, TaskType: "daily_devotion", Count: 3}},
		videoCounts: []TaskCount{{UserID: 2, TaskType: "weekly_video", Count: 1}},
	}
	service := NewService(repo)

	result, err := service.MonthlyRanking(
		context.Background(),
		1,
		"",
		"2026-09-01",
		"2026-09-06",
		time.FixedZone("CST", 8*60*60),
	)
	if err != nil {
		t.Fatalf("MonthlyRanking() error = %v", err)
	}
	if got := result.Items[0].Counts["weekly_video"]; got != 1 {
		t.Fatalf("weekly video count = %d, want 1", got)
	}
	if got := result.Items[0].Total; got != 4 {
		t.Fatalf("total = %d, want 4", got)
	}
}

func TestMonthlyRankingTaskTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		counts []TaskCount
		want   map[string]int
		total  int
	}{
		{
			name: "empty counts expose independent types",
			want: map[string]int{
				"daily_devotion": 0, "daily_scripture": 0, "weekly_checkin": 0,
				"weekly_book": 0, "weekly_video": 0, "weekly_verse": 0, "weekly_outline": 0,
			},
		},
		{
			name: "scripture and aggregate do not complete devotion or books",
			counts: []TaskCount{
				{UserID: 2, TaskType: "daily_scripture", Count: 3},
				{UserID: 2, TaskType: "weekly_checkin", Count: 1},
				{UserID: 99, TaskType: "daily_scripture", Count: 50},
			},
			want: map[string]int{
				"daily_devotion": 0, "daily_scripture": 3, "weekly_checkin": 1,
				"weekly_book": 0, "weekly_video": 0, "weekly_verse": 0, "weekly_outline": 0,
			},
			total: 4,
		},
		{
			name: "old and new task counts coexist",
			counts: []TaskCount{
				{UserID: 2, TaskType: "daily_devotion", Count: 2},
				{UserID: 2, TaskType: "daily_scripture", Count: 3},
				{UserID: 2, TaskType: "weekly_checkin", Count: 1},
				{UserID: 2, TaskType: "weekly_book", Count: 4},
				{UserID: 2, TaskType: "weekly_outline", Count: 1},
			},
			want: map[string]int{
				"daily_devotion": 2, "daily_scripture": 3, "weekly_checkin": 1,
				"weekly_book": 4, "weekly_video": 0, "weekly_verse": 0, "weekly_outline": 1,
			},
			total: 11,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(&serviceTestRepository{
				members: []Member{{MemberID: 1, UserID: 2, MemberName: "member"}},
				counts:  tt.counts,
			})
			result, err := service.MonthlyRanking(t.Context(), 1, "2026-08", "", "", time.UTC)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("ranking includes nonmembers: %#v", result.Items)
			}
			item := result.Items[0]
			if !reflect.DeepEqual(item.Counts, tt.want) || item.Total != tt.total {
				t.Fatalf("ranking = %#v, want counts=%#v total=%d", item, tt.want, tt.total)
			}
		})
	}
	t.Run("legacy devotion tie breaker remains unchanged", func(t *testing.T) {
		service := NewService(&serviceTestRepository{
			members: []Member{{UserID: 1}, {UserID: 2}},
			counts: []TaskCount{
				{UserID: 1, TaskType: "daily_scripture", Count: 2},
				{UserID: 2, TaskType: "daily_devotion", Count: 2},
			},
		})
		result, err := service.MonthlyRanking(t.Context(), 1, "2026-08", "", "", time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Items) != 2 || result.Items[0].UserID != 2 {
			t.Fatalf("legacy tie breaker changed: %#v", result.Items)
		}
	})
}
