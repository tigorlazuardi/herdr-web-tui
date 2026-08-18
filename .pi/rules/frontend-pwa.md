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

Keep PWA install-only: dynamic `/manifest.webmanifest`, `display: standalone`, theme metadata, and bundled 192×192 + 512×512 PNG icons sourced from `https://herdr.dev/`. Operators may override favicon and PWA icons through validated absolute PNG file paths; bundled icons remain defaults. PWA identity derives only from configured `SERVER_NAME`; `name`, `short_name`, and browser title use `APP_NAME` exactly, falling back to `SERVER_NAME`. App has one identity per server and no user identity input. Use native Screen Wake Lock only in standalone mode; reacquire after visibility returns, release on cleanup, and show failure status. Keep focused manifest, icon, and wake-lock tests.

Add service worker, offline cache, or push only when explicitly required.
