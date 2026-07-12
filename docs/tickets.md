# Tickets: Herdr Web TUI

Tracer-bullet slices building a thin browser front for a running Herdr server: live TUI
render + an atomic artifact promptbox, mobile-first, observable. Source spec:
`.scratch/web-tui/PRD.md` (design detail: `docs/design/2026-07-11-herdr-web-tui-spec.mdx`).

Work the **frontier**: any ticket whose blockers are all done. After ticket 0, tickets 1 and 5
are both open and can run in parallel. Clear context between tickets; each `/implement` starts fresh.

Every feature ticket carries its own **telemetry + error handling + agent-audience doc comments**
as part of "done" — the observability spine is built in ticket 0 and each slice wires into it.

## 0. Walking skeleton + observability spine

**What to build:** A single Go binary that serves the built Svelte/Vite frontend over the LAN, and the cross-cutting spine every later slice inherits. Opening the app URL shows a placeholder page served from the embedded frontend; every request is already logged, correlated, and panic-safe.

**Blocked by:** None — can start immediately.

- [ ] Go module + TypeScript/Svelte 5/Vite (no SvelteKit) frontend scaffold in one repo.
- [ ] Vite `dist` embedded via `//go:embed all:frontend/dist`, served with `http.FileServer(http.FS(...))`; no Vite manifest.
- [ ] SPA fallback: non-asset, non-API paths serve `index.html` (so `/anything` returns the app, not 404).
- [ ] Hashed assets sent with long cache headers; `index.html` sent `no-cache`.
- [ ] `context.Context` backbone: per-request ctx threaded to handlers.
- [ ] `log/slog` logging, JSON-first with text fallback via `--log-format`/`LOG_FORMAT` (optional TTY auto-detect).
- [ ] Correlation middleware: `req-id` (time-ordered, e.g. `rs/xid`; echo inbound `X-Request-Id`) in ctx, on every log line, and returned in `X-Request-Id`/`X-Correlation-Id` on all responses.
- [ ] `recover` middleware: a panic in one request logs `runtime/debug.Stack()` + correlation id and returns 500 without crashing the server.
- [ ] Error-source setup: stack-capturing errors package (`go-faster/errors` default) + `slog AddSource:true`.
- [ ] `make`/task to build frontend then the binary; running the binary serves the placeholder page over LAN.
- [ ] Agent-audience doc comments (godoc + package `doc.go`) on the serve/logging/correlation/recover pieces.

## 1. Live TUI render

**What to build:** Opening the app in a browser renders the live Herdr TUI (default session), faithfully, with working mouse and resize. The pty and its goroutines tear down cleanly when the ws drops.

**Blocked by:** Ticket 0.

- [ ] Backend spawns `herdr --session default` in a pty (`creack/pty`), bridged to the browser over `coder/websocket` (context-native).
- [ ] Frontend renders the pty stream with `@xterm/xterm` + `addon-fit` + `addon-webgl`, driven **outside Svelte reactivity** (bytes written straight to xterm).
- [ ] Mouse events (click panes/tabs/borders) pass through to Herdr; right-click does not open the browser context menu.
- [ ] Terminal resize propagates cols/rows to the pty (fit-addon → cols), and Herdr re-lays-out.
- [ ] ws drop / navigate-away cancels the ctx → pty reader goroutine returns and the pty is closed (no leak).
- [ ] ws lifecycle (connect/disconnect+reason) and pty spawn/exit/resize logged with the correlation id.
- [ ] Errors (spawn failure, herdr unreachable) surface clearly rather than hanging; herdr stderr quoted exact.
- [ ] Agent-audience doc comments on the pty bridge and ws client (why bytes bypass the framework, ctx-lifecycle gotcha).

## 5. Artifact inject — backend

**What to build:** A server-side endpoint that receives a promptbox bundle (template + ordered files + session) and atomically types the resolved text into that session's focused pane. Verifiable with `curl` against a live Herdr, and unit-tested without one.

**Blocked by:** Ticket 0.

- [ ] `HerdrClient` interface (`FocusedPane(ctx, session)`, `PaneRun(ctx, session, pane, text)`, `PaneRead(...)`) with a real `exec.CommandContext` implementation shelling to `herdr --session <name> …`.
- [ ] `/send` atomic multipart endpoint: save each file → `/tmp/<prefix>-<server-uid>/<uuid>[.ext]` (flat) → resolve markers to paths server-side → `PaneRun` into the session's focused pane → return ok/fail.
- [ ] All-or-nothing by ordering: any file-save failure aborts before any inject; a failed inject leaves nothing typed. Client never sees `/tmp` paths.
- [ ] Session comes from the request; sanitized, fallback `default`.
- [ ] `/clientlog` endpoint accepts frontend error posts (carrying the correlation id).
- [ ] 4xx for bad session / malformed / oversized multipart; 5xx for server faults; graceful message if herdr unreachable.
- [ ] Inject path instrumented: upload metrics + each `pane run` cmd/exit/stderr/duration + atomic outcome; herdr stderr quoted exact. Optional OTel span behind a flag.
- [ ] `httptest` tests with a fake `HerdrClient` and a temp `/tmp` dir: multipart parsing, resolution, atomicity, and the error + correlation-header behaviour (table-driven).
- [ ] Agent-audience doc comments on `HerdrClient`, the atomic ordering invariant, and the resolve step.

