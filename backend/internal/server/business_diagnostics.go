package server

import (
	"log/slog"
	"net/http"
)

// Diagnostics must not replace a business response, even if a log handler fails.
func logBusinessDiagnostic(r *http.Request, stage string, err error) {
	defer func() { _ = recover() }()
	slog.WarnContext(r.Context(), "business operation diagnostic",
		"stage", stage, "path", r.URL.Path, "error", err)
}
