---
paths:
  - frontend/index.html
  - frontend/public/**
  - frontend/pwa.test.js
  - frontend/src/App.svelte
  - frontend/src/components/Topbar.svelte
  - frontend/src/lib/wake-lock*
  - internal/server/handler.go
  - internal/server/serve.go
  - internal/server/server_test.go
---

# Minimal installable PWA

Keep PWA install-only: dynamic `/manifest.webmanifest`, `display: standalone`, theme metadata, and 192×192 + 512×512 PNG icons sourced from `https://herdr.dev/`. Derive per-user manifest identity from trusted gateway `Remote-User`; never accept user identity from query parameters. Use native Screen Wake Lock only in standalone mode; reacquire after visibility returns, release on cleanup, and show failure status. Keep focused manifest, icon, and wake-lock tests.

Add service worker, offline cache, or push only when explicitly required.
