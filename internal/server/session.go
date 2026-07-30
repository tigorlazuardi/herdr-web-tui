package server

import "regexp"

// defaultSession is used whenever the caller-supplied session name is empty
// or fails sanitization.
const defaultSession = "default"

var sessionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// sanitizeSession validates name against the allowlist [a-zA-Z0-9-] (design
// doc: this string is fed straight into a process spawn, `herdr --session
// <name> …`, so it must never carry shell metacharacters or path
// separators) and falls back to defaultSession for anything empty or
// disallowed. Both the render pty (pty.go's newPTYHandler, reading the
// "session" query param off the /ws URL) and the inject daemon (send.go's
// ServeHTTP, reading the "session" multipart field) call this same helper,
// so a browser tab on one URL path always resolves to the exact same Herdr
// session on both the pty stream and any promptbox send from that tab —
// this is what makes multi-session routing (design doc, "Multi-session
// concurrency") separate runtime namespaces for the single owner: different
// paths do not contend over one session's view, focus, or sizing. Gateway
// access control remains outside this service.
//
// This never itself produces a 4xx: a syntactically bad name silently
// becomes "default", matching the fallback behaviour the design doc
// specifies for URL-path routing. The distinct "bad session" 4xx the
// design doc's Error handling section calls out is a different case,
// handled in send.go: a syntactically valid name that herdr reports has no
// such live session.
func sanitizeSession(name string) string {
	if name == "" || !sessionNamePattern.MatchString(name) {
		return defaultSession
	}
	return name
}
