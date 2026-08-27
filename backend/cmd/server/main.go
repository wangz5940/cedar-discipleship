package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"agp/backend/internal/app"
)

func main() {
	if path := strings.TrimSpace(os.Getenv("AGP_LOG_FILE")); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatalf("create log directory: %v", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			log.Fatalf("open log file: %v", err)
		}
		defer file.Close()

		writer := io.MultiWriter(os.Stderr, file)
		log.SetOutput(writer)
		slog.SetDefault(slog.New(slog.NewTextHandler(writer, nil)))
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
