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

Keep PWA install-only: dynamic `/manifest.webmanifest`, `display: standalone`, theme metadata, and 192×192 + 512×512 PNG icons sourced from `https://herdr.dev/`. PWA `name`, `short_name`, and identity use configured `SERVER_NAME` exactly; app has one PWA identity per server and no user identity input. Use native Screen Wake Lock only in standalone mode; reacquire after visibility returns, release on cleanup, and show failure status. Keep focused manifest, icon, and wake-lock tests.

Add service worker, offline cache, or push only when explicitly required.
