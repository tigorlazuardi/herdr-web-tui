// Package artifact resolves the flat, server-side directory /send stages
// uploaded files into before they are injected as resolved paths — see the
// design doc's "Artifact storage namespace" decision:
// /tmp/<prefix>-<server-uid>/<uuid>[.ext].
package artifact

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/go-faster/errors"
)

// DefaultDir returns the flat staging directory this process should save
// uploads into: os.TempDir()/<prefix>-<server-uid>, where <server-uid> is
// the numeric effective UID (`id -u`) of this process (storage location is a
// server-side concern only — no gateway header, no per-connection id, see
// the design doc). The directory is created (0700, owner-only — uploads may
// be arbitrary user files) if it does not already exist.
func DefaultDir(prefix string) (string, error) {
	dir := filepath.Join(os.TempDir(), prefix+"-"+strconv.Itoa(os.Geteuid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.Wrap(err, "create artifact staging dir")
	}
	return dir, nil
}
