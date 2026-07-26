---
paths:
  - frontend/src/App.svelte
  - frontend/src/components/KeyBar.svelte
  - frontend/src/components/Topbar.svelte
  - frontend/src/lib/frames.ts
  - frontend/src/lib/terminal.ts
  - internal/server/frames.go
  - internal/server/pty.go
---

# Mobile terminal layout

Drive Herdr mobile layout through accurate terminal columns: fit-addon resize frame → PTY SIGWINCH. Herdr switches at `[ui] mobile_width_threshold` (default 64 cols); no Herdr API toggle exists.

Keep both font +/− controls. Larger font reduces fitted columns for wide phones/tablets; both directions support readability.
