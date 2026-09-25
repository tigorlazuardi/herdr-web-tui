package artifact

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// setupRecordingSSH installs a fake ssh binary that appends its full remote
// command to $SSH_CALLS and writes stdin bytes to $REMOTE_ROOT + <path>,
// where <path> is parsed out of `cat > <path>`. POSIX-only, same trade-off
// as the herdrclient CLI-boundary tests.
func setupRecordingSSH(t *testing.T) (callsPath, remoteRoot string) {
	t.Helper()
	if isWindows() {
		t.Skip("fake ssh shell executable requires POSIX shell")
	}
	dir := t.TempDir()
	callsPath = filepath.Join(dir, "calls")
	remoteRoot = filepath.Join(dir, "remote")
	script := "#!/bin/sh\nPATH=/usr/bin:/bin\ncat >> \"$SSH_CALLS\" <<EOF\n$*\nEOF\nremotepath=${2##*cat > }\nif [ -n \"$remotepath\" ]; then mkdir -p \"$REMOTE_ROOT${remotepath%/*}\" && cat > \"$REMOTE_ROOT$remotepath\"; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SSH_CALLS", callsPath)
	t.Setenv("REMOTE_ROOT", remoteRoot)
	return callsPath, remoteRoot
}

func isWindows() bool { return os.PathSeparator == '\\' }

func TestSyncToRemote_StreamsEveryFileUnderSamePath(t *testing.T) {
	callsPath, remoteRoot := setupRecordingSSH(t)
	dir := "/tmp/stage-1000"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.png")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("PNGBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := SyncToRemote(context.Background(), "tigor@box", dir, map[string]string{"f1": a, "f2": b}); err != nil {
		t.Fatalf("SyncToRemote: %v", err)
	}

	for name, want := range map[string]string{a: "PNGBYTES", b: "text"} {
		got, err := os.ReadFile(filepath.Join(remoteRoot, name))
		if err != nil {
			t.Fatalf("%s: remote copy missing: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("%s: remote bytes = %q, want %q", name, got, want)
		}
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(calls), "\n"); n != 2 {
		t.Fatalf("expected 2 ssh invocations, got %d: %q", n, string(calls))
	}
	if !slices.Contains(strings.Split(string(calls), "\n"), "tigor@box mkdir -p /tmp/stage-1000 && cat > /tmp/stage-1000/a.png") {
		t.Fatalf("expected exact mkdir+cat remote command, got %q", string(calls))
	}
}

func TestSyncToRemote_RejectsPathsOutsideStagingNamespace(t *testing.T) {
	callsPath, _ := setupRecordingSSH(t)
	if err := SyncToRemote(context.Background(), "tigor@box", "/tmp/stage-1000", map[string]string{"f": "/etc/passwd"}); err == nil {
		t.Fatal("expected rejection of path outside staging namespace")
	}
	if err := SyncToRemote(context.Background(), "tigor@box", "/tmp/stage-1000", map[string]string{"f": "/tmp/stage-1000/../../etc/passwd"}); err == nil {
		t.Fatal("expected rejection of traversal path")
	}
	if err := SyncToRemote(context.Background(), "tigor@box", "/tmp/stage-1000", map[string]string{"f": "/tmp/stage-1000/x;rm -rf /"}); err == nil {
		t.Fatal("expected rejection of shell-hostile characters")
	}
	if calls, err := os.ReadFile(callsPath); err == nil && len(calls) != 0 {
		t.Fatalf("rejected inputs must not reach ssh, got %q", string(calls))
	}
}

func TestSyncToRemote_SshFailureSurfacesStderr(t *testing.T) {
	staging := "/tmp/stage-1000"
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(staging, "a.png")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	failing := t.TempDir()
	if err := os.WriteFile(filepath.Join(failing, "ssh"), []byte("#!/bin/sh\necho host unreachable >&2\nexit 255\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", failing)

	err := SyncToRemote(context.Background(), "tigor@box", staging, map[string]string{"f": f})
	if err == nil || !strings.Contains(err.Error(), "host unreachable") {
		t.Fatalf("expected ssh stderr verbatim, got %v", err)
	}
}
