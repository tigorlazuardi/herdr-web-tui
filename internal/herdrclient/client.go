package herdrclient

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
)

// ErrUnreachable wraps every error produced when the herdr binary itself
// could not be run at all (not found on PATH, or the process could not be
// spawned) — as opposed to herdr running and reporting a command-level
// failure (bad session, pane not found, ...). Callers use errors.Is(err,
// ErrUnreachable) to distinguish "herdr is down" (5xx, degrade gracefully)
// from "herdr ran and rejected this request" (usually 4xx, a bad session or
// pane).
var ErrUnreachable = errors.New("herdr not reachable")

// PaneInfo is the subset of `herdr pane current`'s JSON output this service
// needs: which pane is focused. Herdr's focused pane is server-wide (shared
// across every attached client, browser or SSH) — see the herdr skill — so
// this is "whatever the user is looking at right now", not a per-client
// concept.
type PaneInfo struct {
	PaneID string
}

// HerdrClient wraps every way this service talks to a running Herdr
// session. It exists so the artifact-inject flow (ticket 5) can be unit
// tested with a fake instead of a live Herdr server — see package doc.
//
// Every method takes the session name as an explicit argument rather than
// binding it at construction time, because a single process may serve
// several concurrent sessions (ticket 2) and HerdrClient itself is
// stateless / safe for concurrent use.
type HerdrClient interface {
	// FocusedPane resolves the pane the given session currently has
	// focused. Callers use this to find the inject target for /send: there
	// is no target picker, injection always goes to the focused pane. A
	// failure here (session doesn't exist) is the caller's cue to return a
	// 4xx "bad session" — as opposed to a PaneRun failure, which happens
	// after the session was proven to exist and so indicates a genuine
	// server-side fault (5xx). See send.go for the full mapping.
	FocusedPane(ctx context.Context, session string) (*PaneInfo, error)

	// PaneRun types text into pane and submits it (text + Enter) as one
	// atomic action — Herdr's `pane run`, verified live to keep paths with
	// slashes/spaces intact.
	PaneRun(ctx context.Context, session, pane, text string) error

	// PaneSendInput performs one raw socket pane.send_input mutation containing
	// both text and the modified submit key. Sequential text/key calls are never
	// used because they could partially inject on failure.
	PaneSendInput(ctx context.Context, session, pane, text, key string) error

	// PaneRead returns raw visible text for the focused-pane browser preview,
	// up to lines lines (0 = herdr's default).
	PaneRead(ctx context.Context, session, pane string, lines int) (string, error)
}

// ExecHerdrClient is the production HerdrClient: it shells out to the
// `herdr` binary found on PATH via exec.CommandContext, so a cancelled ctx
// (request timeout, client disconnect) kills the subprocess instead of
// leaking it. Herdr commands use JSON output and are parsed directly, except
// PaneRead deliberately requests `--format text` and returns raw visible-pane
// output for browser display.
type ExecHerdrClient struct {
	logger *slog.Logger
}

// NewExecHerdrClient returns the production HerdrClient. It has no other
// state: every call re-invokes the herdr binary, matching the CLI's own
// process-per-command model. logger receives one line per herdr invocation
// (command, exit code, stderr, duration — the design doc's "inject path
// instrumented" requirement); its *Context methods pick up the caller's
// correlation id automatically (see internal/logger), so callers only need
// to thread ctx through, not the id itself.
func NewExecHerdrClient(logger *slog.Logger) *ExecHerdrClient {
	return &ExecHerdrClient{logger: logger}
}

type paneCurrentResponse struct {
	Result struct {
		Pane struct {
			PaneID string `json:"pane_id"`
		} `json:"pane"`
	} `json:"result"`
}

func (c *ExecHerdrClient) FocusedPane(ctx context.Context, session string) (*PaneInfo, error) {
	out, err := run(ctx, c.logger, session, "pane", "current")
	if err != nil {
		return nil, err
	}
	var resp paneCurrentResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, errors.Wrap(err, "parse pane current response")
	}
	if resp.Result.Pane.PaneID == "" {
		return nil, errors.New("herdr reported no focused pane")
	}
	return &PaneInfo{PaneID: resp.Result.Pane.PaneID}, nil
}

func (c *ExecHerdrClient) PaneRun(ctx context.Context, session, pane, text string) error {
	_, err := run(ctx, c.logger, session, "pane", "run", pane, text)
	return err
}

type serverStatus struct {
	Running  bool    `json:"running"`
	Protocol *uint32 `json:"protocol"`
	Socket   string  `json:"socket"`
}

