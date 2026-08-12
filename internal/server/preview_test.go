package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
)

type previewHerdrClient struct {
	pane       *herdrclient.PaneInfo
	focusedErr error
	text       string
	readErr    error
	session    string
	paneID     string
	ctx        context.Context
}

func (f *previewHerdrClient) FocusedPane(ctx context.Context, session string) (*herdrclient.PaneInfo, error) {
	f.ctx, f.session = ctx, session
	return f.pane, f.focusedErr
}

func (f *previewHerdrClient) PaneRun(context.Context, string, string, string) error { return nil }
func (f *previewHerdrClient) PaneSendInput(context.Context, string, string, string, string) error {
	return nil
}

func (f *previewHerdrClient) PaneRead(ctx context.Context, session, pane string, _ int) (string, error) {
	f.ctx, f.session, f.paneID = ctx, session, pane
	return f.text, f.readErr
}

func TestPreview_FreshVisibleSnapshot_ReturnsRawText(t *testing.T) {
	herdr := &previewHerdrClient{pane: &herdrclient.PaneInfo{PaneID: "w1:p2"}, text: "  # heading\n  | table |\n"}
	h := newPreviewHandler(herdr, silentLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/pane-preview/work", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body panePreviewResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Text != herdr.text || herdr.session != "work" || herdr.paneID != "w1:p2" {
		t.Fatalf("unexpected preview response/call: body=%q session=%q pane=%q", body.Text, herdr.session, herdr.paneID)
	}
}

func TestPreview_InvalidSession_UsesDefault(t *testing.T) {
	herdr := &previewHerdrClient{pane: &herdrclient.PaneInfo{PaneID: "p1"}}
	h := newPreviewHandler(herdr, silentLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/pane-preview/not%20valid", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || herdr.session != defaultSession {
		t.Fatalf("expected default session 200, got status=%d session=%q", rec.Code, herdr.session)
	}
}

func TestPreview_NewHandler_ReturnsCorrelationHeadersOnSuccessAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		herdr *previewHerdrClient
		code  int
		body  string
	}{
		{
			name:  "success",
			herdr: &previewHerdrClient{pane: &herdrclient.PaneInfo{PaneID: "p1"}, text: "snapshot"},
			code:  http.StatusOK,
			body:  "snapshot",
		},
		{
			name:  "pane read failure",
			herdr: &previewHerdrClient{pane: &herdrclient.PaneInfo{PaneID: "p1"}, readErr: errors.New("socket closed")},
			code:  http.StatusInternalServerError,
			body:  "preview failed: socket closed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(testFS(), silentLogger(), tc.herdr, t.TempDir())
			req := httptest.NewRequest(http.MethodGet, "/api/pane-preview/work", nil)
			req.Header.Set(correlation.HeaderRequestID, "preview-req-123")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.code || !strings.Contains(rec.Body.String(), tc.body) {
				t.Fatalf("expected status=%d body containing %q, got status=%d body=%s", tc.code, tc.body, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get(correlation.HeaderRequestID); got != "preview-req-123" {
				t.Fatalf("expected request id on response, got %q", got)
			}
			if got := rec.Header().Get(correlation.HeaderCorrelationID); got != "preview-req-123" {
				t.Fatalf("expected correlation id on response, got %q", got)
			}
		})
	}
}

func TestPreview_RequestContext_ReachesHerdr(t *testing.T) {
	herdr := &previewHerdrClient{pane: &herdrclient.PaneInfo{PaneID: "p1"}}
	h := newPreviewHandler(herdr, silentLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/pane-preview/work", nil).WithContext(ctx)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if herdr.ctx != ctx {
		t.Fatal("expected request context passed to Herdr client")
	}
}
