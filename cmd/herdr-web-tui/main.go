// Command herdr-web-tui runs the standalone web frontend for a running
// Herdr server: it serves the embedded browser client and (in later
// tickets) bridges it to Herdr over pty/websocket and a file-inject
// endpoint. See docs/design/2026-07-11-herdr-web-tui-spec.mdx for the full
// design.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-faster/errors"
	dist "github.com/tigorlazuardi/herdr-web-tui"
	"github.com/tigorlazuardi/herdr-web-tui/internal/artifact"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
	"github.com/tigorlazuardi/herdr-web-tui/internal/logger"
	"github.com/tigorlazuardi/herdr-web-tui/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", envOr("ADDR", ":8080"), "address to listen on (env ADDR)")
	logFormat := flag.String("log-format", envOr("LOG_FORMAT", ""), "log format: json, text, or empty for TTY auto-detect (env LOG_FORMAT)")
	tmpPrefix := flag.String("tmp-prefix", envOr("TMP_PREFIX", "herdr-web-tui"), "prefix for the /tmp/<prefix>-<server-uid> artifact staging dir (env TMP_PREFIX)")
	flag.Parse()

	isTTY := isTerminal(os.Stdout)
	log := logger.New(*logFormat, isTTY)

	distFS, err := fs.Sub(dist.FS, "frontend/dist")
	if err != nil {
		return errors.Wrap(err, "mount embedded frontend")
	}
	stagingDir, err := artifact.DefaultDir(*tmpPrefix)
	if err != nil {
		return errors.Wrap(err, "resolve artifact staging dir")
	}
	herdr := herdrclient.NewExecHerdrClient(log)
	handler := server.New(distFS, log, herdr, stagingDir)

	srv := &http.Server{
		Addr:    *addr,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", *addr))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.Wrap(err, "serve")
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return errors.Wrap(err, "shutdown")
		}
	}
	return nil
}

// envOr returns the environment variable's value, or def if unset.
func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// isTerminal reports whether f is an interactive TTY, used for log-format
// auto-detection (text for a human dev session, JSON otherwise). Checking
// the ModeCharDevice bit avoids pulling in golang.org/x/term for one bool.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
