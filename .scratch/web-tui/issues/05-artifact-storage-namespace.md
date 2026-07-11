# Decide artifact storage namespace + cleanup

Type: grilling
Status: resolved
Blocked by: —

## Question

Finalize the flat storage convention `/tmp/<prefix>-<userid>/<uuid>[.ext]`:
- `<prefix>` — this project's own prefix string.
- `<userid>` — where does it come from, given auth is at the gateway (nginx)? Options: gateway passes a header (e.g. `X-Forwarded-User`), single-user (OS user running Herdr), or a per-connection id. Pick one.
- `<uuid>` — global unique id per upload; flat, no nesting.
- `.ext` — how derived (from uploaded filename / mime) and why (agent hint).
- Cleanup / TTL — do blobs get purged? on session end? never (rely on /tmp)? Decide the ceiling.

## Answer

- `<userid>` = the **OS user the daemon/server runs as**. The daemon knows its own server user ("whoever calls the daemon"). No gateway header, no per-connection id — storage location is purely a server-side concern. Path: `/tmp/<prefix>-<server-uid>/<uuid>[.ext]`, flat.
- `.ext` = agent hint, derived from the uploaded filename / mime.
- **Cleanup: leave to the OS for now** — no app-managed TTL. `# ponytail: OS /tmp cleanup only; add TTL/purge when it actually bites.`
