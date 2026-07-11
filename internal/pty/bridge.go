package pty

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/go-faster/errors"
)

// Size is the terminal dimensions passed to Spawn and Resize. It mirrors
// creack/pty.Winsize but stays in this package so callers (the ws handler)
// don't need to import creack/pty directly.
type Size struct {
	Cols, Rows uint16
}

// Bridge owns one live "herdr --session <name>" pty. Spawn creates it; Run
// copies bytes until ctx is cancelled or herdr exits; Write and Resize are
// safe to call concurrently with Run from another goroutine (the ws read
// loop) for the lifetime of the Bridge.
type Bridge struct {
	cmd *exec.Cmd
	pty *os.File
}

// Spawn starts "herdr --session <session>" in a new pty sized to size, and
// returns once the process has been started (not once it has produced any
// output). session must already be sanitized by the caller — this function
// passes it straight to exec as an argv element, never through a shell, so
// it cannot be used to inject a different subcommand or shell
// metacharacter; an unexpected value simply surfaces as a "no such
// session"-style error from herdr itself.
//
// The returned *Bridge is not yet wired to ctx: call Run to start pumping
// output and to tie the pty/process lifetime to ctx.
func Spawn(session string, size Size) (*Bridge, error) {
	cmd := exec.Command("herdr", "--session", session)
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
	if err != nil {
		return nil, errors.Wrap(err, "spawn herdr pty")
	}
	return &Bridge{cmd: cmd, pty: f}, nil
}

// Run copies pty output to onOutput until either the pty read errors (most
// commonly: herdr exited, so the pty's slave side closed) or ctx is
// cancelled. It always returns nil for a clean shutdown; a non-nil error
// means the underlying read failed for a reason other than the pty simply
// closing (EOF), which the caller should log and surface as a FrameError.
//
// Why a goroutine-plus-select instead of a context-aware read: os.File has
// no ctx-aware Read, so the only way to make the read interruptible is to
// run it on its own goroutine and abandon that goroutine (by returning from
// Run) when ctx cancels. The read may still be blocked in the kernel
// briefly after Run returns — Close (which the caller must defer) is what
// actually unblocks it, by closing the pty fd out from under the pending
// read. The abandoned goroutine then observes the Read error and exits on
// its own; it never blocks process shutdown because it holds no other
// resources and is never waited on.
func (b *Bridge) Run(ctx context.Context, onOutput func([]byte)) error {
	readErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := b.pty.Read(buf)
			if n > 0 {
				out := make([]byte, n)
				copy(out, buf[:n])
				onOutput(out)
			}
			if err != nil {
				readErr <- err
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-readErr:
		if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.Wrap(err, "read pty")
	}
}

// Write sends bytes to the pty's stdin, i.e. keystrokes/paste from the
// browser. Safe to call concurrently with Run.
func (b *Bridge) Write(p []byte) (int, error) {
	return b.pty.Write(p)
}

// Resize applies a new terminal size to the pty (TIOCSWINSZ), which
// delivers SIGWINCH to herdr so it re-lays-out — this is what makes the
// browser fit-addon's cols/rows changes actually reach Herdr's mobile
// layout threshold. Safe to call concurrently with Run.
func (b *Bridge) Resize(size Size) error {
	return pty.Setsize(b.pty, &pty.Winsize{Cols: size.Cols, Rows: size.Rows})
}

// Close closes the pty fd (unblocking any pending Read in Run) and reaps
// the herdr process, so Run's caller never leaks a pty file descriptor or a
// zombie process. Must be called exactly once, after Run has returned or
// concurrently with it (see Run's doc comment) — it is the only supported
// way to unblock a still-running Run.
func (b *Bridge) Close() error {
	closeErr := b.pty.Close()
	_ = b.cmd.Wait() // best effort: reap the process, ignore its exit error
	return closeErr
}
