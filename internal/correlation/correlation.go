package correlation

import (
	"context"

	"github.com/rs/xid"
)

// Header names used to carry the correlation id between browser and server.
// X-Request-Id is the canonical name (and the one echoed if the client sent
// it); X-Correlation-Id is set to the same value for clients that look for
// that name instead. Both are returned on every response, not just errors,
// so a client can always quote the id that produced a given response.
const (
	HeaderRequestID     = "X-Request-Id"
	HeaderCorrelationID = "X-Correlation-Id"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	connIDKey
)

// NewID generates a new time-ordered, URL-safe correlation id (rs/xid).
// Called once per inbound request by the correlation middleware; later
// tickets call it again for per-websocket-connection ids using the same
// package.
func NewID() string {
	return xid.New().String()
}

// WithRequestID returns a copy of ctx carrying id as the request's
// correlation id. The middleware is the only expected caller — handlers and
// downstream code should read the id back with RequestID, not set it.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the correlation id stored in ctx, or "" if none was set
// (e.g. code running outside the middleware chain, such as in a unit test
// that builds its own context). Callers should treat "" as "no id available"
// rather than an error.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// WithConnID returns a copy of ctx carrying id as the current websocket
// connection's correlation id. Unlike the per-request id (one per HTTP
// request), a conn id is generated once per websocket connection and lives
// for the whole pty session — see internal/server's ws handler, the only
// expected caller.
func WithConnID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, connIDKey, id)
}

// ConnID returns the websocket connection id stored in ctx, or "" if none
// was set (e.g. an HTTP request that never upgraded to a websocket).
func ConnID(ctx context.Context) string {
	id, _ := ctx.Value(connIDKey).(string)
	return id
}
