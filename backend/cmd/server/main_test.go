package main

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"strings"
	"testing"

	"agp/backend/internal/logctx"
)

func TestConfigureLoggingDoesNotReenterStandardLogger(t *testing.T) {
	var output bytes.Buffer
	previousSlog := slog.Default()
	previousWriter := log.Writer()
	defer func() {
		slog.SetDefault(previousSlog)
		log.SetOutput(previousWriter)
	}()

	configureLogging(&output)
	log.Print("startup")
	slog.InfoContext(
		logctx.WithLogID(context.Background(), "0123456789abcdef0123456789abcdef"),
		"request",
	)

	text := output.String()
	if !strings.Contains(text, "startup") {
		t.Fatalf("standard log missing from %q", text)
	}
	if !strings.Contains(text, "log_id=0123456789abcdef0123456789abcdef") {
		t.Fatalf("correlated log missing from %q", text)
	}
}
