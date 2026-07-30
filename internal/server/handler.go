package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"

	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
	"github.com/tigorlazuardi/herdr-web-tui/internal/push"
)

// New assembles the full request handler: the artifact-inject API routes
// (/send, /clientlog), the terminal websocket (/ws), plus the static/SPA
// file server, wrapped in the correlation and recover middleware (see doc.go
// for the required order). API routes are registered ahead of "/" on the mux
// so they take precedence over the SPA fallback.
//
// fsys is the frontend/dist tree (fs.Sub'd from dist.FS by the caller);
// logger is the process-wide slog.Logger built by internal/logger; herdr is
// the HerdrClient /send injects through (production: herdrclient.NewExecHerdrClient;
// tests: a fake); stagingDir is the flat directory /send saves uploads into
// (production: artifact.DefaultDir's result; tests: t.TempDir()).
func New(fsys fs.FS, logger *slog.Logger, herdr herdrclient.HerdrClient, stagingDir string, pushService ...*push.Service) http.Handler {
	mux := http.NewServeMux()
	if len(pushService) > 0 && pushService[0] != nil {
		mux.Handle("/api/push/", pushService[0].Handler())
	}
	mux.Handle("/send", newSendHandler(herdr, stagingDir, logger))
	mux.Handle("/clientlog", newClientlogHandler(logger))
	mux.Handle("/ws", newPTYHandler(logger))
	manifestHandler := newManifestHandler(os.Getenv("SERVER_NAME"), os.Getenv("APP_NAME"))
	mux.Handle("/manifest.webmanifest", manifestHandler)
	mux.Handle("/manifest.json", manifestHandler)
	mux.Handle("/", newStaticHandler(fsys))

	var h http.Handler = mux
	h = withRecover(logger)(h)
	h = withCorrelation(h)
	return h
}
