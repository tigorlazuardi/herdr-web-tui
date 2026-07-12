package server

import "testing"

// TestSanitizeSession_TableDriven covers the allowlist + fallback behaviour
// both newPTYHandler (the /ws?session=... query param) and sendHandler
// (the /send "session" multipart field) rely on. A single table here is
// the one place both callers' contract is pinned down, so a change to the
// allowlist can't silently diverge between the render pty and the inject
// daemon.
func TestSanitizeSession_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty falls back to default", "", defaultSession},
		{"simple alnum name passes through", "work", "work"},
		{"hyphenated name passes through", "foo-bar-99", "foo-bar-99"},
		{"path separator rejected", "../etc/passwd", defaultSession},
		{"space rejected", "not valid", defaultSession},
		{"shell metacharacter rejected", "work; rm -rf /", defaultSession},
		{"leading slash rejected", "/work", defaultSession},
		{"query string leftover rejected", "work?x=1", defaultSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeSession(tt.input); got != tt.want {
				t.Fatalf("sanitizeSession(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
