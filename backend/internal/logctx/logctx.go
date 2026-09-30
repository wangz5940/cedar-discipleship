package logctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
)

const Header = "X-Log-ID"

type contextKey struct{}

func New() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func Valid(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func WithLogID(ctx context.Context, logID string) context.Context {
	if !Valid(logID) {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, logID)
}

func LogID(ctx context.Context) string {
	logID, _ := ctx.Value(contextKey{}).(string)
	return logID
}

type handler struct {
	next slog.Handler
}

func NewHandler(next slog.Handler) slog.Handler {
	return handler{next: next}
}

func (h handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h handler) Handle(ctx context.Context, record slog.Record) error {
	if logID := LogID(ctx); logID != "" {
		record.AddAttrs(slog.String("log_id", logID))
	}
	return h.next.Handle(ctx, record)
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handler{next: h.next.WithAttrs(attrs)}
}

func (h handler) WithGroup(name string) slog.Handler {
	return handler{next: h.next.WithGroup(name)}
}
