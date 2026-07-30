package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
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

func newManifestHandler(serverName string) http.Handler {
	serverName = strings.TrimSpace(serverName)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := strings.TrimSpace(r.Header.Get("Remote-User"))
		labels := []string{"Herdr"}
		if serverName != "" {
			labels = append(labels, serverName)
		}
		if user != "" {
			labels = append(labels, user)
		}
		name := strings.Join(labels, " — ")
		if serverName == "" && user == "" {
			name = "Herdr Web TUI"
		}
		shortName := "Herdr"
		if serverName != "" {
			shortName += " " + serverName
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
		if serverName != "" || user != "" {
			m.ID = fmt.Sprintf("/pwa/%x", sha256.Sum256([]byte(serverName+"\x00"+user)))
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Vary", "Remote-User")
		_ = json.NewEncoder(w).Encode(m)
	})
}
