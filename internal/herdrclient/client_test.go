package herdrclient

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// setupPaneSendInputSocket exposes a controlled reviewed-protocol Unix socket through fake Herdr status output.
func setupPaneSendInputSocket(t *testing.T, protocol int) net.Listener {
	t.Helper()
	dir := t.TempDir()
	socket := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	herdrPath := filepath.Join(dir, "herdr")
	script := "#!/bin/sh\nprintf '{\"running\":true,\"protocol\":" + strconv.Itoa(protocol) + ",\"socket\":\"%s\"}\\n' \"$HERDR_TEST_SOCKET\"\n"
	if err := os.WriteFile(herdrPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HERDR_TEST_SOCKET", socket)
	return listener
}

func TestParseServerStatus_AcceptsReviewedFixturesAndRejectsUnknown(t *testing.T) {
	for _, protocol := range []int{16, 17, 19, 20, 21, 22} {
		fixture := []byte(`{"running":true,"protocol":` + strconv.Itoa(protocol) + `,"socket":"/tmp/herdr.sock"}`)
		if _, err := parseServerStatus(fixture); err != nil {
			t.Fatalf("protocol %d fixture rejected: %v", protocol, err)
		}
	}
	if _, err := parseServerStatus([]byte(`{"running":true,"protocol":23,"socket":"/tmp/herdr.sock"}`)); err == nil || !strings.Contains(err.Error(), "unsupported herdr protocol 23") {
		t.Fatalf("expected unknown protocol to fail closed, got %v", err)
	}
}

func TestExecHerdrClient_PaneSendInput_RejectsUnreviewedKey(t *testing.T) {
	client := NewExecHerdrClient(nil)
	if err := client.PaneSendInput(context.Background(), "work", "w1:p2", "hello", "shift+enter"); err == nil || !strings.Contains(err.Error(), "unsupported pane.send_input key") {
		t.Fatalf("expected unsupported key rejection before I/O, got %v", err)
	}
}

func TestExecHerdrClient_PaneSendInput_WritesReviewedEnvelopeForAcceptedProtocols(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket transport")
	}
	for _, protocol := range []int{16, 17, 19, 20, 21, 22} {
		t.Run(strconv.Itoa(protocol), func(t *testing.T) {
			listener := setupPaneSendInputSocket(t, protocol)
			request := make(chan string, 1)
			go func() {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				defer conn.Close()
				line, _ := bufio.NewReader(conn).ReadBytes('\n')
				request <- string(line)
				_, _ = conn.Write([]byte(`{"id":"herdr-web-tui-send","result":{"type":"ok"}}` + "\n"))
			}()

			client := NewExecHerdrClient(nil)
			if err := client.PaneSendInput(context.Background(), "work", "w1:p2", "hello", "ctrl+enter"); err != nil {
				t.Fatalf("PaneSendInput: %v", err)
			}
			want := `{"id":"herdr-web-tui-send","method":"pane.send_input","params":{"pane_id":"w1:p2","text":"hello","keys":["ctrl+enter"]}}` + "\n"
			if got := <-request; got != want {
				t.Fatalf("envelope = %q, want %q", got, want)
			}
		})
	}
}

func TestExecHerdrClient_PaneSendInput_RejectsSocketFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket transport")
	}
	cases := []struct {
		name     string
		response string
		wantErr  string
	}{
		{name: "error envelope", response: `{"id":"herdr-web-tui-send","error":{"code":"E_DENIED","message":"denied"}}`, wantErr: "herdr socket error E_DENIED: denied"},
		{name: "mismatched id", response: `{"id":"another-request","result":{"type":"ok"}}`, wantErr: "pane.send_input response id mismatch"},
		{name: "missing reviewed ok result", response: `{"id":"herdr-web-tui-send","result":{"type":"pending"}}`, wantErr: "pane.send_input response missing reviewed ok shape"},
		{name: "malformed response", response: `{`, wantErr: "read pane.send_input response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listener := setupPaneSendInputSocket(t, 17)
			serverDone := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					serverDone <- err
					return
				}
				defer conn.Close()
				if _, err = bufio.NewReader(conn).ReadBytes('\n'); err == nil {
					_, err = conn.Write([]byte(tc.response + "\n"))
				}
				serverDone <- err
			}()

			err := NewExecHerdrClient(nil).PaneSendInput(context.Background(), "work", "w1:p2", "hello", "ctrl+enter")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if err := <-serverDone; err != nil {
				t.Fatalf("socket server: %v", err)
			}
		})
	}
}

