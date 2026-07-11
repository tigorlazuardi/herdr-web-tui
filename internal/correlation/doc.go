// Package correlation generates and threads correlation identifiers through
// a request's context.Context so every log line, response header, and (in
// later tickets) websocket frame emitted while handling that request can be
// tied back to one flow.
//
// Why this exists: this service fronts a live Herdr server and the
// bug-prone path (artifact inject, ticket 5) shells out to another process.
// When something breaks, the only practical debugging tool is grepping logs
// for one id. The id is generated once per request (or once per websocket
// connection, in later tickets) and stored in the context so every
// downstream call site — handler, HerdrClient, logger — can pull it out
// without threading an extra parameter through every function signature.
//
// IDs are rs/xid values: 12-byte, time-ordered, URL-safe, and sortable by
// generation time, which makes "what happened around this request" greppable
// by eyeballing adjacent ids even without a log aggregator.
package correlation
