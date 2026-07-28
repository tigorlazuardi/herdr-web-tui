package push

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
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

type agentEvent struct {
	Event string         `json:"event"`
	Data  agentEventData `json:"data"`
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

type snapshotResponse struct {
	Result struct {
		Snapshot struct {
			Panes []struct {
				PaneID       string  `json:"pane_id"`
				AgentStatus  string  `json:"agent_status"`
				Agent        *string `json:"agent"`
				DisplayAgent *string `json:"display_agent"`
			} `json:"panes"`
		} `json:"snapshot"`
	} `json:"result"`
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

// EventTransition returns a notification only for real transitions into done or blocked.
func EventTransition(states map[string]string, e agentEvent) (string, string, string, bool) {
	old, seen := states[e.Data.PaneID]
	states[e.Data.PaneID] = e.Data.AgentStatus
	if !seen || old == e.Data.AgentStatus || !validPaneID(e.Data.PaneID) || (e.Data.AgentStatus != "done" && e.Data.AgentStatus != "blocked") {
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

// runEventsOnce owns one snapshot connection and one event-stream connection.
func (s *Service) runEventsOnce(ctx context.Context, path string) error {
	rawSnapshot, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrap(err, "connect Herdr snapshot socket")
	}
	snapshot := newEventConn(ctx, rawSnapshot)
	// Schema v16 accepts one initial request per connection. Snapshot seeds pane-specific subscriptions.
	if _, err = io.WriteString(snapshot, "{\"id\":\"push-snapshot\",\"method\":\"session.snapshot\",\"params\":{}}\n"); err != nil {
		_ = snapshot.Close()
		return errors.Wrap(err, "write Herdr snapshot request")
	}
	scan := bufio.NewScanner(snapshot)
	scan.Buffer(make([]byte, 4096), 1<<20)
	if !scan.Scan() {
		scanErr := scan.Err()
		_ = snapshot.Close()
		if scanErr != nil {
			return errors.Wrap(scanErr, "read Herdr snapshot response")
		}
		return errors.New("Herdr snapshot response missing")
	}
	var snap snapshotResponse
	if err = json.Unmarshal(scan.Bytes(), &snap); err != nil {
		_ = snapshot.Close()
		return errors.Wrap(err, "decode Herdr snapshot")
	}
	if err = snapshot.Close(); err != nil {
		return errors.Wrap(err, "close Herdr snapshot socket")
	}

	rawEvents, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrap(err, "connect Herdr subscription socket")
	}
	events := newEventConn(ctx, rawEvents)
	defer events.Close()
	states := map[string]string{}
	subs := make([]map[string]string, 0, len(snap.Result.Snapshot.Panes)+1)
	for _, p := range snap.Result.Snapshot.Panes {
		states[p.PaneID] = p.AgentStatus
		subs = append(subs, map[string]string{"type": "pane.agent_status_changed", "pane_id": p.PaneID})
	}
	// Pane status subscriptions require pane_id. Reconnect from a fresh snapshot when pane set grows.
	subs = append(subs, map[string]string{"type": "pane.created"})
	req := map[string]any{"id": "push-events", "method": "events.subscribe", "params": map[string]any{"subscriptions": subs}}
	if err = json.NewEncoder(events).Encode(req); err != nil {
		return errors.Wrap(err, "write Herdr subscription request")
	}
	scan = bufio.NewScanner(events)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var e agentEvent
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			continue
		}
		// Herdr 0.7.4 replays existing pane_created records when a subscription starts.
		// Reconnect only for a pane absent from the snapshot that seeded states.
		if e.Event == "pane.created" || e.Event == "pane_created" {
			if paneCreationChangesSet(states, e) {
				return errPaneSetChanged
			}
			continue
		}
		if e.Event != "pane.agent_status_changed" {
			continue
		}
		opctx, span := otel.Tracer("herdr-web-tui/push").Start(ctx, "agent event handling")
		name, state, paneID, ok := EventTransition(states, e)
		outcome, errorKind := "ignored", "none"
		if ok {
			outcome = "success"
			if err := s.Notify(opctx, name, state, paneID); err != nil {
				outcome, errorKind = "failure", "dispatch"
				span.RecordError(errors.New("agent event push dispatch failed"))
				span.SetStatus(codes.Error, "push dispatch failed")
				s.log.ErrorContext(opctx, "push dispatch failed", "error", err, "agent.name", "<redacted>", "push.endpoint", "<redacted>")
			}
		}
		span.SetAttributes(attribute.String("event.type", state), attribute.String("outcome", outcome), attribute.String("status_class", "none"), attribute.String("error.kind", errorKind), attribute.String("agent.name", "<redacted>"), attribute.String("pane.id", "<redacted>"))
		span.End()
	}
	if err := scan.Err(); err != nil {
		return errors.Wrap(err, "read Herdr subscription events")
	}
	return errors.New("Herdr subscription event stream closed")
}
