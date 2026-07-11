// Package server assembles the HTTP handler chain: SPA static-file serving
// (embedded frontend build, with SPA fallback and cache-header rules) wrapped
// in the correlation and recover middleware every later ticket's endpoints
// (/send, /clientlog, /ws) inherit by being registered on the same mux.
//
// Middleware order (outermost first) is fixed in New: correlation, then
// recover. correlation must be outermost so the request's id is already in
// r.Context() by the time recover's deferred func runs — recover logs with
// that context, so a panic anywhere downstream still gets tagged with the
// correlation id. The tradeoff: a panic inside the correlation middleware
// itself (setting a header, generating an id) would not be caught, but that
// code is trivial by design specifically to keep this risk negligible.
package server
