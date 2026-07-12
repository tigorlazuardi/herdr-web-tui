# Herdr Web TUI

A thin browser frontend for a running [Herdr](https://github.com/tigorlazuardi/herdr)
server: it streams the live Herdr TUI over http+websocket, and adds an atomic
artifact promptbox for getting files into a pane — one static Go binary, no
separate frontend to deploy.

## Quick start

```bash
go build -o herdr-web-tui ./cmd/herdr-web-tui
./herdr-web-tui
```

Open `http://localhost:8080`. Requires a Herdr server reachable via the
`herdr` CLI on `PATH`.

## LAN / security note

This binary has no built-in auth or TLS — it's meant to sit behind a gateway
(nginx) that handles both. Listens on `:8080` by default; don't expose it
directly to a LAN or the internet. See the deploy guide.

## Docs

- [Design spec](docs-site/src/content/docs/design/2026-07-11-herdr-web-tui-spec.mdx) — architecture, scope, decisions.
- [Operator / deploy guide](docs-site/src/content/docs/guides/operator-deploy-guide.mdx) — running the binary, the nginx gateway, env/flags, systemd.
- [Endpoint reference](docs-site/src/content/docs/reference/endpoints.mdx) — `/send`, `/clientlog`, `/ws`.

Published site (once deployed): `docs-site/` is a Starlight site — `cd docs-site && npm run build`, or `npm run dev` to browse locally.