func TestExecHerdrClient_PaneSendInput_CancellationClosesStalledSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket transport")
	}
	listener := setupPaneSendInputSocket(t, 17)
	requestRead := make(chan error, 1)
	peerRead := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			requestRead <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		_, err = reader.ReadBytes('\n')
		requestRead <- err
		if err == nil {
			_, err = reader.ReadByte()
			peerRead <- err
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		returned <- NewExecHerdrClient(nil).PaneSendInput(ctx, "work", "w1:p2", "hello", "ctrl+enter")
	}()
	if err := <-requestRead; err != nil {
		t.Fatalf("read request: %v", err)
	}
	cancel()
	select {
	case err := <-returned:
		if err == nil {
			t.Fatal("expected cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PaneSendInput did not return promptly after cancellation")
	}
	select {
	case err := <-peerRead:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected peer EOF after cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("peer connection remained open after cancellation")
	}
}

// setupArgCapturingHerdr installs a fake herdr binary that answers
// `machine list --json` from $HERDR_MACHINES_JSON and records every other
// invocation's argv, one argument per line, into $HERDR_ARGS. POSIX-only:
// the shell script keeps the CLI boundary test small (same trade-off as
// TestExecHerdrClient_PaneRead).
func setupArgCapturingHerdr(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("controlled herdr shell executable requires POSIX shell")
	}
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	herdrPath := filepath.Join(dir, "herdr")
	const script = "#!/bin/sh\nif [ \"$1\" = \"machine\" ]; then\n  printf '%s' \"$HERDR_MACHINES_JSON\"\n  exit ${HERDR_MACHINES_EXIT:-0}\nfi\nprintf '%s\\n' \"$@\" > \"$HERDR_ARGS\"\nif [ -n \"$HERDR_FAKE_STDOUT\" ]; then printf '%s' \"$HERDR_FAKE_STDOUT\"; fi\n"
	if err := os.WriteFile(herdrPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HERDR_ARGS", argsPath)
	return argsPath
}

func readArgsLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
}

func TestActiveMachine_ReturnsEnabledSelectedProfile(t *testing.T) {
	const catalog = `[
 {"id":"m1","label":"A","target":"a@box","session":"default","enabled":true,"selected":false},
 {"id":"m2","label":"B","target":"tigor@m2box","session":"default","enabled":true,"selected":true}
]`
	t.Setenv("HERDR_MACHINES_JSON", catalog)
	setupArgCapturingHerdr(t)

	profile, err := NewExecHerdrClient(nil).ActiveMachine(context.Background())
	if err != nil {
		t.Fatalf("ActiveMachine: %v", err)
	}
	if profile.ID != "m2" || profile.Target != "tigor@m2box" {
		t.Fatalf("profile = %+v, want id m2 target tigor@m2box", profile)
	}
}

