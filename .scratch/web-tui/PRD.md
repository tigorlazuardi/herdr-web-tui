# PRD: Herdr Web TUI

Status: ready-for-agent

Source: wayfinder map `.scratch/web-tui/map.md` + design doc `docs/design/2026-07-11-herdr-web-tui-spec.mdx`. All decisions in those are authoritative; this PRD is the agent-ready synthesis.

## Problem Statement

I run coding agents inside Herdr (a tmux-like, agent-aware terminal multiplexer) on a
server. I want to reach those sessions from a **browser** over my LAN — including from my
phone — instead of only through an SSH client. The blocker: a browser (like an SSH-first
attach) is server-side only and **cannot read my local clipboard**, so there is no good way
to get a file — a screenshot, a PDF, a log — from my device into an agent's prompt. Today
that means no copy/paste of artifacts at all. On top of that, a raw browser terminal has no
mobile affordances (no modifier keys, broken touch zoom, wrong layout) and no way to trace
what went wrong when something breaks.

## Solution

A thin standalone web service that sits in front of an already-running Herdr server and:

1. **Renders the live Herdr TUI in a browser** over http + websocket, mouse/touch-first, with
   proper mobile behaviour (single-column layout on phones, an on-screen modifier key bar,
   no runaway zoom, installable as a PWA).
2. Adds an **atomic artifact promptbox**: I write a line of text with file attachments placed
   inline (positional placeholders), and on send the files upload to the server, their paths
   are substituted in, and the whole line is typed into the focused pane and submitted in one
   shot — so it works for an agent chatbox *or* any shell command (`imgcat [File 1]`).
3. **Separates concurrent tabs** by URL path → Herdr session, so they do not fight over one view.
4. Is **observable**: structured logs, a correlation id returned to the browser on every error,
   and errors bubbled in full so any failure is trivial to look up.

Security (auth/TLS/limits) is handled by an nginx gateway in front — out of scope here.

## User Stories

1. As a developer, I want to open a URL in my browser and see my live Herdr TUI, so that I can work without an SSH client.
2. As a developer, I want the full Herdr TUI (colors, sidebar, layout) rendered faithfully, so that nothing is lost versus the terminal.
3. As a developer, I want to click/tap panes, tabs, and split borders, so that I can drive Herdr's mouse-first UI in the browser.
4. As a developer, I want right-click to show only Herdr's menu, so that the browser's native context menu doesn't double up.
5. As a mobile user, I want the TUI to switch to Herdr's single-column mobile layout on my phone, so that it's readable without zooming.
6. As a mobile user, I want taps not to trigger runaway page zoom, so that the terminal stays usable without refreshing.
7. As a mobile user, I want tapping the terminal to summon the soft keyboard, so that I can type.
8. As a mobile user, I want an on-screen key bar with Ctrl/Esc/Tab/arrows and sticky modifiers, so that I can send `ctrl+b`, `ctrl+c`, and `esc` on a touch keyboard.
9. As a mobile user, I want the key bar hidden on desktop or when I have a hardware keyboard, so that it doesn't waste space.
10. As a mobile user, I want to install the app to my home screen (PWA, fullscreen), so that the browser chrome doesn't eat my screen height.
11. As a mobile user, I want a stray pull-down not to reload the page, so that I don't lose my place.
12. As a developer, I want the terminal to auto-reconnect when my network drops, so that the pane keeps running and re-renders on reconnect.
13. As a developer, I want to attach a file in a promptbox and place it inline in my text, so that I control exactly where its path lands in the command.
14. As a developer, I want each attachment shown as an atomic pill I can delete with one backspace, so that editing feels like Claude Code's attachments.
15. As a developer, I want to paste a screenshot from my clipboard into the promptbox, so that I can send an image without saving it first.
16. As a developer, I want an image attachment to show a thumbnail and a mime badge, so that I can tell what I'm sending at a glance.
17. As a developer, I want a non-image attachment to show a mime badge (TXT/PDF/PNG/ZIP/…), with a lucide icon fallback and no badge for unknown types, so that the list stays legible.
18. As a developer, I want the text + attachments sent as one atomic bundle that types into the focused pane and submits, so that the agent (or shell) receives a complete command in one action.
19. As a developer, I want the attachment path substituted server-side so I never handle `/tmp` paths, so that the flow stays simple.
20. As a developer, I want the send to be all-or-nothing, so that a failed upload never leaves a half-typed command in my pane.
21. As a developer sending to a plain shell, I want `imgcat [File 1]` to become `imgcat /tmp/…/x.png`, so that the promptbox works beyond agent chatboxes.
22. As a developer, I want the promptbox to inject into whatever pane I last focused, so that I don't need to pick a target.
23. As a developer, I want different browser tabs/paths to map to separate Herdr sessions, so that concurrent views don't contend over focus or sizing.
24. As an operator, I want to run one static binary behind nginx, so that deployment is trivial (no runtime install).
25. As an operator, I want size/auth/TLS enforced by the gateway, so that the app stays thin.
26. As a developer, when something fails I want the full error shown with a copyable correlation id, so that I can grep the server logs directly.
27. As a developer, I want errors to bubble verbatim (this isn't a secrecy-sensitive app), so that I see the real cause (e.g. herdr's stderr).
28. As an operator, I want structured JSON logs (with a human text fallback), so that I can pipe them to an aggregator or read them in dev.
29. As an operator, I want ws/pty/inject lifecycle events logged with the correlation id, so that I can reconstruct one flow end-to-end.
30. As an operator, I want backend errors to identify their source (origin frames, not just a message), so that I can locate the bug despite Go having no native error stacks.
31. As an operator, I want one bad request not to crash the server, so that other sessions keep working.
32. As a developer, I want the app to degrade with a clear message if the Herdr server is unreachable, so that it never just hangs.
33. As a future maintaining coding agent, I want load-bearing doc comments on functions explaining the why/invariants/lifecycle, so that I can safely change the code.
34. As a new user, I want a README and an operator/deploy guide, so that I can run it.
35. As a developer, I want the endpoints documented with I/O examples, so that I understand the contracts.

