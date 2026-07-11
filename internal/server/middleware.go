package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/tigorlazuardi/herdr-web-tui/internal/correlation"
)

// withCorrelation assigns a correlation id to every request and threads it
// through r.Context() to next. It echoes an inbound X-Request-Id if the
// client (or an upstream proxy) already set one — useful when nginx or a
// caller generates its own trace id and wants this service to join the same
// trace rather than starting a new one — otherwise it generates a fresh
// rs/xid. The id is written to both X-Request-Id and X-Correlation-Id on
// every response (not just errors) so any response can be quoted back for a
// log lookup.
func withCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(correlation.HeaderRequestID)
		if id == "" {
			id = correlation.NewID()
		}
		w.Header().Set(correlation.HeaderRequestID, id)
		w.Header().Set(correlation.HeaderCorrelationID, id)

		ctx := correlation.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withRecover catches a panic anywhere downstream in the handler chain,
// logs it with runtime/debug.Stack() (the free, native goroutine stack —
// the only stack trace a panic gives us) plus the request's correlation id,
// and returns 500 instead of letting the panic unwind past net/http and
// crash the whole process. This service serves every session concurrently
// on the same process, so one bad request/handler bug must not take every
// other session down with it.
//
// Must run after withCorrelation in the chain (see doc.go) so the
// correlation id is already in r.Context() when the deferred recover fires.
func withRecover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
						slog.String("stack", string(debug.Stack())),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
