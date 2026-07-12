#!/usr/bin/env bash
# Visual-regression harness (mobile-ux-v2.md verification). Builds the real
# frontend + Go binary, serves them on a throwaway port, then drives a
# Playwright script against that live server from inside a podman container
# (host networking, so the container can hit 127.0.0.1:8099 on the host).
# No mocked terminal/DOM — this is the actual app.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
BIN=/tmp/herdr-web-tui-e2e
ADDR=127.0.0.1:8099
HERDR_BIN_DIR=/nix/store/qvb0xrgyskfqjavy40dgj3yxn5v24waa-herdr-0.7.3/bin
IMAGE=mcr.microsoft.com/playwright:v1.56.0-noble

cd "$ROOT/frontend"
npm run build
cd "$ROOT"
go build -o "$BIN" ./cmd/herdr-web-tui

PATH="$HERDR_BIN_DIR:$PATH" ADDR="$ADDR" LOG_FORMAT=text "$BIN" &
SERVER_PID=$!

cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Bounded poll: the binary should come up in well under a second, but give
# it 10s (50 * 0.2s) before giving up rather than hanging forever.
ok=0
for _ in $(seq 1 50); do
  if curl -sf -o /dev/null "http://$ADDR/"; then
    ok=1
    break
  fi
  sleep 0.2
done
if [ "$ok" -ne 1 ]; then
  echo "server on $ADDR never came up" >&2
  exit 1
fi

mkdir -p "$ROOT/frontend/e2e/screens"

podman run --rm --network=host \
  -v "$ROOT/frontend/e2e:/work:z" \
  -w /work \
  "$IMAGE" \
  sh -c 'npm i playwright@1.56.0 --no-save --silent; node visual.mjs'

echo "screenshots written to $ROOT/frontend/e2e/screens/"