## Implementation Decisions

**Overall** — three thin parts over an already-running Herdr server: a frontend (browser), a
pty↔ws bridge, and an artifact daemon. Backend is one Go binary that embeds the built frontend.

**Backend (Go)**
- **`context.Context` is the teardown backbone**: every connection/request carries a ctx; goroutines, the pty reader, and spawned processes hang off it, so a dropped ws or ended session cancels everything cleanly. Prefer context-native libraries.
- **`HerdrClient` interface** wraps *all* Herdr interaction (`FocusedPane(ctx, session)`, `PaneRun(ctx, session, pane, text)`, `PaneRead(...)`). The real implementation shells out with `exec.CommandContext` to `herdr --session <name> …`; this is the single seam for the untestable-live parts.
- **pty↔ws bridge**: `creack/pty` spawns `herdr --session <path>` (path from the URL, sanitized `[a-zA-Z0-9-]`, fallback `default`); `coder/websocket` (context-native, chosen over gorilla) bridges pty↔browser; resize (cols/rows) propagates to the pty and drives Herdr's responsive/mobile layout.
- **Artifact daemon** — one **atomic multipart endpoint** (`/send`): request carries the promptbox template (ordered text runs + file markers), the files in order, and the session name. Atomically: save each file → `/tmp/<prefix>-<server-uid>/<uuid>[.ext]` (flat; `<server-uid>` = the OS user the daemon runs as) → resolve markers to paths **server-side** → `HerdrClient.PaneRun` into the session's focused pane → return ok/fail. Any failure ⇒ inject nothing (all-or-nothing by ordering: save all → resolve → single run). `/clientlog` accepts frontend error posts.
- **Serve**: `//go:embed all:frontend/dist` → static SPA; **no Vite manifest** (SPA, index.html already carries hashed refs); **SPA fallback** — non-asset/non-API paths serve `index.html`; runtime config derived client-side from `location`. Hashed assets long-cache, `index.html` no-cache.
- **Observability**: `log/slog` **JSON-first, text fallback** via `--log-format`/`LOG_FORMAT` (optional TTY auto-detect). **Correlation ids** in ctx (`session` + `conn-id` + `req-id`, time-ordered e.g. `rs/xid`; echo inbound `X-Request-Id`) tag every log line and are returned in `X-Request-Id`/`X-Correlation-Id` on all responses (ws: conn-id on connect + in error frames). Instrument ws lifecycle, pty spawn/exit/resize, and the inject path (upload metrics + each `pane run` cmd/exit/stderr/duration + atomic outcome; herdr stderr quoted exact). Optional OTel span over the inject flow behind a flag.
- **Error handling**: fail loud / fail atomic / preserve user work / never crash the shared server. Wrapped errors carry context; **error source identification** despite no native stacks — a stack-capturing errors package (`go-faster/errors` default, `cockroachdb/errors` if richer) + `slog AddSource:true` + `recover` middleware logging `runtime/debug.Stack()`. Distinguish 4xx (bad session/multipart) from 5xx; graceful degrade if herdr unreachable.

