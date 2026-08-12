package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tigorlazuardi/herdr-web-tui/internal/artifact"
	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
)

// fakeHerdrClient is the design doc's "fake HerdrClient" test seam: it
// records every call for assertion and returns canned results/errors
// instead of shelling out to a live herdr, so /send's atomicity and
// error-mapping logic is exercised without a live Herdr server.
type fakeHerdrClient struct {
	focusedPane    *herdrclient.PaneInfo
	focusedPaneErr error

	paneRunErr       error
	paneSendInputErr error

	// calls records every final injection invocation, in order, so a test can
	// assert PaneRun was called exactly once (or not at all).
	calls []string
}

func (f *fakeHerdrClient) FocusedPane(context.Context, string) (*herdrclient.PaneInfo, error) {
	if f.focusedPaneErr != nil {
		return nil, f.focusedPaneErr
	}
	return f.focusedPane, nil
}

func (f *fakeHerdrClient) PaneRun(_ context.Context, session, pane, text string) error {
	f.calls = append(f.calls, fmt.Sprintf("%s/%s: %s", session, pane, text))
	return f.paneRunErr
}

func (f *fakeHerdrClient) PaneSendInput(_ context.Context, session, pane, text, key string) error {
	f.calls = append(f.calls, fmt.Sprintf("%s/%s [%s]: %s", session, pane, key, text))
	return f.paneSendInputErr
}

func (f *fakeHerdrClient) PaneRead(context.Context, string, string, int) (string, error) {
	return "", nil
}

// buildMultipart assembles a /send request body: a JSON template field, an
// optional session field, and named file parts (fieldName -> content).
// Passing a template with no "session" field lets a test exercise the
// fallback-to-"default" path.
func buildMultipart(t *testing.T, tmpl *artifact.Template, session string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	return buildMultipartWithSubmit(t, tmpl, session, "", files)
}

func buildMultipartWithSubmit(t *testing.T, tmpl *artifact.Template, session, submit string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	if tmpl != nil {
		tw, err := w.CreateFormField("template")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(tw).Encode(tmpl); err != nil {
			t.Fatal(err)
		}
	}
	if session != "" {
		sw, err := w.CreateFormField("session")
		if err != nil {
			t.Fatal(err)
		}
		sw.Write([]byte(session))
	}
	if submit != "" {
		kw, err := w.CreateFormField("submitKey")
		if err != nil {
			t.Fatal(err)
		}
		kw.Write([]byte(submit))
	}
	for field, content := range files {
		fw, err := w.CreateFormFile(field, field+".txt")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
}

func TestSend_HappyPath_SavesResolvesAndInjects(t *testing.T) {
	dir := t.TempDir()
	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, dir, silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{
		{Text: "imgcat "},
		{File: "file1"},
	}}
	body, ctype := buildMultipart(t, tmpl, "default", map[string]string{"file1": "png-bytes"})

	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(herdr.calls) != 1 {
		t.Fatalf("expected exactly 1 PaneRun call, got %d: %v", len(herdr.calls), herdr.calls)
	}
	if !strings.HasPrefix(herdr.calls[0], "default/w1:p1: imgcat ") {
		t.Fatalf("unexpected injected text: %q", herdr.calls[0])
	}
	// The client must never see a /tmp path: assert the resolved path
	// landed in dir (staged), not in the response body.
	if strings.Contains(rec.Body.String(), dir) {
		t.Fatalf("response leaked a staging path: %s", rec.Body.String())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly 1 saved file in %s, got %v (err %v)", dir, entries, err)
	}
}

func TestSend_ModifiedSubmit_UsesOneAtomicSendInput(t *testing.T) {
	for _, key := range []string{"ctrl-enter", "alt-enter"} {
		t.Run(key, func(t *testing.T) {
			herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
			h := newSendHandler(herdr, t.TempDir(), silentLogger())
			tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "hello"}}}
			body, ctype := buildMultipartWithSubmit(t, tmpl, "default", key, nil)
			req := httptest.NewRequest(http.MethodPost, "/send", body)
			req.Header.Set("Content-Type", ctype)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			herdrKey := map[string]string{"ctrl-enter": "ctrl+enter", "alt-enter": "alt+enter"}[key]
			want := fmt.Sprintf("default/w1:p1 [%s]: hello", herdrKey)
			if len(herdr.calls) != 1 || herdr.calls[0] != want {
				t.Fatalf("expected one atomic send_input %q, got %v", want, herdr.calls)
			}
		})
	}
}

func TestSend_InvalidSubmitKey_Returns400BeforeInject(t *testing.T) {
	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, t.TempDir(), silentLogger())
	tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "hello"}}}
	body, ctype := buildMultipartWithSubmit(t, tmpl, "default", "shift-enter", nil)
	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || len(herdr.calls) != 0 {
		t.Fatalf("expected 400 and no inject, got %d and %v", rec.Code, herdr.calls)
	}
}