type socketResponse struct {
	ID     string `json:"id"`
	Result *struct {
		Type string `json:"type"`
	} `json:"result,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// parseServerStatus validates only reviewed protocol 16/17 status envelopes.
// Both versions share the consumed status and pane.send_input shapes; newer
// protocols fail closed until their schema and a full-path fixture are reviewed.
func parseServerStatus(out []byte) (serverStatus, error) {
	var status serverStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return status, errors.Wrap(err, "parse herdr server status")
	}
	if !status.Running || status.Protocol == nil || status.Socket == "" {
		return status, errors.New("herdr session is not running or status is incomplete")
	}
	if *status.Protocol != 16 && *status.Protocol != 17 {
		return status, fmt.Errorf("unsupported herdr protocol %d", *status.Protocol)
	}
	if !filepath.IsAbs(status.Socket) {
		return status, errors.New("herdr status returned a non-absolute socket path")
	}
	return status, nil
}

// PaneSendInput resolves the named session through Herdr itself, then sends one
// newline-delimited request over its Unix socket. Socket path and protocol come
// from validated status output rather than HERDR_SOCKET_PATH, which may point at
// another session when this service handles multiple URL sessions.
func (c *ExecHerdrClient) PaneSendInput(ctx context.Context, session, pane, text, key string) error {
	if key != "ctrl+enter" && key != "alt+enter" {
		return fmt.Errorf("unsupported pane.send_input key %q", key)
	}
	out, err := run(ctx, c.logger, session, "status", "server", "--json")
	if err != nil {
		return err
	}
	status, err := parseServerStatus(out)
	if err != nil {
		return err
	}
	info, err := os.Stat(status.Socket)
	if err != nil {
		return errors.Wrap(err, "stat herdr session socket")
	}
	if info.Mode()&os.ModeSocket == 0 {
		return errors.New("herdr status path is not a Unix socket")
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", status.Socket)
	if err != nil {
		return errors.Wrap(err, "connect to herdr session socket")
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	const requestID = "herdr-web-tui-send"
	request := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params struct {
			PaneID string   `json:"pane_id"`
			Text   string   `json:"text"`
			Keys   []string `json:"keys"`
		} `json:"params"`
	}{ID: requestID, Method: "pane.send_input"}
	request.Params.PaneID = pane
	request.Params.Text = text
	request.Params.Keys = []string{key}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return errors.Wrap(err, "write pane.send_input request")
	}

	var response socketResponse
	if err := json.NewDecoder(io.LimitReader(bufio.NewReaderSize(conn, 64*1024), 64*1024)).Decode(&response); err != nil {
		return errors.Wrap(err, "read pane.send_input response")
	}
	if response.ID != requestID {
		return errors.New("pane.send_input response id mismatch")
	}
	if response.Error != nil {
		return fmt.Errorf("herdr socket error %s: %s", response.Error.Code, response.Error.Message)
	}
	if response.Result == nil || response.Result.Type != "ok" {
		return errors.New("pane.send_input response missing reviewed ok shape")
	}
	return nil
}

func (c *ExecHerdrClient) PaneRead(ctx context.Context, session, pane string, lines int) (string, error) {
	args := []string{"pane", "read", pane, "--source", "visible", "--format", "text"}
	if lines > 0 {
		args = append(args, "--lines", strconv.Itoa(lines))
	}
	out, err := run(ctx, c.logger, session, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// run invokes `herdr --session <session> <args...>` and returns stdout on
// success. On failure the returned error's message is herdr's stderr,
// quoted exactly (design doc: "herdr stderr quoted exact") — never
// swallowed, never reworded, so the operator sees the real cause. When the
// herdr binary itself could not be spawned (missing from PATH), the error
// instead wraps ErrUnreachable so callers can distinguish "herdr is not
// installed/running at all" from "herdr ran and rejected the command".
//
// Every invocation is logged (command, exit code, stderr, duration) if
// logger is non-nil — this is the source of the design doc's "each pane run
// cmd/exit/stderr/duration" instrumentation requirement; logging here
// (rather than in each exported method) covers FocusedPane, PaneRun, and
// PaneRead uniformly with one code path.
func run(ctx context.Context, logger *slog.Logger, session string, args ...string) ([]byte, error) {
	fullArgs := append([]string{"--session", session}, args...)
	command := "herdr " + strings.Join(fullArgs, " ")
	cmd := exec.CommandContext(ctx, "herdr", fullArgs...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)
	stderrMsg := strings.TrimSpace(stderr.String())

	if logger != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if err != nil {
			exitCode = -1
		}
		attrs := []slog.Attr{
			slog.String("command", command),
			slog.Int("exit_code", exitCode),
			slog.String("stderr", stderrMsg),
			slog.Duration("duration", duration),
		}
		if err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "herdr command failed", attrs...)
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "herdr command ok", attrs...)
		}
	}

	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, errors.Wrap(ErrUnreachable, "herdr binary not found on PATH")
		}
		msg := stderrMsg
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(fmt.Sprintf("herdr command failed: %s", msg))
	}
	return []byte(stdout.String()), nil
}