func TestActiveMachine_EmptyWhenNothingSelectedOrDisabled(t *testing.T) {
	for name, catalog := range map[string]string{
		"none selected":         `[{"id":"m1","enabled":true,"selected":false}]`,
		"selected but disabled": `[{"id":"m1","enabled":false,"selected":true}]`,
		"empty catalog":         `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HERDR_MACHINES_JSON", catalog)
			setupArgCapturingHerdr(t)
			profile, err := NewExecHerdrClient(nil).ActiveMachine(context.Background())
			if err != nil || profile.ID != "" || profile.Target != "" {
				t.Fatalf("profile=%+v err=%v, want empty/local", profile, err)
			}
		})
	}
}

func TestActiveMachine_FailsClosedOnExecOrParseFailure(t *testing.T) {
	t.Setenv("HERDR_MACHINES_JSON", `{`)
	setupArgCapturingHerdr(t)
	if _, err := NewExecHerdrClient(nil).ActiveMachine(context.Background()); err == nil || !strings.Contains(err.Error(), "parse machine list") {
		t.Fatalf("expected parse failure, got %v", err)
	}

	t.Setenv("HERDR_MACHINES_JSON", ``)
	t.Setenv("HERDR_MACHINES_EXIT", "1")
	if _, err := NewExecHerdrClient(nil).ActiveMachine(context.Background()); err == nil {
		t.Fatal("expected exec failure to surface")
	}
}

func TestMachineRoutedClient_MachinePrefixAndAtomicTextArgv(t *testing.T) {
	t.Setenv("HERDR_MACHINES_JSON", `{"id":"m2","enabled":true,"selected":true}`)
	argsPath := setupArgCapturingHerdr(t)
	client := NewExecHerdrClient(nil).For("m2")

	if err := client.PaneRun(context.Background(), "ignored", "w1:p1", "hello spaced  text"); err != nil {
		t.Fatalf("PaneRun: %v", err)
	}
	// The text must stay one argv element: --machine routing is argv-based
	// (no shell), so spaces inside the prompt text cannot be re-split.
	want := []string{"--machine", "m2", "pane", "run", "w1:p1", "hello spaced  text"}
	if got := readArgsLines(t, argsPath); !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestMachineRoutedClient_PaneSendInputRejectedBeforeIO(t *testing.T) {
	client := NewExecHerdrClient(nil).For("m2")
	err := client.PaneSendInput(context.Background(), "ignored", "w1:p1", "hello", "ctrl+enter")
	if err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Fatalf("expected local-only rejection, got %v", err)
	}
}

func TestMachineRoutedClient_FocusedPaneResolvesOnRemote(t *testing.T) {
	t.Setenv("HERDR_MACHINES_JSON", `{"id":"m2","enabled":true,"selected":true}`)
	t.Setenv("HERDR_FAKE_STDOUT", `{"id":"x","result":{"pane":{"pane_id":"w4:p1"}}}`)
	argsPath := setupArgCapturingHerdr(t)

	pane, err := NewExecHerdrClient(nil).For("m2").FocusedPane(context.Background(), "ignored")
	if err != nil {
		t.Fatalf("FocusedPane: %v", err)
	}
	if pane.PaneID != "w4:p1" {
		t.Fatalf("pane = %q, want w4:p1", pane.PaneID)
	}
	// Resolution must run on the remote server (--machine prefix), never
	// against the stale local focus.
	want := []string{"--machine", "m2", "pane", "current"}
	if got := readArgsLines(t, argsPath); !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestMachineRoutedClient_FocusedPaneFailureIsUnreachable(t *testing.T) {
	t.Setenv("HERDR_MACHINES_JSON", `{"id":"m2","enabled":true,"selected":true}`)
	t.Setenv("HERDR_FAIL", "1")
	dir := t.TempDir()
	herdrPath := filepath.Join(dir, "herdr")
	script := "#!/bin/sh\nif [ \"$1\" = \"machine\" ]; then printf '{}' ; exit 0; fi\nprintf 'ssh failed\\n' >&2\nexit 1\n"
	if err := os.WriteFile(herdrPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := NewExecHerdrClient(nil).For("m2").FocusedPane(context.Background(), "default")
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("expected ErrUnreachable for routed focus failure, got %v", err)
	}
}

func TestExecHerdrClient_PaneRead_UsesVisibleTextAndPreservesOutput(t *testing.T) {
	// ponytail: POSIX script keeps CLI boundary test small; use a Go helper executable if Windows support is needed.
	if runtime.GOOS == "windows" {
		t.Skip("controlled herdr shell executable requires POSIX shell")
	}

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	herdrPath := filepath.Join(dir, "herdr")
	const script = "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HERDR_ARGS\"\nif [ \"$HERDR_FAIL\" = 1 ]; then\n  printf 'pane missing\\n' >&2\n  exit 1\nfi\nprintf '  raw output\\n'\n"
	if err := os.WriteFile(herdrPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HERDR_ARGS", argsPath)
	client := NewExecHerdrClient(nil)

	text, err := client.PaneRead(context.Background(), "work", "w1:p2", 42)
	if err != nil {
		t.Fatalf("PaneRead: %v", err)
	}
	if text != "  raw output\n" {
		t.Fatalf("expected raw stdout, got %q", text)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(args)), []string{"--session", "work", "pane", "read", "w1:p2", "--source", "visible", "--format", "text", "--lines", "42"}; !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}

	t.Setenv("HERDR_FAIL", "1")
	if _, err := client.PaneRead(context.Background(), "work", "w1:p2", 0); err == nil || err.Error() != "herdr command failed: pane missing" {
		t.Fatalf("expected exact command failure, got %v", err)
	}
	args, err = os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(args)), []string{"--session", "work", "pane", "read", "w1:p2", "--source", "visible", "--format", "text"}; !slices.Equal(got, want) {
		t.Fatalf("args without lines = %q, want %q", got, want)
	}
}
