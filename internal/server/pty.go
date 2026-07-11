package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/go-faster/errors"
	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
	pty "github.com/tigorlazuardi/herdr-web-tui/internal/pty"
)

// defaultSession is the Herdr session name used by the render pty until
// ticket 2 adds URL-path-based session routing. Hardcoded rather than
// threaded through as a parameter because widening this is a small,
// contained change (one call site) and ticket 1 has no URL-routing
// requirement to satisfy yet — see tickets.md ticket 2.
const defaultSession = "default"

// minResize is the smallest terminal size accepted from the browser's
// initial resize frame; anything smaller almost certainly means the
// fit-addon hasn't measured the DOM yet (a 0x0 or 1x1 pty confuses herdr's
// layout code) rather than a genuinely tiny viewport.
const (
	minCols = 2
	minRows = 2
)

// newPTYHandler returns the /ws handler: it upgrades the request to a
// websocket, spawns a Herdr pty for defaultSession, and bridges bytes both
// directions until the connection drops. logger is the process-wide slog
// logger (already correlation-aware via internal/logger); every log line
// this handler emits carries both the originating HTTP request's req_id
// (from withCorrelation) and a fresh per-connection conn_id, since a ws
// connection outlives the single HTTP request that established it.
func newPTYHandler(logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connID := correlation.NewID()
		ctx := correlation.WithConnID(r.Context(), connID)

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			// Accept already wrote its own response on failure (e.g. not a
			// websocket upgrade request); nothing more to do.
			logger.ErrorContext(ctx, "ws accept failed", slog.String("error", err.Error()))
			return
		}
		// CloseNow (not Close) on unexpected paths: skip the close
		// handshake since something already went wrong and we don't want
		// to block teardown waiting for a client that may be gone.
		defer conn.CloseNow()

		logger.InfoContext(ctx, "ws connect", slog.String("session", defaultSession))
		serveTerminal(ctx, conn, logger)
	})
}

// serveTerminal owns one websocket connection's full lifecycle: read the
// browser's initial resize frame to size the pty correctly at spawn (a pty
// spawned at the wrong size briefly renders wrapped/garbled until the first
// resize lands), spawn the Herdr pty, then run the bidirectional bridge
// until ctx is cancelled or either side errors.
//
// ctx here is r.Context() (via the caller) plus the conn id — it is
// cancelled when the underlying TCP connection closes (browser navigated
// away, network dropped) or the process is shutting down, which is what
// gives the pty reader goroutine and the herdr process a hard stop: see
// internal/pty's doc.go for the exact teardown sequence this triggers.
func serveTerminal(ctx context.Context, conn *websocket.Conn, logger *slog.Logger) {
	size, err := readInitialSize(ctx, conn)
	if err != nil {
		logger.WarnContext(ctx, "ws closed before initial resize", slog.String("error", err.Error()))
		return
	}

	bridge, err := pty.Spawn(defaultSession, size)
	if err != nil {
		logger.ErrorContext(ctx, "pty spawn failed", slog.String("session", defaultSession), slog.String("error", err.Error()))
		sendError(ctx, conn, "herdr unreachable: "+err.Error())
		return
	}
	defer bridge.Close()
	logger.InfoContext(ctx, "pty spawn", slog.String("session", defaultSession), slog.Any("size", size))

	// runCtx is cancelled either by the caller's ctx (ws/http teardown) or
	// by this function returning (readLoop below returning on a read
	// error cancels it too), so whichever side errors first tears down
	// the other — Run and readLoop cannot outlive each other.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	outputErr := make(chan error, 1)
	go func() {
		outputErr <- bridge.Run(runCtx, func(b []byte) {
			// Write errors here (e.g. client gone) are surfaced by
			// readLoop's next read failing, so they're not separately
			// logged — avoids double-logging one disconnect.
			_ = conn.Write(runCtx, websocket.MessageBinary, EncodeFrame(FrameOutput, b))
		})
	}()

	readErr := readLoop(runCtx, conn, bridge, logger)
	cancel()
	<-outputErr

	logDisconnect(ctx, logger, readErr)
}

