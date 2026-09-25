package artifact

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// SyncToRemote copies every staged file to the same absolute path on the
// ssh target so a machine-routed inject can reference them: the promptbox
// contract is strictly drop-path, and the remote agent reads the file from
// its own filesystem. Paths are identical on both hosts (same flat staging
// directory), so callers keep composing text from the same saved map.
//
// Transport is `ssh <target> mkdir -p <dir> && cat > <path>` with the file
// streamed on stdin — nothing beyond sshd is required on the remote host
// (no scp/sftp subsystem). Every file is synced before the first byte is
// typed anywhere: a failure here aborts the send before injection, so the
// user's input is preserved (atomic-bundle rule).
//
// ponytail: paths are uuid-named under one flat dir and re-validated here;
// a general remote-copy helper would want real shell quoting instead.
func SyncToRemote(ctx context.Context, target, dir string, files map[string]string) error {
	if target == "" {
		return fmt.Errorf("sync: empty ssh target")
	}
	if len(files) == 0 {
		return nil
	}
	if err := validateStagingPath(dir); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name, path := range files {
		if err := validateStagingPath(path); err != nil {
			return fmt.Errorf("sync %q: %w", name, err)
		}
		if err := validateInside(path, dir); err != nil {
			return fmt.Errorf("sync %q: %w", name, err)
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := streamOne(ctx, target, dir, files[name]); err != nil {
			return fmt.Errorf("sync %q to %s: %w", name, target, err)
		}
	}
	return nil
}

// streamOne transfers one file: ssh runs `mkdir -p <dir> && cat > <path>`
// on the remote host while stdin streams the staged bytes. stderr is kept
// verbatim in the error (same "quote exact" convention as herdrclient).
func streamOne(ctx context.Context, target, dir, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()

	remote := "mkdir -p " + dir + " && cat > " + path
	cmd := exec.CommandContext(ctx, "ssh", target, remote)
	cmd.Stdin = src
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ssh failed: %s", msg)
	}
	return nil
}

// validateStagingPath rejects anything outside the expected flat staging
// namespace before it can reach a remote shell: /tmp/<prefix>-<uid>/<uuid>.
func validateStagingPath(path string) error {
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, "/tmp/") {
		return fmt.Errorf("path %q is outside the staging namespace", path)
	}
	for _, r := range clean {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("path %q contains unsupported characters", path)
		}
	}
	return nil
}

// validateInside ensures a file path lives directly inside the staging dir
// (no traversal onto other remote locations via crafted names).
func validateInside(path, dir string) error {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("path %q escapes staging dir %q", path, dir)
	}
	return nil
}
