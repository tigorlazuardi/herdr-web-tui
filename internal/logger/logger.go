package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
)

// NewSlogHandler picks the slog.Handler for a format selection. Pure
// function (no globals, no I/O side effects beyond the returned handler's
// eventual writes) so format selection is unit-testable by asserting on the
// concrete type returned.
//
// format is "json", "text", or "" (auto: text when isTTY, json otherwise —
// JSON is the greppable/aggregator-friendly default for piped/systemd
// output; text is nicer for a human staring at a dev terminal). AddSource is
// always on (see package doc for why).
func NewSlogHandler(format string, isTTY bool) slog.Handler {
	opts := &slog.HandlerOptions{AddSource: true}
	switch format {
	case "text":
		return slog.NewTextHandler(os.Stdout, opts)
	case "json":
		return slog.NewJSONHandler(os.Stdout, opts)
	default:
		if isTTY {
			return slog.NewTextHandler(os.Stdout, opts)
		}
		return slog.NewJSONHandler(os.Stdout, opts)
	}
}

// New builds the process-wide logger: the handler from NewSlogHandler,
// wrapped so every *Context call (InfoContext, ErrorContext, ...) picks up
// the request's correlation id from ctx automatically. Handlers built
// without this wrapper (slog.New(NewSlogHandler(...))) would silently drop
// the id on every log line — this is the one place that wiring happens, so
// callers never have to remember correlation.Attr(ctx) by hand.
func New(format string, isTTY bool) *slog.Logger {
	return slog.New(ctxHandler{NewSlogHandler(format, isTTY)})
}

// ctxHandler decorates a slog.Handler so every record handled through it
// gains a req_id attribute pulled from the ctx passed to Handle. slog's
// *Context logging methods (InfoContext etc.) thread ctx through to
// Handle, which is what makes this possible without changing every log call
// site to pass the id explicitly.
type ctxHandler struct {
	slog.Handler
}

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := correlation.RequestID(ctx); id != "" {
		r.AddAttrs(correlation.Attr(ctx))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}
