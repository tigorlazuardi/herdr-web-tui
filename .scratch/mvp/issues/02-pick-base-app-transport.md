# Pick base web-tui app & transport architecture

Type: research
Status: open
Blocked by:

## Question

What is the base app + transport for rendering the Herdr TUI in the browser? Two families:

1. **Pty streamer** (ttyd / gotty / wetty / xterm.js + own ws bridge) — run `herdr` inside a
   pty, stream raw bytes to xterm.js over websocket. Artifact injection = write the path into
   the same pty (send-text). Herdr's mouse-first UI rides along as terminal mouse sequences.
2. **Herdr-native socket bridge** — a custom server that speaks the Herdr socket API
   (`pane.read` + event subscriptions to render, `pane.send_input` to inject), exposing it over
   http+ws to the browser.

Decide the family and the concrete base app. Compare on: rendering fidelity (mouse, resize,
colors), effort to build, how cleanly artifact-path injection fits, co-location constraints.

Deliverable: a short decision doc (linked asset) naming the chosen base + transport and why.
This is the biggest fork — it blocks the build tickets.
