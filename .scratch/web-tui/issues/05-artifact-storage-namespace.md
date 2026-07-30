# Decide artifact storage namespace + cleanup

Type: grilling
Status: resolved
Blocked by: —

## Question

Finalize the flat storage convention `/tmp/<prefix>-<server-uid>/<uuid>[.ext]`:
- `<prefix>` — this project's own prefix string.
- `<server-uid>` — numeric effective UID of daemon process; storage remains server-side.
- `<uuid>` — global unique id per upload; flat, no nesting.
- `.ext` — how derived (from uploaded filename / mime) and why (agent hint).
- Cleanup / TTL — do blobs get purged? on session end? never (rely on /tmp)? Decide the ceiling.

## Answer

- `<server-uid>` = daemon's numeric effective UID. Path: `/tmp/<prefix>-<server-uid>/<uuid>[.ext]`, flat.
- `.ext` = agent hint, derived from the uploaded filename / mime.
- **Cleanup: leave to the OS for now** — no app-managed TTL. `# ponytail: OS /tmp cleanup only; add TTL/purge when it actually bites.`
