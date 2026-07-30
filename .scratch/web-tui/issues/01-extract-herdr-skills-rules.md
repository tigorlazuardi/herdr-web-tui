# Extract Herdr skills + rules for this repo

Type: research
Status: resolved
Blocked by: —

## Question

From the Herdr docs (agent-guide.md, socket API, CLI reference, agent SKILL.md), what durable knowledge should be captured as `.pi/skills` / `.pi/rules` for this repo so every future session inherits the Herdr concept model and control surface?

Capture at least:
- Concept model (session / workspace / tab / pane / agent states / modes).
- Control surface: socket API (`HERDR_SOCKET_PATH`), key methods — `pane.send_input` (send text/keys), `pane.read`, `pane.list`, event subscriptions — and CLI wrappers.
- The remote clipboard gap (SSH-first can't read local clipboard) that motivates this project.
- Rules-for-agents (don't invent keybindings/flags; not tmux).

Produce the markdown skill/rule content as a linked asset. NOTE: writing `.pi/rules` / `.pi/skills` in a normal session needs explicit user OK — confirm before writing files. User already requested this extraction.

## Answer

Extracted two artifacts (user said "Gas" = consent to write):

- **Skill** `.pi/skills/herdr/SKILL.md` — Herdr concept model (session/workspace/tab/pane/agent/modes) + full control surface: CLI (`pane current|read|send-text|send-keys|run`, `agent list|send|read|wait|focus`, workspace/tab lifecycle), socket API (`HERDR_SOCKET_PATH`, methods mirror CLI, event subscriptions), integrations for authoritative agent state, and the remote/clipboard gap that motivates a web layer. Intent-triggered.
- **Rule** `.pi/rules/herdr-web-tui.md` (paths `**/*`) — this repo's constraints: thin layer only (reuse ttyd + socket API, no custom renderer), security = gateway's job, not-a-plugin, not-tmux; atomic promptbox contract (flat `/tmp/<prefix>-<server-uid>/<uuid>[.ext]`, focused-pane inject, prefer `pane run`/`agent send`).

Key facts surfaced for later tickets:
- `herdr pane current` resolves the focused pane; socket: omit `pane_id` → active focused pane.
- `herdr pane run <pane_id> <text>` submits **text + Enter atomically** — ideal for the atomic promptbox submit.
- `herdr agent send <target> <text>` targets the detected agent's pane directly (alternative to pane-focused inject).
- `herdr agent wait <target> --status ...` blocks on agent state — useful for orchestration/UX.
- Confirmed: browser client = SSH-first case, no local clipboard bridge — the promptbox fills exactly this gap.
