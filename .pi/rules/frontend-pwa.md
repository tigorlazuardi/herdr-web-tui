---
paths:
  - frontend/index.html
  - frontend/public/**
  - frontend/pwa.test.js
---

# Minimal installable PWA

Keep PWA install-only: web manifest, `display: standalone`, theme metadata, and 192×192 + 512×512 PNG icons sourced from `https://herdr.dev/`. Keep `frontend/pwa.test.js` validating manifest icon references and dimensions.

Add service worker, offline cache, or push only when explicitly required.
