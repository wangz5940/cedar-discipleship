package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	auditdomain "agp/backend/internal/audit"
	checkindomain "agp/backend/internal/checkin"
	notificationdomain "agp/backend/internal/notification"
)

type notificationCheckinRepository struct {
	checkindomain.Repository
	existing bool
	saveErr  error
}

func (r *notificationCheckinRepository) ValidateWeeklyTarget(context.Context, uint64, uint64, uint64, string, string) error {
	return nil
}

func (r *notificationCheckinRepository) FindExistingWeeklyTask(context.Context, uint64, uint64, uint64, uint64, string) (uint64, error) {
	if r.existing {
		return 42, nil
	}
	return 0, sql.ErrNoRows
}

func (r *notificationCheckinRepository) Create(context.Context, *checkindomain.Record, uint64) (uint64, error) {
	return 42, r.saveErr
}

type notificationAuditRepository struct{ auditdomain.Repository }

func (notificationAuditRepository) Create(context.Context, auditdomain.Log) error { return nil }

type recordingNotifier struct {
	events []notificationdomain.Event
	err    error
}

func (n *recordingNotifier) Enqueue(event notificationdomain.Event) error {
	n.events = append(n.events, event)
	return n.err
}

func TestCreateCheckinNotification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		existing     bool
		saveErr      error
		notifyErr    error
		wantStatus   int
		wantEnqueues int
	}{
		{"new checkin", false, nil, nil, http.StatusCreated, 1},
		{"idempotent retry", true, nil, nil, http.StatusOK, 0},
		{"save failure", false, errors.New("save failed"), nil, http.StatusConflict, 0},
		{"notification disk failure", false, nil, errors.New("disk full"), http.StatusCreated, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &recordingNotifier{err: tt.notifyErr}
			a := &app{
				location: time.FixedZone("CST", 8*3600),
				checkins: checkindomain.NewService(&notificationCheckinRepository{
					existing: tt.existing, saveErr: tt.saveErr,
				}),
				audits:        auditdomain.NewService(notificationAuditRepository{}),
				notifications: notifier,
			}
			request := httptest.NewRequest(http.MethodPost, "/api/checkins", strings.NewReader(
				`{"task_type":"weekly_video","task_id":3,"week_id":4,"logical_date":"2026-01-01"}`,
			))
			request = request.WithContext(context.WithValue(request.Context(), currentUserKey, currentUser{
				ID: 2, CurrentGroupID: 1,
			}))
			response := httptest.NewRecorder()
			a.handleCreateCheckin(response, request)
			if response.Code != tt.wantStatus || len(notifier.events) != tt.wantEnqueues {
				t.Fatalf("status=%d enqueues=%d body=%s", response.Code, len(notifier.events), response.Body)
			}
			if len(notifier.events) == 1 {
				event := notifier.events[0]
				if event.RecordID != 42 || event.GroupID != 1 || event.LogicalDate != "2026-01-01" {
					t.Fatalf("wrong notification event: %#v", event)
				}
			}
		})
	}
}
