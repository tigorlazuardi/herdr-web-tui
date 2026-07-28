package push

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type agentEventPane struct {
	PaneID string `json:"pane_id"`
}

type agentEventData struct {
	PaneID       string          `json:"pane_id"`
	AgentStatus  string          `json:"agent_status"`
	Agent        *string         `json:"agent"`
	DisplayAgent *string         `json:"display_agent"`
	Pane         *agentEventPane `json:"pane"`
}

type subscriptionResult struct {
	Type string `json:"type"`
}

type agentEvent struct {
	ID     string              `json:"id"`
	Result *subscriptionResult `json:"result"`
	Event  string              `json:"event"`
	Data   agentEventData      `json:"data"`
}

var errPaneSetChanged = errors.New("Herdr pane set changed")

type eventConn struct {
	net.Conn
	closeOnce sync.Once
	stopClose func() bool
}

// newEventConn owns conn and registers context cancellation to close it.
func newEventConn(ctx context.Context, conn net.Conn) *eventConn {
	owned := &eventConn{Conn: conn}
	owned.stopClose = context.AfterFunc(ctx, func() { _ = owned.close() })
	return owned
}

// Close stops cancellation ownership before closing the connection explicitly.
func (c *eventConn) Close() error {
	c.stopClose()
	return c.close()
}

// close uses sync.Once so cancellation and explicit cleanup cannot double-close the connection.
func (c *eventConn) close() error {
	var err error
	c.closeOnce.Do(func() { err = c.Conn.Close() })
	return err
}

// paneCreationChangesSet ignores Herdr's initial replay of snapshot panes and detects genuinely new panes.
func paneCreationChangesSet(states map[string]string, e agentEvent) bool {
	if e.Event != "pane.created" && e.Event != "pane_created" {
		return false
	}
	if e.Data.Pane == nil || !validPaneID(e.Data.Pane.PaneID) {
		return false
	}
	_, exists := states[e.Data.Pane.PaneID]
	return !exists
}

// paneExists verifies an absent pane_created replay against current Herdr state.
func (s *Service) paneExists(ctx context.Context, path, paneID string) (bool, error) {
	var current focusSnapshotResult
	request := herdrRequest{ID: "push-pane-check", Method: "session.snapshot", Params: map[string]any{}}
	if err := s.herdrRoundTrip(ctx, path, "pane replay check", request, &current); err != nil {
		return false, err
	}
	if !validSessionSnapshot(current) {
		return false, errors.Wrap(errHerdrProtocol, "invalid pane replay snapshot")
	}
	for _, pane := range current.Snapshot.Panes {
		if pane.PaneID == paneID {
			return true, nil
		}
	}
	return false, nil
}

// EventTransition returns a notification only for real transitions into done or blocked.
func EventTransition(states map[string]string, e agentEvent) (string, string, string, bool) {
	old, seen := states[e.Data.PaneID]
	if !seen || !validPaneID(e.Data.PaneID) {
		return "", "", "", false
	}
	states[e.Data.PaneID] = e.Data.AgentStatus
	if old == e.Data.AgentStatus || (e.Data.AgentStatus != "done" && e.Data.AgentStatus != "blocked") {
		return "", "", "", false
	}
	name := "Agent"
	if e.Data.DisplayAgent != nil && *e.Data.DisplayAgent != "" {
		name = *e.Data.DisplayAgent
	} else if e.Data.Agent != nil && *e.Data.Agent != "" {
		name = *e.Data.Agent
	}
	return name, e.Data.AgentStatus, e.Data.PaneID, true
}

