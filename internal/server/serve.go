package server

import (
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
