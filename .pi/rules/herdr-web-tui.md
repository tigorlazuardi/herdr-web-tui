---
paths:
  - cmd/**
  - internal/pty/**
  - internal/server/**
  - frontend/src/lib/terminal.ts
skills:
  - herdr
---

# Thin standalone Herdr web layer

Keep this service a thin standalone browser layer over Herdr. Render live TUI through PTY + HTTP/WS; use Herdr socket API or CLI for control. Build only web-specific bridges.

Reuse Herdr rendering/control primitives. Keep this service independent from Herdr plugin manifests and tmux.
