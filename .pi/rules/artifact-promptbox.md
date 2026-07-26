---
paths:
  - frontend/src/components/Promptbox.svelte
  - frontend/src/lib/markers.ts
  - frontend/src/lib/segments.ts
  - frontend/src/lib/send.ts
  - frontend/src/lib/transport.ts
  - internal/artifact/**
  - internal/herdrclient/**
  - internal/server/send.go
skills:
  - herdr
---

# Atomic artifact promptbox

Submit text plus attachments as one atomic bundle: upload each blob to flat `/tmp/<prefix>-<userid>/<uuid>[.ext]`, compose text with saved paths, resolve focused pane, then inject once with auto-submit. Preserve all input when any step fails.

Use `herdr pane current` for target and `herdr pane run` or `herdr agent send` for atomic submit. File extension is agent hint.

Before locking transport, compose format, userid source, artifact scope, cleanup/TTL, or endpoint shape, consult `.scratch/web-tui/map.md` and its linked issue.
