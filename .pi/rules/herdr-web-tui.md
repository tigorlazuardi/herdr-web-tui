---
paths:
  - "**/*"
---

# herdr-web-tui — project conventions

This repo is a **thin standalone web layer over Herdr**: render the live Herdr TUI
in a browser over http+ws, and add an artifact upload path the browser lacks. Load
the `herdr` skill for the Herdr control surface.

## Hard constraints (from wayfinder charting)

- **Thin layer only.** Reuse ttyd (or equivalent pty-wrapper) for TUI rendering and the
  Herdr socket API / CLI for control. Write only the artifact bridge (upload endpoint +
  inject glue + promptbox UI). Do NOT build a custom TUI renderer from `pane.read`.
- **Security is the gateway's job.** Auth / TLS / access control live in nginx (or the
  reverse proxy) in front of this app — out of scope here. Do not add auth inside the app.
- **Not a Herdr plugin.** Standalone service; "plugin"/prefix naming ≠ `herdr-plugin.toml`.
- **Not tmux.** Never use tmux commands or config.

## Artifact promptbox contract

- The web promptbox is **atomic**: user text + attachment(s) compose ONE bundle, submitted once.
- Flow: upload blob → `/tmp/<prefix>-<userid>/<uuid>[.ext]` (flat, no nesting) → compose
  `text + path(s)` → inject into Herdr's **focused pane** → auto-submit.
- **Focused pane only** — no target picker. Resolve via `herdr pane current` (socket:
  omit `pane_id` → active focused pane). Simplest wins.
- Prefer `herdr pane run` (text + Enter atomic) or `herdr agent send` over
  `send-text` + separate `send-keys enter` for the submit.
- `.ext` on the blob is an agent hint; the agent consumes the file by reading the path.

## Open decisions (not yet locked — see `.scratch/web-tui/`)

Transport tool, exact inject/compose format, `<userid>` source (auth at gateway),
artifact type scope, cleanup/TTL, upload endpoint shape. Consult the wayfinder map
before hardcoding any of these.
