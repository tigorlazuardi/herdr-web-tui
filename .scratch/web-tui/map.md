# Wayfinder map: Herdr web-tui (thin layer)

Labels: `wayfinder:map`

## Destination

A **spec/design doc** for `herdr-web-tui`: a thin standalone web service over Herdr that
(1) renders the live Herdr TUI in a browser over http+ws via a pty wrapper (ttyd-style, no custom renderer), and
(2) adds an **atomic artifact promptbox** — a **positional template**: the user writes text with attachment **placeholders** (e.g. `imgcat [File 1]`); on send, each attachment uploads to a flat `/tmp/<prefix>-<server-uid>/<uuid>[.ext]` and its placeholder resolves to that path, then the whole resolved line is injected into Herdr's **focused pane** via `herdr pane run` (text + Enter atomic). Works for **any pane** — an agent chatbox or a plain shell command — not just agents.

Done when every decision below is locked and nothing is left to decide before someone builds it.

## Notes

- Domain: thin extra layer over Herdr. Reuse ttyd + Herdr socket API; write only the artifact bridge (upload endpoint + inject glue + promptbox UI).
- Standing prefs (from charting): simplest wins; focused-pane only (no target picker); security/auth/TLS is the **gateway's** job (nginx), out of scope here.
- Skills to consult per ticket: `research`, `prototype`, `grilling` + `domain-modeling`, `frontend-design` (web UI work).
- **Stack (locked FASE-1):** Backend = **Go** (`net/http` stdlib multipart, `creack/pty`, `coder/websocket` [context-native, over gorilla], `exec.CommandContext`; single static binary). **`context.Context` is the teardown backbone** — every conn/request ctx drives goroutine/pty/process cleanup on disconnect; prefer context-native libs. Frontend = **TypeScript + Svelte 5 + Vite** (no SvelteKit); terminal via `@xterm/xterm` + `addon-fit` + `addon-webgl` outside Svelte reactivity; Svelte built-in transitions + `animate:flip` for animations; `lucide-svelte` icons; `pdf.js` lazy-loaded on PDF attach; scoped CSS + variables. **Serve**: `//go:embed all:frontend/dist` → single binary serves static SPA (no Vite manifest — SPA, index.html already has hashed refs); SPA fallback (non-asset/non-API → index.html) so `(domain)/(session)` routing works; runtime config derived client-side from `location`; hashed assets long-cache, index.html no-cache. Full detail in `docs/design/2026-07-11-herdr-web-tui-spec.mdx`.
- **Telemetry (part of done):** Go `log/slog` — **JSON-first, text fallback** via `--log-format`/`LOG_FORMAT` (optional TTY auto-detect: text interactive, JSON piped; optional `tint` for pretty dev), ctx-scoped correlation id (`session`+`conn-id`+`req-id`); instrument ws lifecycle, pty spawn/exit/resize, and the inject path heavily (upload metrics + each `herdr pane run` cmd/exit/stderr/duration + atomic outcome; herdr stderr quoted exact); optional OTel span over the inject flow behind a flag. Frontend: failable error-feedback on every interaction + a small client logger posting to `/clientlog` with the same correlation id. Correlation id threads FE→backend→herdr for end-to-end reflection. **Correlation strategy**: ids (`session`/`conn-id`/`req-id`, time-ordered e.g. `rs/xid`, echo inbound `X-Request-Id`) returned in `X-Request-Id`/`X-Correlation-Id` header on all responses (ws: conn-id on connect + in error frames); FE displays full error + copyable ref id → grep 1:1 against backend logs. Not secrecy-sensitive → errors bubble verbatim.
- **Error handling:** fail loud / fail atomic / preserve user work / never crash the shared server. Backend: **error source identification** (Go has no native stack) via a stack-capturing errors package (`go-faster/errors` default, `cockroachdb/errors` if richer) + `slog AddSource:true` + `recover` logging `debug.Stack()` + descriptive `%w` wrapping at each boundary → every logged error carries correlation id + message chain + origin frames + log-site source; atomic inject by ordering (save all → resolve → one `pane run`, abort before any inject on failure, impossible to partially inject); herdr exec exit+stderr quoted exact mapped to user error; `recover` middleware; 4xx (bad session/multipart) vs 5xx; graceful degrade if herdr unreachable; ws error → cancel ctx → teardown. Frontend: inline failable feedback keeping text+pills intact, visible ws "reconnecting…", never lose user input.
- Control surface reference: Herdr socket API (`HERDR_SOCKET_PATH`) — `pane.send_input`, `pane.read`, `pane.list`, event subscriptions; CLI wrappers mirror it. Agent guide: https://herdr.dev/agent-guide.md ; socket API: https://herdr.dev/docs/socket-api/
- Key finding: Herdr clipboard image-paste only works in `herdr --remote` thin-client mode; browser = SSH-first case = no local clipboard → this promptbox fills that exact gap.

## Decisions so far

<!-- one line per resolved ticket: gist + link -->

