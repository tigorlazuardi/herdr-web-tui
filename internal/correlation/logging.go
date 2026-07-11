package correlation

import (
	"context"
	"log/slog"
)

// Attr returns a slog attribute tagging a log line with ctx's correlation
// id, so every log call made while handling a request can be written as
// `logger.InfoContext(ctx, "msg", correlation.Attr(ctx))` and the id shows
// up on every line without callers remembering to extract it by hand.
func Attr(ctx context.Context) slog.Attr {
	return slog.String("req_id", RequestID(ctx))
}

// ConnAttr returns a slog attribute tagging a log line with ctx's
// websocket connection id (see WithConnID). Used alongside Attr for ws
// lifecycle and pty logging, where both the originating HTTP request id and
// the longer-lived connection id are useful to grep by.
func ConnAttr(ctx context.Context) slog.Attr {
	return slog.String("conn_id", ConnID(ctx))
}
