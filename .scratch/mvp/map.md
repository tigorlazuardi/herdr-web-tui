# Web TUI MVP — map

`wayfinder:map` · tracker: local-markdown · effort slug: `mvp`

## Destination

A **working MVP**: a web app, co-located with the Herdr server, that renders **one
attached Herdr session** in the browser over **HTTP + WebSocket**, plus **artifact
upload** — browser uploads a file (multipart) to `/tmp/<ns>/<file>` as a blob (extension
kept as a type hint), then **injects that path into the focused pane's prompt** via
Herdr send-text / send-keys. Security/auth/TLS stay at gateway; app is single-owner.
**This map carries execution** (Notes override) — tickets build the MVP, not just plan it.

## Notes

- **Domain**: Herdr — tmux-like, agent-aware terminal multiplexer. Background server owns
  real terminals (panes); clients attach to render. Guide: https://herdr.dev/agent-guide.md
- **Control surface**: local unix socket API at `$HERDR_SOCKET_PATH` (+ `herdr` CLI wrappers).
  Methods incl. `pane.read`, `pane.send_input` (text/keys), workspace/tab/pane mgmt, live
  event subscriptions (agent state, pane changes). Docs: https://herdr.dev/docs/socket-api/ ,
  https://herdr.dev/docs/cli-reference/
- **Placement**: web server runs on the same machine as the Herdr server (needs local socket +
  writes artifacts to that machine's `/tmp`). Browser is remote **through nginx** (nginx owns
  TLS/auth — not this project).
- **Execution override**: this map builds the MVP. Produce running code, not only decisions.
- **Skills to consult per session**: `/grilling`, `/domain-modeling`, `/prototype`,
  `frontend-design` (web UI), `research`. Frontend work → load `frontend-design` first.

## Decisions so far

<!-- one line per closed ticket -->

## Not yet specified

<!-- in-scope fog; graduates to tickets as the frontier advances -->

- **Rendering fidelity** — how much of Herdr's mouse-first UI must survive in the browser
  (mouse click/drag, split-border drag, right-click menus, drag-select copy, resize/reflow,
  colors). Depends on the base-app / transport choice.
- **Artifact lifecycle** — `/tmp/<ns>` namespace scheme, cleanup, size/type limits.
- **Injection target** — which pane receives the path (always the focused pane, or
  user-selected), and whether the promptbox sends Enter or only pastes the path.
- **Reconnect/detach behavior** in the browser (client drops, reattach).
- **Deployment / run story** — how the co-located server is launched behind nginx.

## Out of scope

- **Security / auth / TLS** — handled by the gateway (nginx), not this app.
- **Multi-session concurrency** — MVP is one session, one browser. A later effort.
