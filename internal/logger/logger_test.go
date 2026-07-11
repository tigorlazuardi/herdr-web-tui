package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
)

func TestNewSlogHandler(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		isTTY    bool
		wantText bool
	}{
		{"explicit json", "json", false, false},
		{"explicit json overrides tty", "json", true, false},
		{"explicit text", "text", false, true},
		{"explicit text overrides non-tty", "text", false, true},
		{"auto tty picks text", "", true, true},
		{"auto non-tty picks json", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewSlogHandler(tt.format, tt.isTTY)
			_, isText := h.(*slog.TextHandler)
			_, isJSON := h.(*slog.JSONHandler)

			if tt.wantText && !isText {
				t.Fatalf("format=%q isTTY=%v: want *slog.TextHandler, got %T", tt.format, tt.isTTY, h)
			}
			if !tt.wantText && !isJSON {
				t.Fatalf("format=%q isTTY=%v: want *slog.JSONHandler, got %T", tt.format, tt.isTTY, h)
			}
		})
	}
}

func TestNew_TagsLogLinesWithCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(ctxHandler{slog.NewJSONHandler(&buf, nil)})

	ctx := correlation.WithRequestID(context.Background(), "req-abc")
	l.InfoContext(ctx, "hello")

	if !strings.Contains(buf.String(), `"req_id":"req-abc"`) {
		t.Fatalf("expected req_id in log line, got: %s", buf.String())
	}
}
