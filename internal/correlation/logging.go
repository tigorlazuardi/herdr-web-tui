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
