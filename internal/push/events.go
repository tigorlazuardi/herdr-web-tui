package push

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type agentEvent struct {
	Event string `json:"event"`
	Data  struct {
		PaneID       string  `json:"pane_id"`
		AgentStatus  string  `json:"agent_status"`
		Agent        *string `json:"agent"`
		DisplayAgent *string `json:"display_agent"`
	} `json:"data"`
}

var errPaneSetChanged = errors.New("Herdr pane set changed")

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

// runEventsOnce owns one socket attempt; its cancellation hook is stopped when that attempt ends.
func (s *Service) runEventsOnce(ctx context.Context, path string) error {
	c, err := s.dial(ctx, "unix", path)
	if err != nil {
		return errors.Wrap(err, "connect Herdr socket")
	}
	defer c.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stopClose()
	// Schema v16: newline-delimited request; pane_id is required, so snapshot seeds one subscription per current pane.
	if _, err = io.WriteString(c, "{\"id\":\"push-snapshot\",\"method\":\"session.snapshot\",\"params\":{}}\n"); err != nil {
		return err
	}
	scan := bufio.NewScanner(c)
	scan.Buffer(make([]byte, 4096), 1<<20)
	if !scan.Scan() {
		return errors.New("Herdr snapshot response missing")
	}
	var snap snapshotResponse
	if err = json.Unmarshal(scan.Bytes(), &snap); err != nil {
		return errors.Wrap(err, "decode Herdr snapshot")
	}
	states := map[string]string{}
	subs := make([]map[string]string, 0, len(snap.Result.Snapshot.Panes)+1)
	for _, p := range snap.Result.Snapshot.Panes {
		states[p.PaneID] = p.AgentStatus
		subs = append(subs, map[string]string{"type": "pane.agent_status_changed", "pane_id": p.PaneID})
	}
	// Pane status subscriptions require pane_id. Reconnect from a fresh snapshot when pane set grows.
	subs = append(subs, map[string]string{"type": "pane.created"})
	req := map[string]any{"id": "push-events", "method": "events.subscribe", "params": map[string]any{"subscriptions": subs}}
	if err = json.NewEncoder(c).Encode(req); err != nil {
		return errors.Wrap(err, "subscribe Herdr events")
	}
	for scan.Scan() {
		var e agentEvent
		if json.Unmarshal(scan.Bytes(), &e) != nil {
			continue
		}
		// Herdr protocol 16 uses dotted subscription names; accept generic event spelling too.
		if e.Event == "pane.created" || e.Event == "pane_created" {
			return errPaneSetChanged
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
		return errors.Wrap(err, "read Herdr events")
	}
	return errors.New("Herdr event stream closed")
}
