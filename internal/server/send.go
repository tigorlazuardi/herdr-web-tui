package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/go-faster/errors"
	"github.com/tigorlazuardi/herdr-web-tui/internal/artifact"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
)

// sendResponse is the JSON body /send returns on both success and failure.
// Failures also carry the correlation id in the X-Request-Id /
// X-Correlation-Id response headers (set by withCorrelation, ahead of this
// handler in the chain) so the frontend's "ref: req_xxx" display and this
// body's Error field both point at the same log lines.
type sendResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// sendHandler implements POST /send, the artifact-inject endpoint: see the
// design doc's "Artifact daemon — upload endpoint" section for the full
// contract. It is the seam this ticket's tests exercise (fake HerdrClient +
// temp stagingDir via httptest).
type sendHandler struct {
	herdr      herdrclient.HerdrClient
	stagingDir string
	logger     *slog.Logger
}

type submitKey string

const (
	submitEnter     submitKey = "enter"
	submitCtrlEnter submitKey = "ctrl-enter"
	submitAltEnter  submitKey = "alt-enter"
)

// newSendHandler wires a sendHandler. stagingDir is the flat directory
// uploaded files are saved into (production: artifact.DefaultDir's result;
// tests: t.TempDir()).
func newSendHandler(herdr herdrclient.HerdrClient, stagingDir string, logger *slog.Logger) *sendHandler {
	return &sendHandler{herdr: herdr, stagingDir: stagingDir, logger: logger}
}

// ServeHTTP runs the atomic inject flow in a fixed order that is itself the
// all-or-nothing guarantee — there is no rollback logic anywhere, because
// ordering alone makes a partial inject impossible:
//
//  1. Parse the multipart request: the "template" field (JSON-encoded
//     artifact.Template), "session", and optional validated "submitKey" field.
//  2. Save phase — every file field the template references is written to
//     stagingDir. The first save error aborts here, before anything has
//     been typed into any pane; files already saved for this request are
//     simply orphaned in stagingDir (OS /tmp cleanup, not app-managed — see
//     the design doc's storage-namespace decision).
//  3. Resolve phase — pure, in-memory: substitute each file marker for its
//     saved path (artifact.Resolve). The client never sees these paths.
//  4. Inject phase — exactly one HerdrClient.FocusedPane query + one final
//     PaneRun (Enter) or PaneSendInput (modified Enter) mutation. If it fails,
//     nothing was typed, by construction (nothing before it touches the
//     pane at all).
//
// A save failure is a 400 (client sent a malformed/oversized part the
// gateway didn't already reject) unless it's an I/O error, which is a 500.
// A FocusedPane failure is treated as a bad session name (4xx) since it is
// the first herdr call in the flow and a live session was never
// established; a PaneRun failure happens only after the session was proven
// to resolve a focused pane, so it is treated as a genuine server-side
// fault (5xx). herdr being unreachable at all (binary missing, socket
// down) is always 5xx regardless of which call surfaced it.
func (h *sendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		writeSendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		h.logger.WarnContext(ctx, "send: malformed multipart request", slog.String("error", err.Error()))
		writeSendError(w, http.StatusBadRequest, "malformed multipart request: "+err.Error())
		return
	}

	tmpl, session, key, saved, fileCount, err := readParts(mr, h.stagingDir)
	if err != nil {
		h.logger.WarnContext(ctx, "send: request parse/save failed", slog.String("error", err.Error()))
		writeSendError(w, statusFor(err), err.Error())
		return
	}
	session = sanitizeSession(session)

	h.logger.InfoContext(ctx, "send: upload received",
		slog.String("session", session),
		slog.String("submit_key", string(key)),
		slog.Int("file_count", fileCount),
	)

	text, err := artifact.Resolve(tmpl.Segments, saved)
	if err != nil {
		// Only reachable on a caller/template bug (an unresolved marker) —
		// every file the template can reference was just saved above.
		h.logger.ErrorContext(ctx, "send: resolve failed", slog.String("error", err.Error()))
		writeSendError(w, http.StatusBadRequest, err.Error())
		return
	}

	start := time.Now()
	pane, err := h.herdr.FocusedPane(ctx, session)
	if err != nil {
		h.logger.WarnContext(ctx, "send: focused-pane resolution failed",
			slog.String("session", session), slog.String("error", err.Error()))
		if errors.Is(err, herdrclient.ErrUnreachable) {
			writeSendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeSendError(w, http.StatusBadRequest, fmt.Sprintf("bad session %q: %s", session, err))
		return
	}

	if key == submitEnter {
		err = h.herdr.PaneRun(ctx, session, pane.PaneID, text)
	} else {
		herdrKey := map[submitKey]string{submitCtrlEnter: "ctrl+enter", submitAltEnter: "alt+enter"}[key]
		err = h.herdr.PaneSendInput(ctx, session, pane.PaneID, text, herdrKey)
	}
	duration := time.Since(start)
	if err != nil {
		h.logger.ErrorContext(ctx, "send: inject failed",
			slog.String("session", session), slog.String("pane", pane.PaneID),
			slog.String("submit_key", string(key)),
			slog.Duration("duration", duration), slog.String("error", err.Error()),
		)
		writeSendError(w, http.StatusInternalServerError, "inject failed: "+err.Error())
		return
	}

	h.logger.InfoContext(ctx, "send: inject ok",
		slog.String("session", session), slog.String("pane", pane.PaneID),
		slog.String("submit_key", string(key)), slog.Duration("duration", duration),
	)
	writeJSON(w, http.StatusOK, sendResponse{OK: true})
}

