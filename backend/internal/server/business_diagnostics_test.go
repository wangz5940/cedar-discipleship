package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agp/backend/internal/logctx"
	"agp/backend/internal/studymemory"
)

type failingStudyMemory struct{ studyMemoryTestRepository }

func (*failingStudyMemory) Load(context.Context, uint64) (studymemory.Snapshot, error) {
	return studymemory.Snapshot{}, errors.New("storage unavailable")
}

func (*failingStudyMemory) SaveProgress(context.Context, uint64, string, studymemory.Progress) error {
	return errors.New("storage unavailable")
}

func (*failingStudyMemory) SetFavorite(context.Context, uint64, string, *studymemory.Favorite) error {
	return errors.New("storage unavailable")
}

type panickingDiagnosticHandler struct{ slog.Handler }

func (panickingDiagnosticHandler) Handle(context.Context, slog.Record) error {
	panic("logging unavailable")
}

func useDiagnosticTestLogger(t *testing.T, handler slog.Handler) {
	t.Helper()
	previous, writer, flags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() {
		slog.SetDefault(previous)
		log.SetOutput(writer)
		log.SetFlags(flags)
	})
}

func TestStudyFailureDiagnosticsPreserveResponse(t *testing.T) {
	a := &app{studyMemory: &failingStudyMemory{}}
	for _, test := range []struct {
		stage   string
		body    string
		handler http.HandlerFunc
	}{
		{"study_memory_load", "", a.handleStudyMemory},
		{"study_progress_save", `{"key":"asset:9","time":12,"duration":60}`, a.handleStudyProgress},
		{"study_favorite_save", `{"key":"asset:9","favorite":false}`, a.handleStudyFavorite},
	} {
		for _, brokenLogger := range []bool{false, true} {
			t.Run(test.stage+map[bool]string{true: "/logging_failure"}[brokenLogger], func(t *testing.T) {
				var output bytes.Buffer
				var handler slog.Handler = logctx.NewHandler(slog.NewTextHandler(&output, nil))
				if brokenLogger {
					handler = panickingDiagnosticHandler{handler}
				}
				useDiagnosticTestLogger(t, handler)
				request := studyRequest(11, 1, test.body)
				request = request.WithContext(logctx.WithLogID(request.Context(), "0123456789abcdef0123456789abcdef"))
				response := httptest.NewRecorder()
				test.handler(response, request)
				if response.Code != http.StatusInternalServerError ||
					!strings.Contains(response.Body.String(), `"error":"study_memory_failed"`) {
					t.Fatalf("original response changed: %d %s", response.Code, response.Body)
				}
				if !brokenLogger {
					for _, want := range []string{test.stage, "storage unavailable", "0123456789abcdef0123456789abcdef"} {
						if !strings.Contains(output.String(), want) {
							t.Fatalf("missing %q in diagnostic: %s", want, &output)
						}
					}
				}
			})
		}
	}
}
