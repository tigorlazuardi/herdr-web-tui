// Package dist embeds the built frontend (frontend/dist, produced by `npm
// run build` inside frontend/) into the Go binary so the whole app ships as
// one static file with nothing else to deploy alongside it.
//
// Go's //go:embed can only reach files under the directory tree of the file
// that declares it, so this file has to live at the module root (a sibling
// of frontend/) rather than inside cmd/ or internal/. cmd/herdr-web-tui's
// main package imports this package and fs.Sub()s FS down to the dist root
// before handing it to internal/server, so internal/server never needs to
// know the embed's on-disk path.
//
// frontend/dist must exist (i.e. the frontend must already be built) for
// `go build`/`go test` to succeed against this package — see the Makefile,
// which always builds the frontend before the Go binary.
package dist

import "embed"

// FS holds the embedded frontend build. Callers must fs.Sub(FS,
// "frontend/dist") before serving it, since embed keeps the declared path
// prefix.
//
//go:embed all:frontend/dist
var FS embed.FS
