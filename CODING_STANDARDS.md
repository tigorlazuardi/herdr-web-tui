# Coding Standards

These rules describe current repository practice and binding decisions in `docs/2026-07-11-herdr-web-tui-spec.mdx` and `.pi/rules/`.

## Formatting

Go formatting is enforced by `gofmt`; frontend compile and type correctness are enforced by `npm run check` using `frontend/tsconfig.app.json` and `frontend/tsconfig.node.json` (no frontend formatter is configured, so preserve the existing single-quote, no-semicolon style).

## Naming

- Use idiomatic Go names: lower-case package names, `PascalCase` exported identifiers, `camelCase` unexported identifiers, and initialisms such as `ID`, `HTTP`, and `PTY` in uppercase.
- Name Go tests `Test<Subject>_<Condition>_<Outcome>` when conditions matter; keep package tests beside implementation in `*_test.go`.
- Name Svelte components in `PascalCase.svelte`. Name TypeScript modules and colocated tests in lower-case kebab-case or single-word files, with tests ending `.test.ts`.
- Use `camelCase` for TypeScript values/functions, `PascalCase` for types, and explicit state names such as `connectionState`, `sending`, and `wakeLockUnavailable` rather than generic flags.

## Modules and Seams

- Keep the executable in `cmd/herdr-web-tui`; keep backend implementation packages under `internal/`. Add browser UI components under `frontend/src/components` and DOM-independent logic, transports, and browser bridges under `frontend/src/lib`.
- Keep this service a thin standalone browser layer over Herdr: render through PTY + HTTP/WebSocket and use Herdr CLI/socket control primitives. Do not add Herdr plugin-manifest or tmux coupling.
- Thread `context.Context` from HTTP/WebSocket entry points through spawned processes and goroutines. Cancellation must close PTYs, stop readers, and terminate context-bound commands.
- Keep terminal byte writes and xterm hot-path work outside Svelte reactivity. Use Svelte scoped `<style>` blocks and CSS variables; do not add a CSS framework.
- Put replaceable external effects behind the smallest consumer-owned seam needed for tests, as with `HerdrClient` and `UploadClient`; keep pure serialization, parsing, marker, and frame logic DOM-free.
- Preserve the single-binary boundary: Vite builds `frontend/dist`, `dist.go` embeds it, and Go serves the embedded files. Do not add SvelteKit, SSR, a router, or a Vite manifest.

## Error Handling and Telemetry

- Validate request-controlled values at entry points. Session names use the documented allowlist and default; malformed or oversized input and invalid sessions return 4xx, while server faults return 5xx.
- Wrap Go errors descriptively at package/process/I/O boundaries with `github.com/go-faster/errors`; use `errors.Is` for classification. Never swallow Herdr stderr or replace it with a generic message.
- Use `log/slog` with context correlation fields. HTTP responses, including failures, retain request/correlation headers; WebSocket failures retain connection correlation. Log lifecycle and failure details at PTY, WebSocket, upload, and Herdr-command seams.
- Request paths remain panic-safe. WebSocket read/write failure cancels the owning context and tears down resources; normal and abnormal closes remain distinguishable in logs.
- Frontend failures surface inline with full error text and correlation reference. Preserve prompt text and attachments on failure; never clear retryable user input before confirmed success.
- Preserve atomic inject ordering: save every file, resolve every marker, then perform one final `herdr pane run`. Any earlier failure must result in zero inject calls, and client responses must not expose staging paths.

## Testing

- Add focused Go tests beside implementation using `testing`, `httptest`, `t.TempDir`, and small fakes. Use table-driven cases where one behavior has multiple inputs; always cover invalid input, dependency failure, cancellation/cleanup, correlation, and atomicity where applicable.
- Add TypeScript unit tests beside DOM-free modules with Vitest and `describe`/`it`/`expect`. Keep external transport/browser APIs replaceable with small fakes and assert failure paths as well as success.
- Keep PWA manifest/icon behavior checks in `frontend/pwa.test.js`. Do not add a service worker, offline cache, or push behavior unless a ticket explicitly requires it.
- Run `go test ./...` for backend changes. Run `npm test` and `npm run check` from `frontend/` for frontend changes. Changes spanning the embedded app must also pass the frontend build before the Go build.

## Comments and Documentation

- Add godoc to every exported Go identifier and every non-trivial unexported function; keep package role and lifecycle notes in `doc.go` where present.
- Add TSDoc/JSDoc to key frontend modules/functions and component state machines. Explain invariants, ownership, cancellation, atomicity, correlation flow, native-browser workarounds, and other reasons a safe edit depends on; do not restate signatures or obvious statements.
- Keep narrowly scoped `svelte-ignore` directives adjacent to the suppressed node and explain why native/component behavior already satisfies the interaction requirement.

## Repository-Specific Prohibitions

- Do not move TLS, authentication, authorization, or access control into this service without an explicit architecture decision; the reverse proxy owns them. Session separation is concurrency isolation, not tenancy or security.
- Do not accept user identity from query parameters for gateway-derived behavior; use trusted gateway `Remote-User` where the existing feature requires identity.
- Do not expose `/tmp` artifact paths to clients, partially inject a prompt, or clear promptbox state after a failed send.
- Do not add application artifact cleanup/TTL or upload count/size policy until required; OS `/tmp` cleanup and gateway `client_max_body_size` currently own those concerns.
- Do not add dependencies where repository-selected platform/stdlib features already cover the need: `net/http` for HTTP/multipart, `log/slog` for logging, native Screen Wake Lock for standalone wake lock, Svelte transitions for animation, and CSS for styling.
- When `go.mod` or `go.sum` changes, refresh `vendorHash` in `nix/package.nix` from Nix's reported hash and verify `nix build .#herdr-web-tui --no-link`.
- Follow matching path-scoped rules in `.pi/rules/`; they are binding refinements of this document.

The Fowler smell baseline from the `code-review` skill still applies below these standards. Where this document and the baseline disagree, this document wins.

First ticket touching an area sets its living pattern for that area. Later reviews check new code against both this document and that code anchor; disagreement signals that the standard may need updating, not that the code is wrong by default.
