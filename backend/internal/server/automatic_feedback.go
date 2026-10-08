package server

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	feedbackdomain "agp/backend/internal/feedback"
	"agp/backend/internal/logctx"
	notificationdomain "agp/backend/internal/notification"
)

const automaticFeedbackWindow = 5 * time.Minute

type systemFeedbackCreator interface {
	CreateSystemAutomatic(context.Context, feedbackdomain.CreateInput, time.Time) (*feedbackdomain.AutomaticCreateResult, error)
}

type feedbackObservation struct {
	key   string
	input feedbackdomain.CreateInput
}

type feedbackOccurrence struct {
	first    time.Time
	reported time.Time
	count    int
	pending  bool
}

// Backend observations are bounded and best effort; business requests never wait
// for feedback settings, database writes or retries.
type automaticFeedbackReporter struct {
	service systemFeedbackCreator
	queue   chan feedbackObservation
	mu      sync.Mutex
	recent  map[string]feedbackOccurrence
}

func newAutomaticFeedbackReporter(service systemFeedbackCreator) *automaticFeedbackReporter {
	return &automaticFeedbackReporter{
		service: service,
		queue:   make(chan feedbackObservation, 32),
		recent:  make(map[string]feedbackOccurrence),
	}
}

func (p *automaticFeedbackReporter) observe(observation feedbackObservation, threshold int, now time.Time) {
	if p == nil {
		return
	}
	defer func() {
		if recover() != nil {
			slog.Error("automatic feedback observation panicked")
		}
	}()
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, occurrence := range p.recent {
		if !occurrence.pending && now.Sub(occurrence.first) > automaticFeedbackWindow &&
			now.Sub(occurrence.reported) > automaticFeedbackWindow {
			delete(p.recent, key)
		}
	}
	occurrence, exists := p.recent[observation.key]
	if !exists && len(p.recent) >= 256 {
		return
	}
	if occurrence.pending || (!occurrence.reported.IsZero() && now.Sub(occurrence.reported) < automaticFeedbackWindow) {
		return
	}
	if occurrence.first.IsZero() || now.Sub(occurrence.first) > time.Minute {
		occurrence.first, occurrence.count = now, 0
	}
	occurrence.count++
	if occurrence.count >= threshold {
		select {
		case p.queue <- observation:
			occurrence.pending = true
		default:
			// Keep the counter so a later occurrence can try again.
			occurrence.count = threshold
		}
	}
	p.recent[observation.key] = occurrence
}

func (p *automaticFeedbackReporter) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case observation := <-p.queue:
			accepted := p.deliver(ctx, observation)
			p.mu.Lock()
			occurrence := p.recent[observation.key]
			occurrence.pending = false
			if accepted {
				occurrence.reported = time.Now()
			}
			p.recent[observation.key] = occurrence
			p.mu.Unlock()
		}
	}
}

func (p *automaticFeedbackReporter) deliver(ctx context.Context, observation feedbackObservation) (accepted bool) {
	defer func() {
		if recover() != nil {
			slog.ErrorContext(ctx, "automatic feedback writer panicked")
		}
	}()
	for attempt := 0; attempt < 3 && ctx.Err() == nil; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return false
			case <-timer.C:
			}
		}
		result, err := func() (*feedbackdomain.AutomaticCreateResult, error) {
			writeContext, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return p.service.CreateSystemAutomatic(writeContext, observation.input, time.Now())
		}()
		if err == nil && result != nil {
			// Explicitly disabled/muted reports also cool down to avoid querying
			// settings on every request in a playback retry storm.
			return result.Feedback != nil || result.Reason == "disabled" || result.Reason == "muted"
		}
		slog.WarnContext(logctx.WithLogID(ctx, observation.input.LogID),
			"automatic backend feedback failed", "attempt", attempt+1, "error", err)
	}
	return false
}

type feedbackNotificationSource struct {
	notificationdomain.SnapshotSource
	reporter *automaticFeedbackReporter
}

func (s feedbackNotificationSource) ReportNotificationFailure(event notificationdomain.Event, code string) {
	message := "系统自动上报：学习进度通知最终发送失败"
	if code == "potato_3023" {
		message = "系统自动上报：群内禁止机器人发送学习进度通知，需要恢复通知时请解除禁言并重新绑定学习小组"
	}
	s.reporter.observe(feedbackObservation{
		key: fmt.Sprintf("notification:%d:%s", event.GroupID, code),
		input: feedbackdomain.CreateInput{
			GroupID: event.GroupID,
			LogID:   event.LogID,
			Message: message,
			Diagnostics: map[string]string{
				"environment": "server", "action_context": "notification_delivery",
				"error_code": "notification_delivery_failed", "error_message": code,
				"business_action": "发送学习进度通知", "logical_date": event.LogicalDate,
			},
		},
	}, 1, time.Now())
}

func (a *app) reportPlaybackRejection(ctx context.Context, groupID, assetID uint64, reason string) {
	a.automaticFeedback.observe(feedbackObservation{
		key: fmt.Sprintf("playback:%d:%d", groupID, assetID),
		input: feedbackdomain.CreateInput{
			GroupID: groupID,
			LogID:   logctx.LogID(ctx),
			Message: "系统自动上报：音视频播放链接持续被拒绝",
			Diagnostics: map[string]string{
				"environment": "server", "action_context": "asset_playback",
				"error_code": "invalid_playback_url", "error_message": reason,
				"business_action": "播放音视频", "request_method": "GET",
				"request_path": fmt.Sprintf("/api/assets/%d/stream", assetID), "http_status": "403",
			},
		},
	}, 3, time.Now())
}
