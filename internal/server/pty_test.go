package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestWS_UpgradeHandshakeSucceeds exercises the httptest-reachable seam:
// the websocket upgrade handshake itself (Accept succeeds, /ws is routed
// ahead of the SPA fallback). It deliberately never lets herdr spawn — see
// TestWS_MalformedFirstFrameClosesWithoutSpawning — so it runs without a
// live Herdr binary, per the design doc's testing decision that the
// pty↔herdr path itself is integration-only.
func TestWS_UpgradeHandshakeSucceeds(t *testing.T) {
	srv := httptest.NewServer(New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir()))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.CloseNow()
	// Handshake succeeded; that's the seam under test. Close immediately
	// rather than proceeding into the resize/pty-spawn path.
}

// TestWS_MalformedFirstFrameClosesWithoutSpawning asserts the handler
// rejects a connection whose first frame isn't a resize frame — the
// invariant readInitialSize enforces before pty.Spawn is ever called, so
// this path is exercisable without herdr installed.
func TestWS_MalformedFirstFrameClosesWithoutSpawning(t *testing.T) {
	srv := httptest.NewServer(New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir()))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer conn.CloseNow()

	// Send an input frame first instead of the required initial resize.
	if err := conn.Write(ctx, websocket.MessageBinary, EncodeFrame(FrameInput, []byte("x"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The server should close the connection rather than hang waiting for
	// a resize frame that will never come correctly-typed.
	_, _, err = conn.Read(ctx)
	if err == nil {
		t.Fatal("expected connection to close after malformed first frame")
	}
}

// TestWS_ConnectLogsResolvedSession asserts the /ws handler extracts the
// session from the "session" query param, sanitizes it, and tags the "ws
// connect" log line with the resolved name — the correlation-field
// requirement from ticket 2 ("session name logged as a correlation
// field"). It never lets herdr spawn (no resize frame sent), so it runs
// without a live Herdr binary, matching this file's other handshake-only
// tests.
func TestWS_ConnectLogsResolvedSession(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		wantSession string
	}{
		{"valid session name passes through", "?session=work", "work"},
		{"missing session falls back to default", "", defaultSession},
		{"invalid session falls back to default", "?session=not+valid", defaultSession},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuf := &syncBuffer{}
			log := slog.New(slog.NewTextHandler(logBuf, nil))

			srv := httptest.NewServer(New(testFS(), log, noopHerdrClient{}, t.TempDir()))
			defer srv.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws" + tt.query
			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				t.Fatalf("dial /ws: %v", err)
			}
			defer conn.CloseNow()

			// The handler logs "ws connect" from the serve goroutine after
			// the handshake returns to us, so poll with a short bounded
			// retry instead of racing a single read of the buffer.
			want := "session=" + tt.wantSession
			var got string
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				got = logBuf.String()
				if strings.Contains(got, "ws connect") && strings.Contains(got, want) {
					return
				}
				time.Sleep(5 * time.Millisecond)
			}
			t.Fatalf("expected \"ws connect\" log line with %q, got: %s", want, got)
		})
	}
}

// syncBuffer is a mutex-guarded bytes.Buffer so tests can safely read log
// output that's written concurrently by net/http's serve goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestClampSize_NeverBelowMinimum(t *testing.T) {
	for _, cols := range []uint16{0, 1} {
		for _, rows := range []uint16{0, 1} {
			got := clampSize(cols, rows)
			if got.Cols < minCols || got.Rows < minRows {
				t.Fatalf("clampSize(%d,%d) = %+v below minimum", cols, rows, got)
			}
		}
	}
}

func TestLogDisconnect_DoesNotPanicOnNilError(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	logDisconnect(context.Background(), log, nil)
	if !strings.Contains(buf.String(), "ws disconnect") {
		t.Fatalf("expected disconnect log line, got: %s", buf.String())
	}
}
