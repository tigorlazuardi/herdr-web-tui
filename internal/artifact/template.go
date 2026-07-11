package artifact

import (
	"strings"

	"github.com/go-faster/errors"
)

// Segment is one piece of a promptbox template: either a literal text run
// (Text set, File empty) or a file marker (File set to the multipart form
// field name carrying that attachment, Text empty). The frontend's segment
// editor (ticket 6) sends these in caret order — order in the slice IS the
// ordering invariant; there is no separate index field.
type Segment struct {
	Text string `json:"text,omitempty"`
	File string `json:"file,omitempty"`
}

// Template is the JSON shape carried in the /send multipart request's
// "template" form field: the ordered segment list described in the design
// doc's "Artifact promptbox" section (positional template, not string
// parsing with [File N] placeholders).
type Template struct {
	Segments []Segment `json:"segments"`
}

// Resolve is the **resolve step**: it turns a template into the final
// string that gets typed into the pane, substituting each file marker for
// its already-saved server-side path. saved maps a segment's File field
// (the multipart form field name) to the absolute path SaveFiles wrote it
// to.
//
// This must run strictly after every file in the template has been saved
// (see the save-then-resolve-then-inject ordering documented on the /send
// handler) — Resolve itself does no I/O and cannot fail for any reason
// other than a template referencing a marker that was never saved, which
// indicates a caller bug (an unresolved File field slipped through), not a
// runtime condition.
func Resolve(segments []Segment, saved map[string]string) (string, error) {
	var b strings.Builder
	for _, seg := range segments {
		if seg.File == "" {
			b.WriteString(seg.Text)
			continue
		}
		path, ok := saved[seg.File]
		if !ok {
			return "", errors.New("resolve template: no saved file for marker " + seg.File)
		}
		b.WriteString(path)
	}
	return b.String(), nil
}
