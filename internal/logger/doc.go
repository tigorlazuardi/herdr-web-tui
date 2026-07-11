// Package logger builds the process-wide log/slog handler and wraps it so
// every log call made with a context.Context (InfoContext, ErrorContext,
// ...) automatically carries the request's correlation id, without every
// call site remembering to add it.
//
// Format selection is JSON-first (greppable, aggregator-friendly) with an
// optional text fallback for interactive dev sessions, chosen via
// --log-format / LOG_FORMAT, or auto-detected from whether stdout is a TTY
// when neither is set. AddSource is always on: combined with the
// go-faster/errors stack-capturing wrap chain used elsewhere in this
// service, every log line carries both the log call site (AddSource) and,
// for wrapped errors, the origin frame of the failure — the two pieces Go's
// lack of native stack traces would otherwise cost us.
package logger
