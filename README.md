# Herdr Web TUI

A thin browser frontend for a running [Herdr](https://github.com/tigorlazuardi/herdr)
server. It streams the live Herdr TUI over HTTP + WebSocket and adds an atomic
artifact promptbox for getting files into a pane — shipped as one static Go
binary with the frontend embedded, so there's nothing else to deploy.

## Quick start

```bash
go build -o herdr-web-tui ./cmd/herdr-web-tui
./herdr-web-tui
```

Open `http://localhost:8080`. Requires a Herdr server reachable via the `herdr`
CLI on `PATH`.

Nix users can run it straight from the flake:

```bash
nix run github:tigorlazuardi/herdr-web-tui
```

A Home Manager module is exported too (`homeManagerModules.default`) — see the
deploy guide.

## Security note

No built-in auth or TLS — it's meant to sit behind a gateway (nginx) that
handles both. Listens on `:8080` by default; don't expose it directly to a LAN
or the internet.

## Documentation

Operator and API docs are published from `docs-site/` (a Starlight site):

- **[Operator / deploy guide](docs-site/src/content/docs/guides/operator-deploy-guide.mdx)** — Nix flake install, running the binary, the nginx gateway, env/flags, systemd.
- **[Endpoint reference](docs-site/src/content/docs/reference/endpoints.mdx)** — `/send`, `/clientlog`, `/ws`.

Browse locally with `cd docs-site && npm run dev`, or build with `npm run build`.

Internal design notes and history live under `docs/`.

## License

[MIT](LICENSE)
