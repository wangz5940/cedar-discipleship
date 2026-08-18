package ministry

import (
	"context"
	"errors"
	"testing"
	"time"
)

type serviceTestRepository struct {
	Repository
	group        GroupSummary
	access       Access
	members      []Member
	request      Request
	decideCalls  int
	createdShare Status
}

func (r *serviceTestRepository) EnsureCatalog(context.Context, uint64, time.Time) error {
	return nil
}

func (r *serviceTestRepository) Group(
	context.Context,
	uint64,
	uint64,
	uint64,
) (*GroupSummary, error) {
	group := r.group
	return &group, nil
}

func (r *serviceTestRepository) Access(context.Context, uint64, uint64, uint64) (Access, error) {
	return r.access, nil
}

func (r *serviceTestRepository) ListMembers(context.Context, uint64, uint64) ([]Member, error) {
	return r.members, nil
}

func (r *serviceTestRepository) ListShares(
	context.Context,
	uint64,
	uint64,
	uint64,
	bool,
) ([]Share, error) {
	return []Share{}, nil
}

func (r *serviceTestRepository) ListProgress(context.Context, uint64, uint64, int) ([]Progress, error) {
	return []Progress{}, nil
}

func (r *serviceTestRepository) Request(context.Context, uint64, uint64) (*Request, error) {
	request := r.request
	return &request, nil
}

func (r *serviceTestRepository) DecideRequest(
	context.Context,
	uint64,
	uint64,
	uint64,
	Status,
	time.Time,
) error {
	r.decideCalls++
	return nil
}

func (r *serviceTestRepository) CreateShare(
	_ context.Context,
	_, _, _ uint64,
	_ ShareInput,
	status Status,
	_ time.Time,
) (uint64, error) {
	r.createdShare = status
	return 1, nil
}

func (r *serviceTestRepository) UpdateSettings(
	context.Context,
	uint64,
	uint64,
	GroupSettingsInput,
	time.Time,
) error {
	return nil
}

func TestServiceDetailIdentityVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		visibility      Visibility
		identityPublic  bool
		actor           Actor
		expectedVisible bool
	}{
		{
			name:            "group default shows all members",
			visibility:      VisibilityAll,
			expectedVisible: true,
		},
		{
			name:            "private default hides another member",
			visibility:      VisibilitySelf,
			expectedVisible: false,
		},
		{
			name:            "member public choice overrides private default",
			visibility:      VisibilitySelf,
			identityPublic:  true,
			expectedVisible: true,
		},
		{
			name:            "study admin sees private identities",
			visibility:      VisibilitySelf,
			actor:           Actor{IsStudyAdmin: true},
			expectedVisible: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := &serviceTestRepository{
				group: GroupSummary{
					ID:               3,
					MemberVisibility: test.visibility,
				},
				access: Access{MemberVisibility: test.visibility},
				members: []Member{{
					UserID:         9,
					DisplayName:    "成员甲",
					Role:           MemberRoleMember,
					IdentityPublic: test.identityPublic,
				}},
			}
			service := NewService(repo)
			detail, err := service.Detail(
				context.Background(),
				1,
				3,
				Actor{
					UserID:       7,
					IsStudyAdmin: test.actor.IsStudyAdmin,
				},
			)
			if err != nil {
				t.Fatalf("Detail() error = %v", err)
			}
			if got := detail.Members[0].IsVisible; got != test.expectedVisible {
				t.Fatalf("member visibility = %v, want %v", got, test.expectedVisible)
			}
			if !test.expectedVisible && detail.Members[0].UserID != 0 {
				t.Fatalf("hidden member user id = %d, want 0", detail.Members[0].UserID)
			}
		})
	}
}

func TestServiceDecideRequestRejectsReviewedRequest(t *testing.T) {
	t.Parallel()

	repo := &serviceTestRepository{
		request: Request{ID: 12, GroupID: 3, Status: StatusApproved},
		access:  Access{IsAdmin: true},
	}
	service := NewService(repo)
	err := service.DecideRequest(
		context.Background(),
		1,
		12,
		Actor{UserID: 7},
		StatusApproved,
		time.Now(),
	)
	if !errors.Is(err, ErrRequestAlreadyReviewed) {
		t.Fatalf("DecideRequest() error = %v, want %v", err, ErrRequestAlreadyReviewed)
	}
	if repo.decideCalls != 0 {
		t.Fatalf("repository decision calls = %d, want 0", repo.decideCalls)
	}
}

func TestServiceCreateShareChoosesApprovalStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		access         Access
		actor          Actor
		expectedStatus Status
	}{
		{
			name:           "member share waits for leader",
			access:         Access{IsMember: true},
			expectedStatus: StatusPending,
		},
		{
			name:           "auto approval publishes member share",
			access:         Access{IsMember: true, ShareAutoApprove: true},
			expectedStatus: StatusPublished,
		},
		{
			name:           "leader share waits for admin",
			access:         Access{IsMember: true, IsLeader: true},
			expectedStatus: StatusPending,
		},
		{
			name:           "ministry admin publishes directly",
			access:         Access{IsMember: true, IsAdmin: true},
			expectedStatus: StatusPublished,
		},
		{
			name:           "study admin has all permissions",
			actor:          Actor{IsStudyAdmin: true},
			expectedStatus: StatusPublished,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := &serviceTestRepository{access: test.access}
			service := NewService(repo)
			_, err := service.CreateShare(
				context.Background(),
				1,
				3,
				Actor{UserID: 7, IsStudyAdmin: test.actor.IsStudyAdmin},
				ShareInput{Title: "经验", Body: "正文"},
				time.Now(),
			)
			if err != nil {
				t.Fatalf("CreateShare() error = %v", err)
			}
			if repo.createdShare != test.expectedStatus {
				t.Fatalf("created share status = %q, want %q", repo.createdShare, test.expectedStatus)
			}
		})
	}
}

func TestServiceDecideRequestAllowsMinistryAdmin(t *testing.T) {
	t.Parallel()

	repo := &serviceTestRepository{
		request: Request{ID: 12, GroupID: 3, Status: StatusPending},
		access:  Access{IsMember: true, IsAdmin: true},
	}
	service := NewService(repo)
	err := service.DecideRequest(
		context.Background(),
		1,
		12,
		Actor{UserID: 7},
		StatusApproved,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("DecideRequest() error = %v", err)
	}
	if repo.decideCalls != 1 {
		t.Fatalf("repository decision calls = %d, want 1", repo.decideCalls)
	}
}

func TestServiceUpdateSettingsRestrictsLeaderAssignment(t *testing.T) {
	t.Parallel()

	leaderUserID := uint64(9)
	service := NewService(&serviceTestRepository{access: Access{IsLeader: true}})
	err := service.UpdateSettings(
		context.Background(),
		1,
		3,
		Actor{UserID: 7},
		GroupSettingsInput{LeaderUserID: &leaderUserID},
		time.Now(),
	)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("UpdateSettings() error = %v, want %v", err, ErrForbidden)
	}
}
