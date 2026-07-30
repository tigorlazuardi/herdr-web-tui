package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
)

// noopHerdrClient satisfies herdrclient.HerdrClient for tests in this file
// that only exercise static/correlation/recover behaviour, never /send —
// every method fails loudly if accidentally called.
type noopHerdrClient struct{}

func (noopHerdrClient) FocusedPane(context.Context, string) (*herdrclient.PaneInfo, error) {
	panic("noopHerdrClient: unexpected call")
}

func (noopHerdrClient) PaneRun(context.Context, string, string, string) error {
	panic("noopHerdrClient: unexpected call")
}

func (noopHerdrClient) PaneRead(context.Context, string, string, int) (string, error) {
	panic("noopHerdrClient: unexpected call")
}

func testFS() fs.FS {
	return fstest.MapFS{
		"index.html":              {Data: []byte("<html>app</html>")},
		"assets/index-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc123.css": {Data: []byte("body{}")},
	}
}

func TestCorrelation_GeneratesIDWhenAbsent(t *testing.T) {
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	reqID := rec.Header().Get(correlation.HeaderRequestID)
	corrID := rec.Header().Get(correlation.HeaderCorrelationID)

	if reqID == "" {
		t.Fatal("expected X-Request-Id to be set")
	}
	if corrID != reqID {
		t.Fatalf("expected X-Correlation-Id (%q) to match X-Request-Id (%q)", corrID, reqID)
	}
}

func TestCorrelation_EchoesInboundID(t *testing.T) {
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlation.HeaderRequestID, "inbound-id-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(correlation.HeaderRequestID); got != "inbound-id-123" {
		t.Fatalf("expected inbound id to be echoed, got %q", got)
	}
	if got := rec.Header().Get(correlation.HeaderCorrelationID); got != "inbound-id-123" {
		t.Fatalf("expected inbound id echoed on X-Correlation-Id, got %q", got)
	}
}

func TestRecover_PanicReturns500WithoutCrashing(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("/panic", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	var h http.Handler = mux
	h = withRecover(log)(h)
	h = withCorrelation(h)

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	rec := httptest.NewRecorder()

	// The panic must not escape ServeHTTP and crash the test process.
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if rec.Header().Get(correlation.HeaderRequestID) == "" {
		t.Fatal("expected correlation id still set on a recovered-panic response")
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "panic recovered") {
		t.Fatalf("expected panic log line, got: %s", logged)
	}
	if !strings.Contains(logged, "goroutine") {
		t.Fatalf("expected runtime/debug.Stack() output in log, got: %s", logged)
	}
}

func TestStatic_ServesIndexAtRoot(t *testing.T) {
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("expected index.html content, got: %s", rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected no-cache on index.html, got %q", got)
	}
}

func TestStatic_SPAFallbackForUnknownPath(t *testing.T) {
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/anything/goes-here", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (SPA fallback), got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "app") {
		t.Fatalf("expected index.html content on fallback, got: %s", rec.Body.String())
	}
}

func TestStatic_HashedAssetGetsLongCache(t *testing.T) {
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/assets/index-abc123.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "max-age=31536000") || !strings.Contains(cc, "immutable") {
		t.Fatalf("expected long immutable cache on hashed asset, got %q", cc)
	}
}

func TestManifest_UsesServerNameAndIgnoresRemoteUser(t *testing.T) {
	get := func(serverName, user string) manifest {
		t.Helper()
		t.Setenv("SERVER_NAME", serverName)
		handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir())
		req := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
		req.Header.Set("Remote-User", user)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/manifest+json" {
			t.Fatalf("unexpected content type %q", got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("unexpected cache control %q", got)
		}
		if got := rec.Header().Get("Vary"); got != "" {
			t.Fatalf("unexpected vary header %q", got)
		}
		var m manifest
		if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
			t.Fatalf("decode manifest: %v", err)
		}
		return m
	}

	fallback := get("", "$tinyauth_remote_user")
	if fallback.ID != "" || fallback.Name != "Herdr Web TUI" || fallback.ShortName != "Herdr" {
		t.Fatalf("unexpected fallback: %+v", fallback)
	}

	server := get("sg-prod-1", "$tinyauth_remote_user")
	serverOtherUser := get("sg-prod-1", "alice")
	if server.Name != "sg-prod-1" || server.ShortName != "sg-prod-1" || server.ID == "" || server.ID != serverOtherUser.ID || server.Name != serverOtherUser.Name {
		t.Fatalf("unexpected server manifest: %+v", server)
	}
	if server.Display != "standalone" || len(server.Icons) != 2 || server.Icons[0].Src != "/icon-192.png" || server.Icons[1].Src != "/icon-512.png" {
		t.Fatalf("unexpected install metadata: %+v", server)
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
}