// readParts consumes every multipart part exactly once (a multipart.Reader
// is forward-only): the "template", "session", and optional "submitKey"
// form fields, and every
// file part named in the template's segments, saving each to dir as it is
// streamed off the wire (never buffering a whole upload in memory). It
// returns the parsed template, the raw session field value, a map from
// form field name to saved path (Resolve's input), and how many files were
// saved (for the upload-received log line).
func readParts(mr *multipart.Reader, dir string) (tmpl artifact.Template, session string, key submitKey, saved map[string]string, fileCount int, err error) {
	saved = map[string]string{}
	key = submitEnter
	haveTemplate := false

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return tmpl, session, key, saved, fileCount, badRequestf("read multipart part: %s", err)
		}

		name := part.FormName()
		switch {
		case name == "template":
			if err := json.NewDecoder(part).Decode(&tmpl); err != nil {
				part.Close()
				return tmpl, session, key, saved, fileCount, badRequestf("decode template field: %s", err)
			}
			haveTemplate = true
		case name == "session":
			buf := make([]byte, 256)
			n, _ := part.Read(buf)
			session = string(buf[:n])
		case name == "submitKey":
			buf, readErr := io.ReadAll(io.LimitReader(part, 32))
			if readErr != nil {
				part.Close()
				return tmpl, session, key, saved, fileCount, badRequestf("read submitKey field: %s", readErr)
			}
			key = submitKey(buf)
			if key != submitEnter && key != submitCtrlEnter && key != submitAltEnter {
				part.Close()
				return tmpl, session, key, saved, fileCount, badRequestf("invalid submitKey %q", key)
			}
		default:
			// Any other named part is a file attachment referenced by a
			// template segment's File field (the form field name).
			contentType := part.Header.Get("Content-Type")
			if contentType != "" {
				if mt, _, mErr := mime.ParseMediaType(contentType); mErr == nil {
					contentType = mt
				}
			}
			path, saveErr := artifact.SaveFile(dir, part, part.FileName(), contentType)
			part.Close()
			if saveErr != nil {
				// I/O failure staging a file is a server-side fault, not a
				// client mistake.
				return tmpl, session, key, saved, fileCount, internalf("save %q: %s", name, saveErr)
			}
			saved[name] = path
			fileCount++
			continue
		}
		part.Close()
	}

	if !haveTemplate {
		return tmpl, session, key, saved, fileCount, badRequestf("missing required \"template\" field")
	}
	return tmpl, session, key, saved, fileCount, nil
}

// clientlogRequest is the JSON body POST /clientlog accepts: a frontend
// error-boundary report, tagged with the correlation id the frontend read
// off X-Request-Id/X-Correlation-Id on the failing request/response so this
// log line and the backend's own logging of that request reconcile under
// one id (design doc: "FE and BE errors reconcile in one stream").
type clientlogRequest struct {
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	URL     string `json:"url,omitempty"`
	RefID   string `json:"ref_id,omitempty"`
}

// clientlogHandler implements POST /clientlog.
type clientlogHandler struct {
	logger *slog.Logger
}

func newClientlogHandler(logger *slog.Logger) *clientlogHandler {
	return &clientlogHandler{logger: logger}
}

func (h *clientlogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeSendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req clientlogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSendError(w, http.StatusBadRequest, "malformed clientlog body: "+err.Error())
		return
	}
	if req.Message == "" {
		writeSendError(w, http.StatusBadRequest, "missing required \"message\" field")
		return
	}

	// req.RefID is the id the FE display showed the user; correlation.Attr
	// tags this line with *this request's own* id (from X-Request-Id on
	// this POST, per the correlation middleware). Both are logged: ref_id
	// lets an operator jump straight to the failing request's log lines
	// even though this /clientlog POST itself has a different id.
	h.logger.ErrorContext(r.Context(), "client error report",
		slog.String("message", req.Message),
		slog.String("stack", req.Stack),
		slog.String("url", req.URL),
		slog.String("ref_id", req.RefID),
	)
	writeJSON(w, http.StatusOK, sendResponse{OK: true})
}

func writeSendError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, sendResponse{OK: false, Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// statusCoder lets readParts's internal errors (badRequestf / internalf)
// carry an HTTP status alongside their message without a bespoke error type
// per call site.
type statusCoder interface {
	StatusCode() int
}

type statusErr struct {
	code int
	msg  string
}

func (e *statusErr) Error() string   { return e.msg }
func (e *statusErr) StatusCode() int { return e.code }

func badRequestf(format string, a ...any) error {
	return &statusErr{code: http.StatusBadRequest, msg: fmt.Sprintf(format, a...)}
}

func internalf(format string, a ...any) error {
	return &statusErr{code: http.StatusInternalServerError, msg: fmt.Sprintf(format, a...)}
}

// statusFor extracts the intended HTTP status from an error produced by
// readParts, defaulting to 400 (the historically common case: a malformed
// request) if err doesn't carry one.
func statusFor(err error) int {
	var sc statusCoder
	if errors.As(err, &sc) {
		return sc.StatusCode()
	}
	return http.StatusBadRequest
}
