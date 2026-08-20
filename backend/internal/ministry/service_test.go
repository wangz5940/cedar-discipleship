package ministry

import (
	"context"
	"errors"
	"testing"
	"time"
)

type serviceTestRepository struct {
	Repository
	group            GroupSummary
	access           Access
	members          []Member
	request          Request
	decideCalls      int
	createdShare     Status
	joinAutoApprove  bool
	createdGroup     GroupInput
	updatedGroup     GroupInput
	deletedGroupID   uint64
	pinnedShare      bool
	deletedShare     bool
	restoredShare    bool
	deletedProgress  bool
	restoredProgress bool
	deleteCanManage  bool
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

func (r *serviceTestRepository) ListDeletedShares(
	context.Context,
	uint64,
	uint64,
	uint64,
	bool,
	int,
) ([]Share, error) {
	return []Share{}, nil
}

func (r *serviceTestRepository) ListDeletedProgress(
	context.Context,
	uint64,
	uint64,
	uint64,
	bool,
	int,
) ([]Progress, error) {
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

func (r *serviceTestRepository) RequestJoin(
	_ context.Context,
	_, _, _ uint64,
	_ string,
	autoApprove bool,
	_ time.Time,
) (uint64, error) {
	r.joinAutoApprove = autoApprove
	return 1, nil
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

func (r *serviceTestRepository) CreateGroup(
	_ context.Context,
	_ uint64,
	input GroupInput,
	_ time.Time,
) (uint64, error) {
	r.createdGroup = input
	return 18, nil
}

func (r *serviceTestRepository) UpdateGroup(
	_ context.Context,
	_, _ uint64,
	input GroupInput,
	_ time.Time,
) error {
	r.updatedGroup = input
	return nil
}

func (r *serviceTestRepository) DeleteGroup(
	_ context.Context,
	_, groupID uint64,
	_ time.Time,
) error {
	r.deletedGroupID = groupID
	return nil
}

func (r *serviceTestRepository) SetSharePinned(
	_ context.Context,
	_, _, _ uint64,
	_ uint64,
	pinned bool,
	_ time.Time,
) error {
	r.pinnedShare = pinned
	return nil
}

func (r *serviceTestRepository) DeleteShare(
	_ context.Context,
	_, _, _, _ uint64,
	canManage bool,
	_ time.Time,
) error {
	r.deletedShare = true
	r.deleteCanManage = canManage
	return nil
}

func (r *serviceTestRepository) RestoreShare(
	_ context.Context,
	_, _, _, _ uint64,
	canManage bool,
	_ time.Time,
) error {
	r.restoredShare = true
	r.deleteCanManage = canManage
	return nil
}

func (r *serviceTestRepository) DeleteProgress(
	_ context.Context,
	_, _, _, _ uint64,
	canManage bool,
	_ time.Time,
) error {
	r.deletedProgress = true
	r.deleteCanManage = canManage
	return nil
}

func (r *serviceTestRepository) RestoreProgress(
	_ context.Context,
	_, _, _, _ uint64,
	canManage bool,
	_ time.Time,
) error {
	r.restoredProgress = true
	r.deleteCanManage = canManage
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

func TestServiceRequestJoinUsesAutoApprovalSetting(t *testing.T) {
	t.Parallel()

	repo := &serviceTestRepository{
		access: Access{ShareAutoApprove: true},
	}
	service := NewService(repo)
	if _, err := service.RequestJoin(context.Background(), 1, 3, Actor{UserID: 7}, "", time.Now()); err != nil {
		t.Fatalf("RequestJoin() error = %v", err)
	}
	if !repo.joinAutoApprove {
		t.Fatal("RequestJoin() did not pass auto approval setting to repository")
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

func TestServiceManageCatalogRequiresStudyAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   Actor
		wantErr error
	}{
		{name: "member forbidden", actor: Actor{UserID: 7}, wantErr: ErrForbidden},
		{name: "study admin allowed", actor: Actor{UserID: 7, IsStudyAdmin: true}},
		{name: "super admin allowed", actor: Actor{UserID: 7, IsSuperAdmin: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := &serviceTestRepository{}
			service := NewService(repo)
			_, err := service.CreateGroup(
				context.Background(),
				1,
				test.actor,
				GroupInput{Name: " 新专项组 ", Description: " 说明 "},
				time.Now(),
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateGroup() error = %v, want %v", err, test.wantErr)
			}
			err = service.UpdateGroup(
				context.Background(),
				1,
				18,
				test.actor,
				GroupInput{Name: " 修改后 ", Description: ""},
				time.Now(),
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("UpdateGroup() error = %v, want %v", err, test.wantErr)
			}
			err = service.DeleteGroup(context.Background(), 1, 18, test.actor, time.Now())
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("DeleteGroup() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil {
				if repo.createdGroup.Name != "新专项组" {
					t.Fatalf("created group name = %q, want trimmed name", repo.createdGroup.Name)
				}
				if repo.updatedGroup.Name != "修改后" {
					t.Fatalf("updated group name = %q, want trimmed name", repo.updatedGroup.Name)
				}
				if repo.deletedGroupID != 18 {
					t.Fatalf("deleted group id = %d, want 18", repo.deletedGroupID)
				}
			}
		})
	}
}

func TestServiceSetSharePinnedRequiresShareAdmin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actor   Actor
		access  Access
		wantErr error
	}{
		{
			name:    "member forbidden",
			actor:   Actor{UserID: 7},
			access:  Access{IsMember: true},
			wantErr: ErrForbidden,
		},
		{
			name:   "ministry admin allowed",
			actor:  Actor{UserID: 7},
			access: Access{IsMember: true, IsAdmin: true},
		},
		{
			name:  "study admin allowed",
			actor: Actor{UserID: 7, IsStudyAdmin: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := &serviceTestRepository{access: test.access}
			service := NewService(repo)
			err := service.SetSharePinned(
				context.Background(),
				1,
				3,
				12,
				test.actor,
				true,
				time.Now(),
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("SetSharePinned() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && !repo.pinnedShare {
				t.Fatal("SetSharePinned() did not call repository")
			}
		})
	}
}

func TestServiceContentDeletionUsesRepositoryAuthorization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		actor         Actor
		access        Access
		wantCanManage bool
		run           func(*Service, Actor) error
		called        func(*serviceTestRepository) bool
	}{
		{
			name:   "author deletes share",
			actor:  Actor{UserID: 7},
			access: Access{IsMember: true},
			run: func(service *Service, actor Actor) error {
				return service.DeleteShare(context.Background(), 1, 3, 12, actor, time.Now())
			},
			called: func(repo *serviceTestRepository) bool { return repo.deletedShare },
		},
		{
			name:          "ministry admin restores share",
			actor:         Actor{UserID: 8},
			access:        Access{IsMember: true, IsAdmin: true},
			wantCanManage: true,
			run: func(service *Service, actor Actor) error {
				return service.RestoreShare(context.Background(), 1, 3, 12, actor, time.Now())
			},
			called: func(repo *serviceTestRepository) bool { return repo.restoredShare },
		},
		{
			name:   "author deletes progress",
			actor:  Actor{UserID: 7},
			access: Access{IsMember: true},
			run: func(service *Service, actor Actor) error {
				return service.DeleteProgress(context.Background(), 1, 3, 15, actor, time.Now())
			},
			called: func(repo *serviceTestRepository) bool { return repo.deletedProgress },
		},
		{
			name:          "study admin restores progress",
			actor:         Actor{UserID: 9, IsStudyAdmin: true},
			wantCanManage: true,
			run: func(service *Service, actor Actor) error {
				return service.RestoreProgress(context.Background(), 1, 3, 15, actor, time.Now())
			},
			called: func(repo *serviceTestRepository) bool { return repo.restoredProgress },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			repo := &serviceTestRepository{access: test.access}
			if err := test.run(NewService(repo), test.actor); err != nil {
				t.Fatalf("content mutation returned error: %v", err)
			}
			if !test.called(repo) {
				t.Fatal("repository mutation was not called")
			}
			if repo.deleteCanManage != test.wantCanManage {
				t.Fatalf("canManage = %v, want %v", repo.deleteCanManage, test.wantCanManage)
			}
		})
	}
}

