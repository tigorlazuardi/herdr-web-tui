package artifact

import (
	"io"
	"mime"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
	"github.com/rs/xid"
)

// SaveFile writes src to dir/<uuid>[.ext], flat (no nesting), and returns
// the absolute path written. The uuid is an rs/xid value — the same
// generator internal/correlation uses for request ids, reused here purely
// for "cheap unique id", not for any correlation meaning. ext is an "agent
// hint" only (design doc): derived from the original filename if it has
// one, else guessed from contentType; the file is never opened/sniffed for
// content, so a wrong/missing hint never blocks the save.
//
// Any I/O error here must abort the whole /send flow before anything is
// injected — see the ordering invariant documented on the /send handler.
func SaveFile(dir string, src io.Reader, filename, contentType string) (path string, err error) {
	ext := filepath.Ext(filename)
	if ext == "" && contentType != "" {
		if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
			ext = exts[0]
		}
	}

	path = filepath.Join(dir, xid.New().String()+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", errors.Wrap(err, "create staged file")
	}
	defer f.Close()

	if _, err := io.Copy(f, src); err != nil {
		os.Remove(path) // best-effort: don't leave a half-written file behind on failure.
		return "", errors.Wrap(err, "write staged file")
	}
	return path, nil
}
