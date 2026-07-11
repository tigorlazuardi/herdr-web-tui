---
name: herdr
description: Concept model and control surface for Herdr, the tmux-like agent-aware terminal multiplexer. Load when driving Herdr programmatically — rendering/streaming panes, reading pane output, sending text/keys into a pane or agent, resolving the focused pane, subscribing to agent/pane events, or building a layer on top of Herdr (e.g. this web-tui).
---

# Herdr control surface

Herdr = terminal workspace manager for AI coding agents. Like tmux: a background
**server** owns real terminal processes; **clients** attach to render. Unlike tmux:
mouse-first (whole UI clickable) and agent-aware (detects coding agents in panes,
reports state). Not tmux — never give tmux commands/config for Herdr.

Docs: agent guide https://herdr.dev/agent-guide.md · socket API
https://herdr.dev/docs/socket-api/ · CLI https://herdr.dev/docs/cli-reference/ ·
agent SKILL https://raw.githubusercontent.com/ogulcancelik/herdr/master/SKILL.md
Do not invent keybindings, config keys, or CLI flags — verify against docs.

## Concept model (teach in this order)

- **Session** — persistent background server namespace. `herdr` attaches to default; named sessions (`herdr session attach <name>`) are separate runtime namespaces.
- **Workspace** — project-level container (one per repo/task). Owns tabs+panes. Sidebar rolls agent states up per workspace.
- **Tab** — a layout inside a workspace (e.g. `agents`, `logs`).
- **Pane** — a real terminal. Splittable right/down. Survives client detach.
- **Agent** — a process Herdr recognizes in a pane. States: `working`, `blocked`, `done`, `idle`, `unknown`.
- **Modes** — terminal mode (keys → focused pane); prefix mode (`ctrl+b` then one key → one Herdr command); navigate mode.

`HERDR_ENV=1` means you are running inside a Herdr pane. Nested launches are blocked.

## Control surface — three layers, one control plane

1. **Agent skill** — teaches an agent to drive Herdr from inside a pane.
2. **CLI wrappers** — shell scripts, orchestration, debugging. Start here.
3. **Raw socket API** — direct request/response + long-lived event subscriptions. Unix socket at `HERDR_SOCKET_PATH`. Print schema: `herdr api schema --json`.

The CLI and socket API share the same methods. CLI is the easy entry; use raw
socket for custom tools and event subscribers.

### Panes — read & send input (core for automation)

```
herdr pane current [--pane ID|--current]          # resolve the current/focused pane id
herdr pane list [--workspace <id>]
herdr pane get <pane_id>                           # includes scroll (offset_from_bottom==0 => at bottom)
herdr pane read <pane_id> [--source visible|recent|recent-unwrapped|detection] [--lines N] [--ansi]
herdr pane send-text <pane_id> <text>              # text only, no submit
herdr pane send-keys <pane_id> <key> [key ...]     # keys: enter tab esc backspace left right shift+tab f1 ...
herdr pane run <pane_id> <command>                 # text + Enter ATOMICALLY — prefer over send-text + send-keys enter
```

For `--current`, Herdr uses the calling pane's `HERDR_PANE_ID` when run inside a pane.
Socket method names mirror these (`pane.read`, `pane.send_input`, etc.); omitting
`pane_id` targets the server's active focused pane.

**Focus is server-wide / shared across clients.** `herdr pane current` returns the
single globally-focused pane, and *any* attached client navigating (a browser, an SSH
client) changes it for everyone. Verified live: a browser client switching workspace
moved `pane current` to that pane. Useful (a UI that injects into "the focused pane"
hits whatever the user is looking at) but multiple concurrent clients contend for one
focus. For server-side automation that must not follow the user, target an explicit
`pane_id`.

**Terminal size = "last active client wins."** Multiple clients attached to one session
share each pane's terminal dimensions; Herdr sizes a pane to the cols/rows of the
last-active client. Two clients of different sizes (e.g. desktop + phone) therefore
contend and the view resizes on switch — expected for a shared multiplexer, seamless
with a single active viewer. This is also how the responsive/mobile layout follows the
active client's width.

### Agents — target the detected agent directly

```
herdr agent list
herdr agent get <target>
herdr agent read <target> [--source ...] [--lines N] [--ansi]
herdr agent send <target> <text>                  # send text to the agent's pane
herdr agent focus <target>
herdr agent wait <target> --status idle|working|blocked|unknown [--timeout MS]
herdr agent explain <target> [--json]              # why the detector classified it so
```

`<target>` = terminal id, unique agent name, detected/reported label, or legacy pane
id. `agent wait` blocks until a state — useful for orchestration.

### Integrations & state reporting

- `herdr integration install <name>` (e.g. claude) gives Herdr **authoritative** agent state instead of screen detection. Status: `herdr integration status`.
- Report state programmatically: `herdr pane report-agent <pane_id> --source ID --agent LABEL --state ...`; display-only metadata: `herdr pane report-metadata ...`.

### Workspaces / tabs / lifecycle

`herdr workspace create|list|focus|rename|close`, `herdr tab create|list|focus|...`,
`herdr pane split|swap|move|zoom|resize|close`. Worktree helpers:
`herdr worktree create|open|remove` (Git checkouts as workspaces).

### Events (socket API)

Subscribe for live updates: pane create/close/move, agent state changes, workspace/tab
lifecycle, `worktree.*`. This is how you stream state to a UI without polling.

## Responsive / mobile layout

Herdr's TUI adapts to narrow screens. The mobile single-column layout is chosen
purely from **terminal width in columns**: `[ui] mobile_width_threshold` (default
`64`) is the width at or below which Herdr goes mobile. This travels through the
normal TTY window size (cols×rows via SIGWINCH) — a client just needs to report
accurate columns for the device; there is no separate "mobile" API. Raise the
threshold for foldables/tablets.

## Remote & clipboard gap (why a web layer exists)

- `herdr --remote <host>` = thin **local** client → can bridge local desktop features like **image clipboard paste** to the remote server.
- SSH-first (`ssh host` then `herdr`) or a **browser client** = server-side only → **cannot** read the local clipboard. A web frontend must supply its own artifact/file path in mechanism (upload → stage → inject path into the pane).

## Diagnosis

- Agent wrong state: `herdr agent list`, `herdr agent explain <target> --json`. Install the integration for authoritative state.
- Logs: `~/.config/herdr/herdr.log`, `-client.log`, `-server.log`. Runtime: `herdr status[ server|client]`.
- Keybinding does nothing: outer terminal/DE consumed the chord — see https://herdr.dev/docs/keyboard/.
- Config: `~/.config/herdr/config.toml`; `herdr --default-config`; apply live with `herdr server reload-config`.