- [Extract Herdr skills + rules](issues/01-extract-herdr-skills-rules.md) — wrote `.pi/skills/herdr/SKILL.md` (concept model + control surface) and `.pi/rules/herdr-web-tui.md` (thin-layer + promptbox constraints); confirmed `pane current` (focused pane), `pane run` (atomic text+Enter), `agent send` (direct-to-agent) as the inject primitives. Also: Herdr mobile view = terminal width ≤ `[ui] mobile_width_threshold` (64 cols), arrives via normal pty resize — folded into ticket 02.
- [Decide artifact storage namespace + cleanup](issues/05-artifact-storage-namespace.md) — `/tmp/<prefix>-<server-uid>/<uuid>[.ext]` flat; `<uid>` = the OS user the daemon runs as (storage is server-side only, no gateway header); cleanup left to the OS for now (no app TTL).
- [Decide artifact type scope + agent consumption](issues/06-artifact-type-scope.md) — any file type; strictly drop-path (agent reads the file); rich attachment-list UI (thumbnails / mime badges / lucide fallback) routed to ticket 04.
- [Lock the TUI rendering transport](issues/02-lock-tui-rendering-transport.md) — verified ttyd renders herdr over LAN and mobile view triggers when cols ≤ 64 (font hack). But raw ttyd's page is **not shippable**: mobile-view needs a font hack, right-click double-fires the browser menu, and touch triggers runaway zoom — all from ttyd's uncontrolled HTML. **Decision: build our own thin xterm.js frontend** (viewport `user-scalable=no`, `preventDefault` contextmenu, `touch-action:none`, fit-addon→cols, **toggleable mobile accessory key bar** for Ctrl/Esc/Tab/arrows/prefix, hidden on desktop / hardware-keyboard devices; **PWA standalone** + `overscroll-behavior:none` + **ws auto-reconnect**) with ttyd optional as a pty-ws backend. Inject-while-mirrored confirmed by transitivity.
- [Verify the artifact inject mechanism](issues/03-verify-artifact-inject-mechanism.md) — verified on live Herdr 0.7.3: `herdr pane run <pane> '<text>'` injects text+Enter atomically (path with slashes/spaces intact); `pane current` gives focused pane, `pane read --source visible` reads output; all CLI is JSON. Caveat: focused pane can be the caller's own session — target explicit `pane_id` for server-side automation.
- [Define the upload endpoint contract](issues/07-upload-endpoint-contract.md) — **single atomic multipart endpoint**: client posts template (ordered segments) + files; daemon saves to `/tmp`, resolves markers server-side, `herdr pane run` into focused pane, returns ok/fail (all-or-nothing). Size limits = gateway (nginx). Client never sees `/tmp` paths.
- [Design the atomic promptbox compose + UX](issues/04-atomic-promptbox-compose-ux.md) — positional segment editor with atomic file pills (native backspace-delete), clipboard-paste → pill, ordering inherent in pill position, thumbnails/mime-badges/lucide-fallback. Residual (attachment cap, placement) = build-time frontend-design, non-blocking.

- **Multi-session concurrency routing** — `(domain)/(session-path)` → `herdr --session <path>` (fallback `default`) on both render pty and inject daemon; per-session separation handles view/focus contention only. Sanitize session name (allowlist charset). Verified: `herdr --session <name>` is a global flag and each session has its own socket (`herdr session list`).

- **Documentation (part of done):** README + operator/deploy guide (nginx gateway, `(domain)/(session)` routing, env/flags, systemd) + design docs published via Starlight (`astro-docs-setup`/`astro-docs-authoring`) + `llms.txt` + endpoint reference (`/send`, `/clientlog`, ws with I/O examples) + **agent-audience code docs** (the future reader is a coding agent — godoc on all exported + non-trivial unexported Go + package `doc.go`; tsdoc on key TS/Svelte modules; explain why/invariants/ctx-lifecycle/atomicity/correlation + call out gotchas like shared-focus & all-or-nothing inject; every build worker prompt requires this) + lesson-learnt reports (`report-authoring`).

> **Map at destination.** Every decision is locked and the core mechanisms (inject, transport, mobile, focus) are verified live. Nothing left to decide before building. Next = assemble the spec/design doc and hand off to execution.

## Not yet specified

<!-- in-scope fog; graduates to tickets as the frontier advances -->

- **Overall architecture / component diagram** — how ttyd, the web backend, the socket-API inject bridge, and the browser fit together. Graduates once transport + inject mechanisms are locked.
- **Herdr session ↔ web-instance mapping** — one Herdr session per web instance? (partly answered: focus is server-wide/shared — a browser client navigating moves `pane current` for everyone, so daemon's focused-pane inject naturally targets current browser view, no picker needed. Remaining: multi-client focus contention.)
- ~~WebSocket resilience~~ — graduated to a transport requirement (ticket 02): **ws auto-reconnect** (Herdr panes survive detach → reconnect = re-attach + re-render), resize propagation via fit-addon, standalone PWA, no pull-to-refresh.
- ~~Multi-browser concurrency~~ — **resolved: per-session URL routing.** `(herdr-tui-domain)/(path)` → `(path)` = Herdr session name (fallback `default`). Each named session is a separate runtime namespace (own socket, focus, sizing), so different paths do not contend for view/focus. Service remains one trusted owner with access to every session. Cheap because `herdr --session <name>` is a global flag: render pty spawns `herdr --session <path>`, daemon injects with `herdr --session <path> pane run ...`. **Must sanitize** path→session name (allowlist `[a-zA-Z0-9-]`, fallback `default`) — URL-controlled string fed to process spawn. `/tmp` namespace stays per-uid flat (blobs are session-agnostic staging).
- **Final spec doc assembly** — the destination artifact; blocked on all decision tickets.

## Out of scope

<!-- ruled beyond the destination; closed, never graduates -->

- **Security / auth / TLS** — handled by the gateway (nginx). Explicit user call.
- **Application identity and permissions** — absent by design. Deployment has one trusted owner; gateway controls access before requests reach service.
- **Custom TUI renderer** from `pane.read` — rejected; ttyd streams the real TUI.
- **Herdr plugin packaging** — chose standalone thin service; "plugin" is just a name prefix.
- **Local desktop clipboard bridging** — that is `herdr --remote`'s job; we are the SSH-first gap-filler.
