package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
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
