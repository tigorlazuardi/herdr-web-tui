package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-faster/errors"
)

// newStaticHandler serves the embedded frontend build with SPA-fallback
// routing: any request whose path does not resolve to a real file in fsys
// is rewritten to "/" (index.html) instead of 404ing. This is what lets
// arbitrary paths like /work (a future session name, ticket 2) return the
// app shell rather than a static-file 404 — the frontend, not this handler,
// interprets the path.
//
// fsys is expected to be the frontend/dist tree (see dist.go), already
// fs.Sub'd down from the embed.FS root by the caller. No Vite manifest is
// read or needed: Vite already rewrote index.html with the hashed asset
// URLs at build time, so serving dist/ as-is is sufficient (see design doc,
// "Build & serve").
func newStaticHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCacheHeaders(w, r.URL.Path)

		if _, err := fs.Stat(fsys, cleanFSPath(r.URL.Path)); err != nil {
			// Not a real file: SPA fallback to index.html so client-side
			// routing (and, later, session-name paths) works.
			r = cloneWithPath(r, "/")
			setCacheHeaders(w, "/")
		}
		fileServer.ServeHTTP(w, r)
	})
}

// cleanFSPath converts a URL path to the fs.FS-relative form http.FileServer
// / fs.Stat expect: no leading slash, "index.html" for "/".
func cleanFSPath(urlPath string) string {
	p := strings.TrimPrefix(path.Clean(urlPath), "/")
	if p == "" || p == "." {
		p = "index.html"
	}
	return p
}

// cloneWithPath returns a shallow copy of r with URL.Path replaced. Used to
// rewrite the request for the SPA fallback without mutating the original
// request the caller (a future logging/metrics middleware) may still expect
// to reflect the originally-requested path.
func cloneWithPath(r *http.Request, p string) *http.Request {
	r2 := r.Clone(r.Context())
	r2.URL.Path = p
	return r2
}

// setCacheHeaders applies the caching split described in the design doc:
// hashed, content-addressed assets (anything under /assets/, Vite's output
// convention) get a long, immutable cache; index.html (and, via fallback,
// every SPA route) gets no-cache so a new deploy's hashed references are
// picked up on next load instead of being served stale from a browser
// cache.
func setCacheHeaders(w http.ResponseWriter, urlPath string) {
	if strings.HasPrefix(urlPath, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
}

const maxIconOverrideBytes = 1 << 20

// IconOverrides contains validated operator-provided PNG assets. Empty fields
// retain the corresponding frontend-bundled default.
type IconOverrides struct {
	favicon []byte
	icon192 []byte
	icon512 []byte
}

// IconOverridesFromEnv loads optional icon overrides from absolute paths in
// FAVICON_PATH, PWA_ICON_192_PATH, and PWA_ICON_512_PATH.
func IconOverridesFromEnv() (IconOverrides, error) {
	var overrides IconOverrides
	for _, icon := range []struct {
		env           string
		width, height int
		destination   *[]byte
	}{
		{"FAVICON_PATH", 32, 32, &overrides.favicon},
		{"PWA_ICON_192_PATH", 192, 192, &overrides.icon192},
		{"PWA_ICON_512_PATH", 512, 512, &overrides.icon512},
	} {
		data, err := loadPNGOverride(icon.env, icon.width, icon.height)
		if err != nil {
			return IconOverrides{}, err
		}
		*icon.destination = data
	}
	return overrides, nil
}

func loadPNGOverride(env string, width, height int) ([]byte, error) {
	filename := os.Getenv(env)
	if filename == "" {
		return nil, nil
	}
	if !filepath.IsAbs(filename) {
		return nil, errors.Errorf("%s must be an absolute path", env)
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Wrapf(err, "open %s", env)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, errors.Wrapf(err, "stat %s", env)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Errorf("%s must reference a regular file", env)
	}

	data, err := io.ReadAll(io.LimitReader(file, maxIconOverrideBytes+1))
	if err != nil {
		return nil, errors.Wrapf(err, "read %s", env)
	}
	if len(data) > maxIconOverrideBytes {
		return nil, errors.Errorf("%s exceeds %d bytes", env, maxIconOverrideBytes)
	}
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrapf(err, "decode %s as PNG", env)
	}
	bounds := image.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		return nil, errors.Errorf("%s must be %dx%d PNG, got %dx%d", env, width, height, bounds.Dx(), bounds.Dy())
	}
	return data, nil
}

type manifest struct {
	ID              string         `json:"id,omitempty"`
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	Description     string         `json:"description"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Display         string         `json:"display"`
	BackgroundColor string         `json:"background_color"`
	ThemeColor      string         `json:"theme_color"`
	Icons           []manifestIcon `json:"icons"`
}

type manifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose"`
}

func newManifestHandler(serverName, appName string) http.Handler {
	if appName == "" {
		appName = serverName
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		name := "Herdr Web TUI"
		shortName := "Herdr"
		if appName != "" {
			name = appName
			shortName = appName
		}

		m := manifest{
			Name:            name,
			ShortName:       shortName,
			Description:     "Live Herdr TUI in the browser",
			StartURL:        "/",
			Scope:           "/",
			Display:         "standalone",
			BackgroundColor: "#000000",
			ThemeColor:      "#000000",
			Icons: []manifestIcon{
				{Src: "/icon-192.png", Sizes: "192x192", Type: "image/png", Purpose: "any"},
				{Src: "/icon-512.png", Sizes: "512x512", Type: "image/png", Purpose: "any"},
			},
		}
		if serverName != "" {
			m.ID = fmt.Sprintf("/pwa/%x", sha256.Sum256([]byte(serverName)))
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "application/manifest+json")
		_ = json.NewEncoder(w).Encode(m)
	})
}
