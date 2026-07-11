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
// disallowed. Ticket 2 reuses this same helper for the URL-path → session
// mapping so both the render pty and the inject daemon agree on one
// sanitization rule.
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
