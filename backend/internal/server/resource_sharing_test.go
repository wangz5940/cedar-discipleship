package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	assetdomain "agp/backend/internal/asset"
	auditdomain "agp/backend/internal/audit"
	learningdomain "agp/backend/internal/learning"
	notificationdomain "agp/backend/internal/notification"
)

type renameAssetRepository struct {
	item         assetdomain.Asset
	renamedTitle string
}

func (r *renameAssetRepository) FindByID(context.Context, uint64, uint64) (*assetdomain.Asset, error) {
	item := r.item
	item.Title = r.renamedTitle
	return &item, nil
}

func (*renameAssetRepository) List(context.Context, uint64, int) ([]assetdomain.Asset, error) {
	return nil, errors.New("not implemented")
}

func (*renameAssetRepository) Create(context.Context, *assetdomain.Asset, uint64) (uint64, error) {
	return 0, errors.New("not implemented")
}

func (*renameAssetRepository) Delete(context.Context, uint64, uint64) error {
	return errors.New("not implemented")
}

func (r *renameAssetRepository) Rename(_ context.Context, _, _ uint64, title string, _ time.Time) error {
	r.renamedTitle = title
	return nil
}

type renameNotifier struct {
	enqueues int
	wakes    int
}

func (n *renameNotifier) Enqueue(notificationdomain.Event) error {
	n.enqueues++
	return nil
}

func (n *renameNotifier) WakeInitial(uint64, time.Time) error {
	n.wakes++
	return nil
}

func TestRenameAssetInvalidatesTodayContentWithoutSendingNotification(t *testing.T) {
	t.Parallel()

	location := time.FixedZone("CST", 8*60*60)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, location)
	cache := newTodayContentCache(4, 1<<20, location)
	for _, groupID := range []uint64{1, 2} {
		_, _, _ = cache.GetOrLoad(context.Background(), groupID, "2026-09-25", now, func(context.Context) (learningdomain.TodayContent, error) {
			return testTodayContent("2026-09-25", "旧名称"), nil
		})
	}

	repo := &renameAssetRepository{item: assetdomain.Asset{ID: 12, GroupID: 1, Title: "旧名称"}}
	notifier := &renameNotifier{}
	a := &app{
		assets:        assetdomain.NewService(repo, nil, ""),
		audits:        auditdomain.NewService(notificationAuditRepository{}),
		notifications: notifier,
		todayCache:    cache,
	}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/assets/12/title", strings.NewReader(`{"title":"新名称"}`))
	request.SetPathValue("id", "12")
	request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
		ID:             7,
		CurrentGroupID: 1,
	}))
	response := httptest.NewRecorder()

	a.handleRenameAsset(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if metrics := cache.Metrics(); metrics.Entries != 0 {
		t.Fatalf("today cache entries = %d, want 0", metrics.Entries)
	}
	if notifier.enqueues != 0 || notifier.wakes != 0 {
		t.Fatalf("notification calls = enqueues %d, wakes %d; want none", notifier.enqueues, notifier.wakes)
	}
}

func TestWriteAssetErrorMapsReferencedResourceToConflict(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()

	(&app{}).writeAssetError(response, assetdomain.ErrAssetInUse)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "asset_in_use") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