func TestSend_SaveFailure_AbortsBeforeInject(t *testing.T) {
	// Point the staging dir at a path that cannot be written to (a file,
	// not a directory) so SaveFile's os.OpenFile fails deterministically.
	base := t.TempDir()
	blockedDir := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(blockedDir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, blockedDir, silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{{File: "file1"}}}
	body, ctype := buildMultipart(t, tmpl, "default", map[string]string{"file1": "data"})

	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on save I/O failure, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(herdr.calls) != 0 {
		t.Fatalf("expected PaneRun never called after a save failure, got %v", herdr.calls)
	}
	assertErrorResponse(t, rec)
}

func TestSend_InjectFailure_ReturnsErrorAndCorrelationHeader(t *testing.T) {
	dir := t.TempDir()
	herdr := &fakeHerdrClient{
		focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"},
		paneRunErr:  fmt.Errorf("herdr command failed: pane w1:p1 not found"),
	}
	h := newSendHandler(herdr, dir, silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "echo hi"}}}
	body, ctype := buildMultipart(t, tmpl, "default", nil)

	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	req = withCorrelationID(req, "req-test-123")
	rec := httptest.NewRecorder()

	var h2 http.Handler = h
	h2 = withCorrelation(h2)
	h2.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on inject failure, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get(correlation.HeaderRequestID); got != "req-test-123" {
		t.Fatalf("expected correlation id echoed on error response, got %q", got)
	}
	if !strings.Contains(rec.Body.String(), "pane w1:p1 not found") {
		t.Fatalf("expected herdr stderr quoted in error body, got: %s", rec.Body.String())
	}
}

func TestSend_MalformedMultipart_Returns400(t *testing.T) {
	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, t.TempDir(), silentLogger())

	req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader("not multipart at all"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed multipart, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(herdr.calls) != 0 {
		t.Fatalf("expected no inject on malformed request, got %v", herdr.calls)
	}
}

func TestSend_MissingTemplateField_Returns400(t *testing.T) {
	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, t.TempDir(), silentLogger())

	body, ctype := buildMultipart(t, nil, "default", nil)
	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing template field, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSend_BadSession_Returns400(t *testing.T) {
	herdr := &fakeHerdrClient{focusedPaneErr: fmt.Errorf("herdr command failed: session not found")}
	h := newSendHandler(herdr, t.TempDir(), silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "echo hi"}}}
	body, ctype := buildMultipart(t, tmpl, "no-such-session", nil)
	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad session, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(herdr.calls) != 0 {
		t.Fatalf("expected no inject after focused-pane failure, got %v", herdr.calls)
	}
}

func TestSend_HerdrUnreachable_Returns500WithMessage(t *testing.T) {
	herdr := &fakeHerdrClient{focusedPaneErr: fmt.Errorf("%w: herdr binary not found on PATH", herdrclient.ErrUnreachable)}
	h := newSendHandler(herdr, t.TempDir(), silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "echo hi"}}}
	body, ctype := buildMultipart(t, tmpl, "default", nil)
	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for herdr unreachable, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "not reachable") {
		t.Fatalf("expected graceful 'not reachable' message, got: %s", rec.Body.String())
	}
}

func TestSend_SessionFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	herdr := &fakeHerdrClient{focusedPane: &herdrclient.PaneInfo{PaneID: "w1:p1"}}
	h := newSendHandler(herdr, dir, silentLogger())

	tmpl := &artifact.Template{Segments: []artifact.Segment{{Text: "echo hi"}}}
	// No session field at all.
	body, ctype := buildMultipart(t, tmpl, "", nil)
	req := httptest.NewRequest(http.MethodPost, "/send", body)
	req.Header.Set("Content-Type", ctype)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(herdr.calls) != 1 || !strings.HasPrefix(herdr.calls[0], "default/") {
		t.Fatalf("expected injection scoped to fallback session %q, got %v", "default", herdr.calls)
	}
}

func TestClientlog_AcceptsErrorReport(t *testing.T) {
	h := newClientlogHandler(silentLogger())

	body := strings.NewReader(`{"message":"boom","ref_id":"req-abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/clientlog", body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestClientlog_MalformedBody_Returns400(t *testing.T) {
	h := newClientlogHandler(silentLogger())

	req := httptest.NewRequest(http.MethodPost, "/clientlog", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func withCorrelationID(r *http.Request, id string) *http.Request {
	r.Header.Set(correlation.HeaderRequestID, id)
	return r
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var resp sendResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("expected JSON error body, got: %s (%v)", rec.Body.String(), err)
	}
	if resp.OK {
		t.Fatalf("expected ok=false, got: %s", rec.Body.String())
	}
	if resp.Error == "" {
		t.Fatalf("expected non-empty error message, got: %s", rec.Body.String())
	}
}
