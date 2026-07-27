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
	"github.com/tigorlazuardi/herdr-web-tui/internal/push"
	"github.com/tigorlazuardi/herdr-web-tui/internal/server"
	"github.com/tigorlazuardi/herdr-web-tui/internal/telemetry"
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
	baseLog := logger.New(*logFormat, isTTY)
	log := slog.New(telemetry.CorrelatingHandler{Handler: baseLog.Handler()})

	distFS, err := fs.Sub(dist.FS, "frontend/dist")
	if err != nil {
		return errors.Wrap(err, "mount embedded frontend")
	}
	stagingDir, err := artifact.DefaultDir(*tmpPrefix)
	if err != nil {
		return errors.Wrap(err, "resolve artifact staging dir")
	}
	herdr := herdrclient.NewExecHerdrClient(log)
	pushConfig, pushStore, err := loadPush(log)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry, err := telemetry.Setup(ctx, log)
	if err != nil {
		return errors.Wrap(err, "initialize OpenTelemetry")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTelemetry(shutdownCtx)
	}()
	pushService := push.NewService(pushConfig, pushStore, log)
	pushService.SetSocketPath(os.Getenv("HERDR_SOCKET_PATH"))
	log.Info("Web Push configuration ready", "enabled", pushConfig.Enabled(), "vapid.private_key", "<redacted>")
	if pushConfig.Enabled() {
		go func() {
			if err := pushService.RunEvents(ctx, os.Getenv("HERDR_SOCKET_PATH")); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("Web Push event subscriber stopped", "error", err)
			}
		}()
	}
	handler := server.New(distFS, log, herdr, stagingDir, pushService)

	srv := &http.Server{
		Addr:    *addr,
		Handler: handler,
	}

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

// loadPush validates configuration and opens persistence before network readiness, logging redacted structured failure details.
func loadPush(log *slog.Logger) (push.Config, *push.Store, error) {
	config, err := push.ConfigFromEnv()
	if err != nil {
		log.Error("Web Push configuration failed", "error.kind", "invalid_config", "vapid.public_key", "<redacted>", "vapid.private_key", "<redacted>", "vapid.subject", "<redacted>")
		return push.Config{}, nil, errors.Wrap(err, "Web Push configuration")
	}
	store, err := push.OpenStore(config.StorePath)
	if err != nil {
		log.Error("Web Push configuration failed", "error.kind", "store_open", "web_push.store_path", "<redacted>")
		return push.Config{}, nil, errors.Wrap(err, "open Web Push store")
	}
	return config, store, nil
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
