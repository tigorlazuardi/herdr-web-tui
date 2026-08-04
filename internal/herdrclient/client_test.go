package herdrclient

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

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