**Frontend (TypeScript + Svelte 5 + Vite, no SvelteKit)**
- Terminal via `@xterm/xterm` + `@xterm/addon-fit` + `@xterm/addon-webgl`, driven **outside Svelte reactivity** (pty bytes written straight to xterm — the framework is never in the hot render path).
- Required page controls: viewport `width=device-width, user-scalable=no`; `preventDefault` on `contextmenu`; `touch-action:none`; fit-addon→cols; preserve xterm's focusable textarea (tap = soft keyboard); **toggleable Termux-style accessory key bar** with **sticky/one-shot modifiers** (Ctrl/Alt/Fn latch for the next key), default keys ESC/CTRL/ALT/TAB/FN/arrows and a second row of HOME/END/PGUP/PGDN/`/`/`-`/`|`/`~`/`:`/F1–F12; **PWA manifest + `display:standalone`**; `overscroll-behavior:none`; **ws auto-reconnect**; optional font +/− using the `fontSize` lever.
- **Promptbox** = ordered **segment list** (text runs + file **pills**), not string parsing. A pill is an atomic chip (`contenteditable=false` or equivalent) so backspace deletes it whole. Clipboard `paste` of a file item inserts a pill. On send, **`serialize(segments)`** produces `{template, files[]}` posted as one atomic multipart request; ordering is inherent in pill position.
- **Attachment UI**: image → local thumbnail (object URL); PDF → optional pdf.js preview (lazy `import()`); corner mime badge (TXT/JPEG/PDF/PNG/ZIP/TAR-GZIP…), unknown → no badge; no preview → lucide icon. No per-type border.
- Animations via Svelte built-in transitions + `animate:flip` (pill/attachment reorder); `lucide-svelte` icons; scoped CSS + variables.
- **Failable error-feedback** on every interaction: upload/inject failure and ws-disconnect surface inline with the full message + correlation ref id, keeping the user's text + pills intact; ws shows a visible "reconnecting…" state; a small client logger posts errors to `/clientlog` with the same id.
- Frontend work loads the `frontend-design` skill.

**Multi-session** — `(domain)/(path)` → `herdr --session <path>` on both the render pty and the inject daemon (fallback `default`, sanitized). Separate runtime focus and sizing for the single owner.

**Docs** — README + operator/deploy guide (nginx gateway, routing, env/flags, systemd) + design docs via Starlight (`astro-docs-setup`/`astro-docs-authoring`) + `llms.txt` + endpoint reference with I/O examples. **Code docs are written for coding agents** (the future reader): godoc on all exported + non-trivial unexported Go + package `doc.go`; tsdoc on key TS/Svelte modules; explain why/invariants/ctx-lifecycle/atomicity/correlation and call out gotchas (shared focus, all-or-nothing inject, framework-out-of-hot-path).

## Testing Decisions

Good tests assert **external behaviour**, not implementation details. Test at the **highest seam**;
prefer few seams. This is greenfield, so these tests **set the pattern** (no prior art in-repo).

- **Backend — `HerdrClient` interface (primary seam).** All Herdr interaction is behind it, so unit tests inject a **fake** `HerdrClient` and never need a live Herdr. Table-driven tests.
- **Backend — HTTP boundary via `httptest`.** Exercise `/send` end-to-end with the fake `HerdrClient` and a temp dir for `/tmp`: multipart parsing, path resolution, **atomicity** (any failure injects nothing), and the error + correlation-header behaviour. Same for `/clientlog`.
- **Backend — pty↔ws bridge is integration-level**, verified against a live Herdr (not unit-tested); keep its logic thin so little is untested.
- **Frontend — `serialize(segments)` pure function (primary seam), Vitest.** The ordering/marker logic (the bug-prone part) is pure and DOM-free.
- **Frontend — transport behind an interface**, faked in component tests. Pill-editor DOM behaviour (contenteditable, backspace-deletes-pill) is secondary, higher-effort component/e2e (Testing Library / Playwright).

## Out of Scope

- **Security / auth / TLS / upload size limits** — the nginx gateway.
- **Application identity and permissions** — absent by design. Deployment has one trusted owner; gateway controls access before requests reach service.
- **A custom TUI renderer** — we stream the real Herdr TUI (xterm), never redraw it.
- **Herdr plugin packaging** — standalone service; "plugin" is only a name prefix.
- **Local desktop clipboard bridging** — that is `herdr --remote`'s job.
- **App-managed `/tmp` cleanup** — left to the OS for now.

## Further Notes

- Verified live against Herdr 0.7.3: `herdr pane run <pane> '<text>'` injects text+Enter atomically (paths with slashes/spaces intact); `herdr pane current` gives the server-wide focused pane; focus is **shared across clients** (a browser navigating moves it for everyone — which is why "inject into the focused pane" needs no picker); terminal size is **"last active client wins"**; all `herdr` CLI output is JSON.
- Herdr's mobile layout triggers purely on terminal width ≤ `[ui] mobile_width_threshold` (default 64 cols) — reached via accurate cols reporting, not any Herdr API.
- Durable Herdr knowledge is captured in `.pi/skills/herdr/SKILL.md`; repo constraints in `.pi/rules/herdr-web-tui.md`.
