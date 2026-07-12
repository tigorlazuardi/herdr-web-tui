/**
 * sessionFromPath is the one place that decides which part of the URL is a
 * Herdr session name. Both the terminal ws client (terminal.ts) and, once
 * built, the promptbox's /send transport must derive the same session from
 * the same page's location.pathname — otherwise a browser tab could render
 * one session's pty while injecting into another's focused pane. Keeping
 * this in one shared module instead of duplicating a `pathname.split(...)`
 * in each caller is what guarantees that.
 *
 * This does not replicate the backend's allowlist ([a-zA-Z0-9-], see
 * internal/server/session.go's sanitizeSession) — it just picks the
 * candidate substring out of the URL. The server is the sole source of
 * truth for what a *valid* session name is and silently falls back to
 * "default" for anything else, so this function can stay a dumb path
 * split with no validation duplicated on the client.
 */
export function sessionFromPath(pathname: string): string {
  return pathname.split('/').filter(Boolean)[0] ?? ''
}