// RunEvents subscribes directly to verified Herdr socket events until cancellation.
func (s *Service) RunEvents(ctx context.Context, socketPath string) error {
	if !s.cfg.Enabled() {
		<-ctx.Done()
		return ctx.Err()
	}
	if socketPath == "" {
		socketPath = os.Getenv("HERDR_SOCKET_PATH")
	}
	if socketPath == "" {
		return errors.New("HERDR_SOCKET_PATH is required for Web Push events")
	}
	for {
		err := s.runEventsOnce(ctx, socketPath)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, errPaneSetChanged) {
			continue
		}
		s.log.ErrorContext(ctx, "Herdr event subscription failed", "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// runEventsOnce owns one validated snapshot round trip and one event-stream connection.
func (s *Service) runEventsOnce(ctx context.Context, path string) error {
	var snap focusSnapshotResult
	snapshotRequest := herdrRequest{ID: "push-snapshot", Method: "session.snapshot", Params: map[string]any{}}
	if err := s.herdrRoundTrip(ctx, path, "event snapshot", snapshotRequest, &snap); err != nil {
		return err
	}
	if !validSessionSnapshot(snap) {
		return errors.Wrap(errHerdrProtocol, "event snapshot missing required shape")
	}

	rawEvents, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrap(err, "connect Herdr subscription socket")
	}
	events := newEventConn(ctx, rawEvents)
	defer events.Close()
	states := map[string]string{}
	subs := make([]map[string]string, 0, len(snap.Snapshot.Panes)+1)
	for _, p := range snap.Snapshot.Panes {
		states[p.PaneID] = p.AgentStatus
		subs = append(subs, map[string]string{"type": "pane.agent_status_changed", "pane_id": p.PaneID})
	}
	// Pane status subscriptions require pane_id. Reconnect from a fresh snapshot when pane set grows.
	subs = append(subs, map[string]string{"type": "pane.created"})
	req := map[string]any{"id": "push-events", "method": "events.subscribe", "params": map[string]any{"subscriptions": subs}}
	if err = json.NewEncoder(events).Encode(req); err != nil {
		return errors.Wrap(err, "write Herdr subscription request")
	}
	scan := bufio.NewScanner(events)
	scan.Buffer(make([]byte, 4096), 1<<20)
	started := false
	for scan.Scan() {
		var e agentEvent
		if err := json.Unmarshal(scan.Bytes(), &e); err != nil {
			s.log.WarnContext(ctx, "Herdr event frame ignored", "reason", "malformed")
			continue
		}
		if e.ID != "" || e.Result != nil {
			if e.ID != "push-events" || e.Result == nil || e.Result.Type != "subscription_started" {
				return errors.Wrap(errHerdrProtocol, "unexpected Herdr subscription response")
			}
			started = true
			s.log.InfoContext(ctx, "Herdr event subscription started", "pane.count", len(states))
			continue
		}
		if !started {
			return errors.Wrap(errHerdrProtocol, "Herdr event received before subscription started")
		}
		// Herdr 0.7.4 replays existing pane_created records when a subscription starts.
		// Reconnect only for a pane absent from the snapshot that seeded states.
		if e.Event == "pane.created" || e.Event == "pane_created" {
			if !paneCreationChangesSet(states, e) {
				s.log.InfoContext(ctx, "Herdr pane creation replay ignored", "reason", "snapshot_pane", "pane.id", "<redacted>")
				continue
			}
			exists, err := s.paneExists(ctx, path, e.Data.Pane.PaneID)
			if err != nil {
				return err
			}
			if !exists {
				s.log.InfoContext(ctx, "Herdr pane creation replay ignored", "reason", "stale_pane", "pane.id", "<redacted>")
				continue
			}
			s.log.InfoContext(ctx, "Herdr pane set changed", "reason", "new_pane", "pane.id", "<redacted>")
			return errPaneSetChanged
		}
		if e.Event != "pane.agent_status_changed" {
			continue
		}
		opctx, span := otel.Tracer("herdr-web-tui/push").Start(ctx, "agent event handling")
		previous, seen := states[e.Data.PaneID]
		eventType := e.Data.AgentStatus
		if eventType != "idle" && eventType != "working" && eventType != "blocked" && eventType != "done" && eventType != "unknown" {
			eventType = "unknown"
		}
		if !seen {
			if validPaneID(e.Data.PaneID) {
				exists, err := s.paneExists(opctx, path, e.Data.PaneID)
				if err != nil {
					span.End()
					return err
				}
				if exists {
					s.log.InfoContext(opctx, "Herdr pane set changed", "reason", "unseeded_event", "pane.id", "<redacted>")
					span.End()
					return errPaneSetChanged
				}
			}
			s.log.InfoContext(opctx, "Herdr agent event handled", "event.type", eventType, "outcome", "ignored", "reason", "unseeded", "agent.name", "<redacted>", "pane.id", "<redacted>")
			span.SetAttributes(attribute.String("event.type", ""), attribute.String("outcome", "ignored"), attribute.String("status_class", "none"), attribute.String("error.kind", "none"), attribute.String("agent.name", "<redacted>"), attribute.String("pane.id", "<redacted>"))
			span.End()
			continue
		}
		name, state, paneID, ok := EventTransition(states, e)
		outcome, errorKind, reason := "ignored", "none", "nonterminal"
		switch {
		case !validPaneID(e.Data.PaneID):
			reason = "invalid_pane"
		case previous == e.Data.AgentStatus:
			reason = "duplicate"
		case ok:
			outcome, reason = "success", "dispatch"
			if err := s.Notify(opctx, name, state, paneID); err != nil {
				outcome, errorKind = "failure", "dispatch"
				span.RecordError(errors.New("agent event push dispatch failed"))
				span.SetStatus(codes.Error, "push dispatch failed")
				s.log.ErrorContext(opctx, "push dispatch failed", "error", err, "agent.name", "<redacted>", "push.endpoint", "<redacted>")
			}
		}
		s.log.InfoContext(opctx, "Herdr agent event handled", "event.type", eventType, "outcome", outcome, "reason", reason, "agent.name", "<redacted>", "pane.id", "<redacted>")
		span.SetAttributes(attribute.String("event.type", state), attribute.String("outcome", outcome), attribute.String("status_class", "none"), attribute.String("error.kind", errorKind), attribute.String("agent.name", "<redacted>"), attribute.String("pane.id", "<redacted>"))
		span.End()
	}
	if err := scan.Err(); err != nil {
		return errors.Wrap(err, "read Herdr subscription events")
	}
	return errors.New("Herdr subscription event stream closed")
}
