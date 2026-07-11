package server

import "github.com/go-faster/errors"

// Frame types multiplex the pty↔browser websocket, which otherwise carries
// only opaque bytes: a single binary websocket message can be terminal
// output, terminal input, or a resize instruction, so every message on the
// /ws stream is tagged with a one-byte type prefix.
const (
	// FrameOutput carries raw pty stdout/stderr bytes, server → client.
	FrameOutput byte = 'o'
	// FrameInput carries raw keystroke/paste bytes, client → server.
	FrameInput byte = 'i'
	// FrameResize carries a 4-byte cols/rows payload (see EncodeResize),
	// client → server, sent whenever the browser's fit-addon recomputes
	// the terminal size.
	FrameResize byte = 'r'
	// FrameError carries a human-readable error message, server → client,
	// sent when the pty/herdr spawn fails or herdr becomes unreachable so
	// the frontend can show it instead of hanging on a dead connection.
	FrameError byte = 'e'
)

// EncodeFrame prepends typ to data, producing the wire format for one
// websocket binary message on the /ws stream. Pure and allocation-cheap
// (one prepend) since it runs on every pty read in the hot path.
func EncodeFrame(typ byte, data []byte) []byte {
	out := make([]byte, len(data)+1)
	out[0] = typ
	copy(out[1:], data)
	return out
}

// DecodeFrame splits a raw websocket binary message back into its type tag
// and payload. Errors on an empty message, which is never valid on this
// stream (EncodeFrame always emits at least the type byte).
func DecodeFrame(msg []byte) (typ byte, data []byte, err error) {
	if len(msg) == 0 {
		return 0, nil, errors.New("empty frame")
	}
	return msg[0], msg[1:], nil
}

// EncodeResize packs a terminal size into a FrameResize payload: cols and
// rows as big-endian uint16s. uint16 comfortably covers any real terminal
// (max 65535 cols/rows) while keeping the resize frame fixed-size and
// alloc-free to decode.
func EncodeResize(cols, rows uint16) []byte {
	return []byte{byte(cols >> 8), byte(cols), byte(rows >> 8), byte(rows)}
}

// DecodeResize is EncodeResize's inverse. Errors if data isn't exactly 4
// bytes, which means the client sent a malformed resize frame.
func DecodeResize(data []byte) (cols, rows uint16, err error) {
	if len(data) != 4 {
		return 0, 0, errors.Errorf("resize frame must be 4 bytes, got %d", len(data))
	}
	cols = uint16(data[0])<<8 | uint16(data[1])
	rows = uint16(data[2])<<8 | uint16(data[3])
	return cols, rows, nil
}
