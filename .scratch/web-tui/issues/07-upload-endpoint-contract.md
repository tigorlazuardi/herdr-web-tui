# Define the upload endpoint contract

Type: grilling
Status: resolved
Blocked by: —

## Question

Specify the upload endpoint the promptbox calls.

## Answer

**Single atomic multipart endpoint** (multipart/form-data — most flexible). One request
carries the whole bundle:
- the promptbox **template** (ordered segments: text runs + file markers), and
- all attached **files** (in order).

The request also carries the **session name** (from the `(domain)/(path)` route, fallback `default`; sanitized to `[a-zA-Z0-9-]`). All Herdr calls are scoped with `herdr --session <name>` so inject targets the right session's focused pane.

Daemon, atomically:
1. Save each file to `/tmp/<prefix>-<server-uid>/<uuid>[.ext]` (server-side; see ticket 05).
2. Resolve each marker to its saved path — **path resolution is server-side; the client never sees `/tmp` paths**.
3. `herdr --session <name> pane run <focused-pane> <resolved-text>` (text + Enter atomic) into that session's focused pane.
4. Return `ok` / `fail`. Any file-save or inject failure → **nothing is injected** (all-or-nothing, per ticket 04).

- **Size / count limits = the gateway's job** (nginx `client_max_body_size`), not the app — consistent with security being out of scope here.
- Text does not need a separate call; it rides in the same multipart request as the template field.
