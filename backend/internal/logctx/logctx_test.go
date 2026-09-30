package logctx

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestNewReturnsCanonicalRandomID(t *testing.T) {
	t.Parallel()

	first, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New()
	if err != nil {
		t.Fatalf("New() second error = %v", err)
	}
	if !Valid(first) {
		t.Fatalf("New() = %q, want 32 lowercase hexadecimal characters", first)
	}
	if first == second {
		t.Fatalf("New() returned duplicate IDs %q", first)
	}
}

func TestValid(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid", value: "0123456789abcdef0123456789abcdef", want: true},
		{name: "empty", value: "", want: false},
		{name: "short", value: "0123456789abcdef", want: false},
		{name: "uppercase", value: "0123456789ABCDEF0123456789ABCDEF", want: false},
		{name: "non hex", value: "0123456789abcdef0123456789abcdeg", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := Valid(test.value); got != test.want {
				t.Fatalf("Valid(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestContextAndHandlerAddLogID(t *testing.T) {
	t.Parallel()

	const id = "0123456789abcdef0123456789abcdef"
	ctx := WithLogID(context.Background(), id)
	if got := LogID(ctx); got != id {
		t.Fatalf("LogID() = %q, want %q", got, id)
	}

	var output bytes.Buffer
	logger := slog.New(NewHandler(slog.NewTextHandler(&output, nil)))
	logger.InfoContext(ctx, "handled", "status", 200)

	line := output.String()
	for _, want := range []string{`msg=handled`, `status=200`, `log_id=` + id} {
		if !strings.Contains(line, want) {
			t.Fatalf("log output %q does not contain %q", line, want)
		}
	}
}

func TestWithLogIDRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	ctx := WithLogID(context.Background(), "not-valid")
	if got := LogID(ctx); got != "" {
		t.Fatalf("LogID() = %q, want empty", got)
	}
}
