---
paths:
  - frontend/index.html
  - frontend/public/**
  - frontend/pwa.test.js
---

# Minimal installable PWA

Keep PWA install-only: dynamic `/manifest.webmanifest`, `display: standalone`, theme metadata, and 192×192 + 512×512 PNG icons sourced from `https://herdr.dev/`. Derive per-user manifest identity from trusted gateway `Remote-User`; never accept user identity from query parameters. Keep server tests validating manifest identity and icon references; keep `frontend/pwa.test.js` validating icon dimensions.

Add service worker, offline cache, or push only when explicitly required.
