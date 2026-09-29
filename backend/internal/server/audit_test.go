package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	auditdomain "agp/backend/internal/audit"
	learningdomain "agp/backend/internal/learning"
)

type serverAuditRepository struct {
	auditdomain.Repository
	logs       []auditdomain.Log
	contextErr error
	createErr  error
}

func (r *serverAuditRepository) Create(ctx context.Context, log auditdomain.Log) error {
	r.contextErr = ctx.Err()
	if r.createErr != nil {
		return r.createErr
	}
	r.logs = append(r.logs, log)
	return nil
}

func TestAuditChangesSkipsNoOpAndRecordsChanges(t *testing.T) {
	t.Parallel()

	repo := &serverAuditRepository{}
	application := &app{audits: auditdomain.NewService(repo)}
	state := &requestAuditState{}
	request := httptest.NewRequest(http.MethodPut, "/api/admin/learning-config", nil)
	request = request.WithContext(context.WithValue(request.Context(), requestAuditStateKey, state))

	application.auditChanges(
		7,
		9,
		"save_learning_config",
		"group_settings",
		7,
		map[string]any{"daily_enabled": true},
		map[string]any{"daily_enabled": true},
		request,
	)
	if len(repo.logs) != 0 || state.recorded || !state.handled {
		t.Fatalf("no-op audit = %d logs, recorded=%t, handled=%t", len(repo.logs), state.recorded, state.handled)
	}

	application.auditChanges(
		7,
		9,
		"save_learning_config",
		"group_settings",
		7,
		map[string]any{"daily_enabled": true},
		map[string]any{"daily_enabled": false},
		request,
	)
	if len(repo.logs) != 1 || !state.recorded {
		t.Fatalf("changed audit = %d logs, recorded=%t", len(repo.logs), state.recorded)
	}
	if repo.logs[0].BeforeJSON != `{"daily_enabled":true}` ||
		repo.logs[0].AfterJSON != `{"daily_enabled":false}` {
		t.Fatalf("audit changes = %s -> %s", repo.logs[0].BeforeJSON, repo.logs[0].AfterJSON)
	}
}

func TestAuditSurvivesCanceledRequestContext(t *testing.T) {
	t.Parallel()

	repo := &serverAuditRepository{}
	application := &app{audits: auditdomain.NewService(repo)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/api/checkins", nil).WithContext(ctx)

	application.audit(7, 9, "create_checkin", "checkin_records", 11, nil, map[string]any{"task_type": "daily_devotion"}, request)

	if len(repo.logs) != 1 {
		t.Fatalf("audit count = %d, want 1", len(repo.logs))
	}
	if repo.contextErr != nil {
		t.Fatalf("audit context error = %v, want nil", repo.contextErr)
	}
}

func TestAuditFailureLeavesRequestEligibleForFallback(t *testing.T) {
	t.Parallel()

	repo := &serverAuditRepository{createErr: errors.New("database unavailable")}
	application := &app{audits: auditdomain.NewService(repo)}
	state := &requestAuditState{}
	request := httptest.NewRequest(http.MethodPost, "/api/checkins", nil)
	request = request.WithContext(context.WithValue(request.Context(), requestAuditStateKey, state))

	application.audit(7, 9, "create_checkin", "checkin_records", 11, nil, map[string]any{
		"password":  "must-not-be-logged",
		"task_type": "daily_devotion",
	}, request)

	if state.recorded || state.handled {
		t.Fatalf("failed audit marked request recorded=%t handled=%t", state.recorded, state.handled)
	}
}

func TestRequiresBusinessAuditExcludesTechnicalMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		method  string
		pattern string
		want    bool
	}{
		{name: "learning config", method: http.MethodPut, pattern: "PUT /api/admin/learning-config", want: true},
		{name: "checkin", method: http.MethodPost, pattern: "POST /api/checkins", want: true},
		{name: "preview", method: http.MethodPost, pattern: "POST /api/admin/resource-imports/preview", want: false},
		{name: "switch session group", method: http.MethodPost, pattern: "POST /api/auth/switch-group", want: false},
		{name: "notification read marker", method: http.MethodPost, pattern: "POST /api/ministry-notifications/{id}/read", want: false},
		{name: "read request", method: http.MethodGet, pattern: "GET /api/today", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/", nil)
			request.Pattern = test.pattern
			if got := requiresBusinessAudit(request); got != test.want {
				t.Fatalf("requiresBusinessAudit() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestStudyWeekAuditIgnoresRebuiltTaskIDs(t *testing.T) {
	t.Parallel()

	before := studyWeekAuditValue(learningdomain.WeekInput{
		StartDate: "2026-09-21",
		EndDate:   "2026-09-27",
		Readings: []learningdomain.TaskBinding{{
			TaskID:  10,
			Title:   "读物",
			AssetID: 100,
		}},
	})
	after := studyWeekAuditValue(learningdomain.WeekInput{
		StartDate: "2026-09-21",
		EndDate:   "2026-09-27",
		Readings: []learningdomain.TaskBinding{{
			TaskID:  11,
			Title:   "读物",
			AssetID: 100,
		}},
	})
	_, _, changed, err := auditdomain.ChangedFields(before, after)
	if err != nil {
		t.Fatalf("ChangedFields() error = %v", err)
	}
	if changed {
		t.Fatal("rebuilt internal task IDs produced an audit change")
	}
}
