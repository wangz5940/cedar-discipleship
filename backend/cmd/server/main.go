package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"agp/backend/internal/app"
	"agp/backend/internal/logctx"
)

func main() {
	writer := io.Writer(os.Stderr)
	if path := strings.TrimSpace(os.Getenv("AGP_LOG_FILE")); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatalf("create log directory: %v", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open log file: %v", err)
		}
		defer file.Close()

		writer = io.MultiWriter(os.Stderr, file)
	}
	configureLogging(writer)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func configureLogging(writer io.Writer) {
	slog.SetDefault(slog.New(logctx.NewHandler(slog.NewTextHandler(writer, nil))))
	log.SetOutput(writer)
}
