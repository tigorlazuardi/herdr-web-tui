# Visual-test harness

Builds the frontend + Go binary, serves them locally, and drives
`visual.mjs` (Playwright, Pixel 7 viewport) against it from inside podman
to capture screenshots of the mobile UI chrome in `screens/`.

Run: `bash frontend/e2e/run-visual.sh`
