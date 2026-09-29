package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ministrydomain "agp/backend/internal/ministry"
)

func TestWriteMinistryErrorMapsDecisionConflicts(t *testing.T) {
	for _, err := range []error{
		ministrydomain.ErrSubmissionRoundConflict,
		ministrydomain.ErrRequestApplicantNotMember,
	} {
		t.Run(err.Error(), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", nil)

			(&app{}).writeMinistryError(recorder, request, err)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
			}
			if !strings.Contains(recorder.Body.String(), err.Error()) {
				t.Fatalf("response does not include %q: %s", err, recorder.Body.String())
			}
		})
	}
}
