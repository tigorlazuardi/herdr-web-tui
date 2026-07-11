// Package pty spawns "herdr --session <name>" in a pseudo-terminal and
// bridges its byte stream to a websocket connection.
//
// Lifecycle: Bridge.Run is handed a context.Context and never outlives it.
// The pty-reader goroutine it starts returns as soon as either the pty read
// fails (herdr exited) or ctx is cancelled (the caller — internal/server's
// ws handler — cancels ctx when the browser's websocket read fails or the
// http request's context is done, i.e. the client navigated away or the
// TCP connection dropped). On return, Run always closes the pty file and
// waits for the herdr process to exit, so a dropped ws connection can never
// leak a pty fd or a zombie herdr process — see Run's doc comment for the
// exact teardown sequence.
//
// This package deliberately knows nothing about websockets or HTTP; it
// only needs an io.ReadWriter-shaped pair of callbacks (see Bridge) so it
// stays testable and reusable if the transport ever changes.
package pty
