package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	feedbackdomain "agp/backend/internal/feedback"
	"agp/backend/internal/logctx"
	notificationdomain "agp/backend/internal/notification"
)

func TestStreamRejectsAsBeforeAndAggregatesTrustedFailures(t *testing.T) {
	for _, reason := range []string{"expired", "session", "forged"} {
		t.Run(reason, func(t *testing.T) {
			repo := &feedbackHandlerRepository{settings: feedbackdomain.AutomaticSettings{Enabled: true}}
			service := feedbackdomain.NewService(repo, &feedbackHandlerStorage{})
			reporter := newAutomaticFeedbackReporter(service)
			a := &app{secret: []byte("test-secret"), automaticFeedback: reporter}
			expires := time.Now().Add(-time.Hour).Unix()
			if reason == "session" {
				expires = time.Now().Add(time.Hour).Unix()
				a.db = &sql.DB{} // No cookie: rejection occurs before any database call.
			}
			signature := a.signAssetPlayback(928, 7, expires)
			if reason == "forged" {
				signature = "forged-signature"
			}
			for index := range 20 {
				request := httptest.NewRequest(http.MethodGet,
					fmt.Sprintf("/api/assets/928/stream?group_id=7&expires=%d&signature=%s", expires, signature), nil)
				request.SetPathValue("id", "928")
				request = request.WithContext(logctx.WithLogID(request.Context(), "0123456789abcdef0123456789abcdef"))
				response := httptest.NewRecorder()
				a.handleStreamAsset(response, request)
				if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "invalid_playback_url") {
					t.Fatalf("playback response changed: %d %s", response.Code, response.Body)
				}
				if index < 2 && len(reporter.queue) != 0 {
					t.Fatal("isolated failures should not report")
				}
			}
			if reason == "forged" {
				if len(reporter.queue) != 0 {
					t.Fatal("forged signature created feedback")
				}
				return
			}
			if len(reporter.queue) != 1 {
				t.Fatalf("queued %d reports for one storm", len(reporter.queue))
			}
			observation := <-reporter.queue
			if !reporter.deliver(t.Context(), observation) {
				t.Fatal("system feedback not saved")
			}
			view, err := service.AdminDetail(t.Context(), repo.item.ID)
			if err != nil || view.UserID != 0 || view.GroupID != 7 || view.Source != feedbackdomain.SourceAutomatic ||
				view.Diagnostics["request_path"] != "/api/assets/928/stream" ||
				view.LogID != "0123456789abcdef0123456789abcdef" {
				t.Fatalf("admin feedback=%+v err=%v", view, err)
			}
			if strings.Contains(repo.item.DiagnosticsJSON, signature) || strings.Contains(repo.item.DiagnosticsJSON, "?") {
				t.Fatal("playback credentials leaked into feedback")
			}
		})
	}
}

type transientFeedbackCreator struct {
	calls atomic.Int32
	saved chan feedbackdomain.CreateInput
	panic bool
}

func (s *transientFeedbackCreator) CreateSystemAutomatic(
	_ context.Context, input feedbackdomain.CreateInput, _ time.Time,
) (*feedbackdomain.AutomaticCreateResult, error) {
	if s.panic {
		panic("writer failed")
	}
	if s.calls.Add(1) == 1 {
		return nil, errors.New("database unavailable")
	}
	s.saved <- input
	return &feedbackdomain.AutomaticCreateResult{Feedback: &feedbackdomain.UserView{ID: 1}}, nil
}

func TestBackendFeedbackRetriesAndStopsWithWorker(t *testing.T) {
	creator := &transientFeedbackCreator{saved: make(chan feedbackdomain.CreateInput, 1)}
	reporter := newAutomaticFeedbackReporter(creator)
	source := feedbackNotificationSource{reporter: reporter}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); reporter.run(ctx) }()
	defer func() { cancel(); <-done }()
	event := notificationdomain.Event{GroupID: 7, LogicalDate: "2026-10-05"}
	source.ReportNotificationFailure(event, "http_400")
	select {
	case input := <-creator.saved:
		if input.GroupID != 7 || input.Diagnostics["error_code"] != "notification_delivery_failed" {
			t.Fatalf("saved input=%+v", input)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transient failure was not retried")
	}
	source.ReportNotificationFailure(event, "http_400")
	if len(reporter.queue) != 0 {
		t.Fatal("successful report was not deduplicated")
	}
}

func TestBackendFeedbackBoundsMemoryAndContainsWriterPanic(t *testing.T) {
	reporter := newAutomaticFeedbackReporter(&transientFeedbackCreator{panic: true})
	now := time.Now()
	for index := range 1000 {
		reporter.observe(feedbackObservation{key: fmt.Sprint(index)}, 1, now)
	}
	if len(reporter.queue) != 32 || len(reporter.recent) != 256 {
		t.Fatalf("queue=%d keys=%d", len(reporter.queue), len(reporter.recent))
	}
	if reporter.deliver(t.Context(), <-reporter.queue) {
		t.Fatal("panic was treated as success")
	}
}

func TestBackendFeedbackThresholdWindowAndCooldown(t *testing.T) {
	reporter := newAutomaticFeedbackReporter(nil)
	observation := feedbackObservation{key: "asset"}
	now := time.Now()
	reporter.observe(observation, 3, now)
	reporter.observe(observation, 3, now.Add(2*time.Minute))
	reporter.observe(observation, 3, now.Add(2*time.Minute))
	if len(reporter.queue) != 0 {
		t.Fatal("old observation counted in current window")
	}
	reporter.observe(observation, 3, now.Add(2*time.Minute))
	if len(reporter.queue) != 1 {
		t.Fatal("continuous failures not reported")
	}
	<-reporter.queue
	reporter.recent["asset"] = feedbackOccurrence{reported: now}
	reporter.observe(observation, 3, now.Add(time.Minute))
	if len(reporter.queue) != 0 {
		t.Fatal("cooldown ignored")
	}
	for range 3 {
		reporter.observe(observation, 3, now.Add(6*time.Minute))
	}
	if len(reporter.queue) != 1 {
		t.Fatal("cooldown never expired")
	}
}
