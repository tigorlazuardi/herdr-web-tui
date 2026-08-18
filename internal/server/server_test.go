package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func (noopHerdrClient) PaneSendInput(context.Context, string, string, string, string) error {
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
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})

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
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})

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
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})

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
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})

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
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})

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

func TestIconOverridesFromEnv_ServesValidatedPNGs(t *testing.T) {
	for _, key := range []string{"FAVICON_PATH", "PWA_ICON_192_PATH", "PWA_ICON_512_PATH"} {
		t.Setenv(key, "")
	}

	icons := []struct {
		env           string
		path          string
		width, height int
		data          []byte
	}{
		{env: "FAVICON_PATH", path: "/favicon.png", width: 32, height: 32},
		{env: "PWA_ICON_192_PATH", path: "/icon-192.png", width: 192, height: 192},
		{env: "PWA_ICON_512_PATH", path: "/icon-512.png", width: 512, height: 512},
	}
	for i := range icons {
		filename, data := writeTestPNG(t, icons[i].width, icons[i].height)
		t.Setenv(icons[i].env, filename)
		icons[i].data = data
	}

	overrides, err := IconOverridesFromEnv()
	if err != nil {
		t.Fatalf("load icon overrides: %v", err)
	}
	handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), overrides)
	for _, icon := range icons {
		req := httptest.NewRequest(http.MethodGet, icon.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), icon.data) {
			t.Fatalf("%s did not serve configured PNG", icon.path)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/png" {
			t.Fatalf("%s content type = %q", icon.path, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s cache control = %q", icon.path, got)
		}
	}
}

func TestIconOverridesFromEnv_RejectsInvalidConfig(t *testing.T) {
	wrongSize, _ := writeTestPNG(t, 16, 16)
	notPNG := filepath.Join(t.TempDir(), "not-png.png")
	if err := os.WriteFile(notPNG, []byte("not png"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "relative path", path: "icon.png"},
		{name: "missing file", path: filepath.Join(t.TempDir(), "missing.png")},
		{name: "non PNG", path: notPNG},
		{name: "wrong dimensions", path: wrongSize},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, key := range []string{"FAVICON_PATH", "PWA_ICON_192_PATH", "PWA_ICON_512_PATH"} {
				t.Setenv(key, "")
			}
			t.Setenv("FAVICON_PATH", test.path)
			if _, err := IconOverridesFromEnv(); err == nil {
				t.Fatal("expected invalid icon override to fail")
			}
		})
	}
}

func writeTestPNG(t *testing.T, width, height int) (string, []byte) {
	t.Helper()
	filename := filepath.Join(t.TempDir(), fmt.Sprintf("%dx%d.png", width, height))
	file, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{R: 0x42, G: 0xa5, B: 0xf5, A: 0xff}), image.Point{}, draw.Src)
	if err := png.Encode(file, pixels); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return filename, data
}

func TestManifestUsesConfiguredNames(t *testing.T) {
	get := func(serverName, appName, requestPath string) manifest {
		t.Helper()
		t.Setenv("SERVER_NAME", serverName)
		t.Setenv("APP_NAME", appName)
		handler := New(testFS(), silentLogger(), noopHerdrClient{}, t.TempDir(), IconOverrides{})
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
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

	fallback := get("", "", "/manifest.webmanifest")
	if fallback.ID != "" || fallback.Name != "Herdr Web TUI" || fallback.ShortName != "Herdr" {
		t.Fatalf("unexpected fallback: %+v", fallback)
	}

	server := get(" sg-prod-1 ", "", "/manifest.webmanifest")
	custom := get(" sg-prod-1 ", " Production Shell ", "/manifest.webmanifest")
	legacyPath := get(" sg-prod-1 ", " Production Shell ", "/manifest.json")
	expectedID := fmt.Sprintf("/pwa/%x", sha256.Sum256([]byte(" sg-prod-1 ")))
	if server.Name != " sg-prod-1 " || server.ShortName != " sg-prod-1 " || server.ID != expectedID {
		t.Fatalf("unexpected default server manifest: %+v", server)
	}
	if custom.Name != " Production Shell " || custom.ShortName != " Production Shell " || custom.ID != expectedID || legacyPath.Name != custom.Name || legacyPath.ID != custom.ID {
		t.Fatalf("unexpected custom app manifest: %+v", custom)
	}
	if server.Display != "standalone" || len(server.Icons) != 2 || server.Icons[0].Src != "/icon-192.png" || server.Icons[1].Src != "/icon-512.png" {
		t.Fatalf("unexpected install metadata: %+v", server)
	}
}

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(new(bytes.Buffer), nil))
}
