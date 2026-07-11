package server

import (
	"io/fs"
	"log/slog"
	"net/http"
)

// New assembles the full request handler: the static/SPA file server plus
// the correlation and recover middleware (see doc.go for the required
// order). Later tickets add API routes (/send, /clientlog, /ws) to a mux
// here, ahead of the static handler, so those routes take precedence over
// the SPA fallback.
//
// fsys is the frontend/dist tree (fs.Sub'd from dist.FS by the caller);
// logger is the process-wide slog.Logger built by internal/logger.
func New(fsys fs.FS, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", newStaticHandler(fsys))

	var h http.Handler = mux
	h = withRecover(logger)(h)
	h = withCorrelation(h)
	return h
}