func TestContentVODeletionPermissions(t *testing.T) {
	t.Parallel()

	deletedAt := time.Now()
	tests := []struct {
		name        string
		actor       Actor
		access      Access
		deletedAt   *time.Time
		wantDelete  bool
		wantRestore bool
		wantEdit    bool
		wantPin     bool
	}{
		{
			name:       "author can delete active content",
			actor:      Actor{UserID: 7},
			wantDelete: true,
			wantEdit:   true,
		},
		{
			name:        "author can restore deleted content",
			actor:       Actor{UserID: 7},
			deletedAt:   &deletedAt,
			wantRestore: true,
		},
		{
			name:       "ministry admin can delete active content",
			actor:      Actor{UserID: 8},
			access:     Access{IsAdmin: true},
			wantDelete: true,
			wantEdit:   true,
			wantPin:    true,
		},
		{
			name:   "another member cannot mutate content",
			actor:  Actor{UserID: 8},
			access: Access{IsMember: true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			shares := shareVOs([]Share{{
				ID: 1, AuthorID: 7, Status: StatusPublished, DeletedAt: test.deletedAt,
			}}, test.actor, test.access)
			if shares[0].CanDelete != test.wantDelete || shares[0].CanRestore != test.wantRestore {
				t.Fatalf(
					"share permissions delete=%v restore=%v, want delete=%v restore=%v",
					shares[0].CanDelete,
					shares[0].CanRestore,
					test.wantDelete,
					test.wantRestore,
				)
			}
			if shares[0].CanEdit != test.wantEdit || shares[0].CanPin != test.wantPin {
				t.Fatalf(
					"share active permissions edit=%v pin=%v, want edit=%v pin=%v",
					shares[0].CanEdit,
					shares[0].CanPin,
					test.wantEdit,
					test.wantPin,
				)
			}

			progress := progressVOs([]Progress{{
				ID: 1, AuthorID: 7, DeletedAt: test.deletedAt,
			}}, test.actor, test.access)
			if progress[0].CanDelete != test.wantDelete || progress[0].CanRestore != test.wantRestore {
				t.Fatalf(
					"progress permissions delete=%v restore=%v, want delete=%v restore=%v",
					progress[0].CanDelete,
					progress[0].CanRestore,
					test.wantDelete,
					test.wantRestore,
				)
			}
		})
	}
}