## 2. Multi-session routing

**What to build:** Different URL paths map to different Herdr sessions, so concurrent browser views don't contend over one session's focus or sizing.

**Blocked by:** Ticket 1.

- [ ] The URL path `(domain)/(path)` selects `herdr --session <path>` for the render pty (sanitized `[a-zA-Z0-9-]`, fallback `default`).
- [ ] The same session name is used by the inject daemon so a promptbox lands in the pane of the session the user is viewing.
- [ ] Two different paths render two independent sessions in two tabs without cross-contention.
- [ ] Session name logged as a correlation field.
- [ ] Agent-audience doc comments noting this is concurrency isolation, not tenancy.

## 3. Mobile/touch hardening + PWA

**What to build:** The app is usable on a phone — Herdr's single-column mobile layout, no runaway zoom, soft keyboard on tap, installable, survives network blips.

**Blocked by:** Ticket 1.

- [ ] Viewport meta `width=device-width, user-scalable=no`; `touch-action:none`; `overscroll-behavior:none` (no pull-to-refresh reload).
- [ ] Accurate cols reporting so Herdr drops to mobile layout on phones (≤ `mobile_width_threshold`); optional font +/− lever.
- [ ] Tapping the terminal summons the soft keyboard; long-press does not leave the page zoomed.
- [ ] PWA manifest + `display:standalone` (installable, no browser chrome eating rows).
- [ ] ws auto-reconnect with a visible "reconnecting…" state; on reconnect the pane re-attaches and re-renders.
- [ ] Verified on-device (phone portrait): mobile layout, no zoom lock-up, reconnect works.
- [ ] Agent-audience doc comments on the viewport/touch levers and the reconnect logic.

## 4. Accessory key bar (sticky modifiers)

**What to build:** A toggleable on-screen key bar lets a touch keyboard send keys it otherwise can't — Ctrl/Esc/Tab/arrows/prefix — using sticky one-shot modifiers (no chording).

**Blocked by:** Ticket 1.

- [ ] Toggleable bar; hidden on desktop / with a hardware keyboard.
- [ ] Sticky modifiers: tapping Ctrl/Alt/Fn latches for the next key (tap Ctrl then `c` sends `ctrl+c`); prefix (Ctrl+B) button.
- [ ] Row 1: ESC/CTRL/ALT/TAB/FN/arrows; row 2: HOME/END/PGUP/PGDN + common special chars + F-keys; long-press for alternates.
- [ ] Keys inject into the same ws input path as the physical keyboard.
- [ ] Verified on-device: `ctrl+c`, `esc`, `tab`, arrows, and the prefix all reach Herdr.
- [ ] Agent-audience doc comments on the sticky-modifier state machine.

## 6. Artifact promptbox — frontend

**What to build:** A promptbox where the user writes text with inline file attachments and sends the whole thing atomically to the focused pane. Attachments are atomic pills; a pasted screenshot becomes a pill; send is all-or-nothing with clear failure feedback.

**Blocked by:** Ticket 5, Ticket 1.

- [ ] Ordered **segment editor**: text runs + file **pills**; a pill is atomic (backspace deletes it whole); attach inserts a pill at the caret showing filename + mime badge.
- [ ] Clipboard `paste` of a file item (e.g. screenshot) inserts a pill (mime from item, magic-byte sniff fallback) instead of pasting text.
- [ ] Attachment UI: image thumbnail (object URL); PDF preview via lazy `import()` of pdf.js; corner mime badge (TXT/JPEG/PDF/PNG/ZIP/TAR-GZIP…), unknown → no badge; no-preview → lucide icon; Svelte `animate:flip` on reorder.
- [ ] On send, `serialize(segments)` produces `{template, files[]}` posted as one atomic multipart request to `/send`; ordering inherent in pill position.
- [ ] All-or-nothing: on any failure nothing is sent and the user's text + pills stay intact for retry.
- [ ] Failure surfaces inline with the full error message + copyable correlation ref id (from `X-Request-Id`); errors bubble verbatim; client logger posts to `/clientlog` with the same id.
- [ ] `serialize(segments)` is a pure, DOM-free function unit-tested with Vitest (ordering/marker cases); transport behind a faked interface.
- [ ] Frontend work loads the `frontend-design` skill; agent-audience tsdoc on the pill editor, `serialize`, and the transport client.

## 7. Documentation

**What to build:** Someone can deploy and understand the service; the endpoints and design are documented.

**Blocked by:** Ticket 6.

- [ ] README (repo root): what it is, quick run, LAN caveat, link to the design doc.
- [ ] Operator/deploy guide: run the binary; nginx gateway (auth/TLS/`client_max_body_size`); `(domain)/(session)` routing; env/flags (`LOG_FORMAT`, port, `/tmp` prefix, correlation header names); systemd unit; mobile-cols note.
- [ ] Endpoint reference for `/send`, `/clientlog`, and the ws stream, each with an I/O examples block.
- [ ] Design docs published via Starlight (`astro-docs-setup` scaffold, `astro-docs-authoring` dialect) + `llms.txt`, with this spec as the first doc.
- [ ] Note that agent-audience code comments (godoc/tsdoc) ship per-slice, not here.
