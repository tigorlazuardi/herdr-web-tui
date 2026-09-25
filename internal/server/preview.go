package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/tigorlazuardi/herdr-web-tui/internal/herdrclient"
)

// panePreviewResponse is the raw visible terminal snapshot. Text stays plain
// text so clients can select and copy it without HTML conversion.
type panePreviewResponse struct {
	Text string `json:"text"`
}

// previewHandler resolves the session's currently focused pane and returns a
// fresh visible snapshot. r.Context reaches both Herdr commands, so closing a
// browser preview aborts any command still in flight.
type previewHandler struct {
	herdr  herdrclient.HerdrClient
	logger *slog.Logger
}

func newPreviewHandler(herdr herdrclient.HerdrClient, logger *slog.Logger) *previewHandler {
	return &previewHandler{herdr: herdr, logger: logger}
}

func (h *previewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeSendError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	session := sanitizeSession(strings.TrimPrefix(r.URL.Path, "/api/pane-preview/"))
	// Follow the operator's machine selection so the preview shows the pane
	// the browser is actually rendering (same routing rationale as /send).
	profile, err := h.herdr.ActiveMachine(r.Context())
	if err != nil {
		h.logger.WarnContext(r.Context(), "preview: machine selection unresolved; routing to Local", slog.String("error", err.Error()))
		profile = herdrclient.MachineProfile{}
	}
	routed := h.herdr.For(profile.ID)
	start := time.Now()
	pane, err := routed.FocusedPane(r.Context(), session)
	if err != nil {
		h.logger.WarnContext(r.Context(), "preview: focused-pane resolution failed", slog.String("session", session), slog.String("error", err.Error()))
		if errors.Is(err, herdrclient.ErrUnreachable) {
			writeSendError(w, http.StatusInternalServerError, "preview unavailable: "+err.Error())
			return
		}
		writeSendError(w, http.StatusBadRequest, "bad session "+session+": "+err.Error())
		return
	}

	text, err := routed.PaneRead(r.Context(), session, pane.PaneID, 0)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "preview: pane read failed", slog.String("session", session), slog.String("pane", pane.PaneID), slog.Duration("duration", time.Since(start)), slog.String("error", err.Error()))
		writeSendError(w, http.StatusInternalServerError, "preview failed: "+err.Error())
		return
	}

	h.logger.InfoContext(r.Context(), "preview: pane read ok", slog.String("session", session), slog.String("pane", pane.PaneID), slog.Duration("duration", time.Since(start)))
	writeJSON(w, http.StatusOK, panePreviewResponse{Text: text})
}