// readInitialSize blocks for the browser's first frame, which the frontend
// always sends as a FrameResize immediately after the socket opens (see
// frontend/src/lib/terminal.ts) so the pty can be spawned at the correct
// size instead of a default guess.
func readInitialSize(ctx context.Context, conn *websocket.Conn) (pty.Size, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, msg, err := conn.Read(ctx)
	if err != nil {
		return pty.Size{}, err
	}
	typ, data, err := DecodeFrame(msg)
	if err != nil {
		return pty.Size{}, err
	}
	if typ != FrameResize {
		return pty.Size{}, errUnexpectedFirstFrame(typ)
	}
	cols, rows, err := DecodeResize(data)
	if err != nil {
		return pty.Size{}, err
	}
	return clampSize(cols, rows), nil
}

// readLoop reads client → server frames (input, resize) until the
// connection errors or ctx cancels, writing input straight to the pty and
// applying resizes. Returns the read error (nil on a clean/expected close)
// so the caller can log connect-vs-error distinctly.
func readLoop(ctx context.Context, conn *websocket.Conn, bridge *pty.Bridge, logger *slog.Logger) error {
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		typ, data, err := DecodeFrame(msg)
		if err != nil {
			logger.WarnContext(ctx, "malformed ws frame", slog.String("error", err.Error()))
			continue
		}
		switch typ {
		case FrameInput:
			if _, err := bridge.Write(data); err != nil {
				logger.ErrorContext(ctx, "pty write failed", slog.String("error", err.Error()))
				return err
			}
		case FrameResize:
			cols, rows, err := DecodeResize(data)
			if err != nil {
				logger.WarnContext(ctx, "malformed resize frame", slog.String("error", err.Error()))
				continue
			}
			size := clampSize(cols, rows)
			if err := bridge.Resize(size); err != nil {
				logger.ErrorContext(ctx, "pty resize failed", slog.String("error", err.Error()))
				continue
			}
			logger.InfoContext(ctx, "pty resize", slog.Any("size", size))
		}
	}
}

// logDisconnect distinguishes a normal ws close (client navigated away,
// nil/expected close-status err) from an abnormal one, per the design
// doc's "ws lifecycle" telemetry requirement.
func logDisconnect(ctx context.Context, logger *slog.Logger, readErr error) {
	status := websocket.CloseStatus(readErr)
	if readErr == nil || status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
		logger.InfoContext(ctx, "ws disconnect", slog.String("reason", "closed"))
		return
	}
	logger.WarnContext(ctx, "ws disconnect", slog.String("reason", readErr.Error()))
}

// sendError best-effort writes a FrameError so the frontend can show the
// real failure (e.g. "herdr unreachable") instead of the connection just
// hanging or silently closing. Ignored if the write itself fails — the
// connection is already being torn down by the caller in that case.
func sendError(ctx context.Context, conn *websocket.Conn, message string) {
	_ = conn.Write(ctx, websocket.MessageBinary, EncodeFrame(FrameError, []byte(message)))
}

// errUnexpectedFirstFrame reports a first frame that wasn't the expected
// FrameResize (see readInitialSize).
func errUnexpectedFirstFrame(typ byte) error {
	return errors.Errorf("expected resize frame as first message, got type %q", typ)
}

// clampSize floors cols/rows to (minCols, minRows) so a not-yet-measured
// browser viewport (0x0) can never be handed to the pty, which would spawn
// herdr into a degenerate layout.
func clampSize(cols, rows uint16) pty.Size {
	if cols < minCols {
		cols = minCols
	}
	if rows < minRows {
		rows = minRows
	}
	return pty.Size{Cols: cols, Rows: rows}
}
