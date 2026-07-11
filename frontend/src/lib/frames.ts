/**
 * Wire format for the /ws pty↔browser stream. Mirrors
 * internal/server/frames.go exactly — a change here must be mirrored there
 * (and vice versa) or the two sides silently stop understanding each
 * other's frames.
 *
 * Every websocket binary message is `[type byte][payload...]`. This module
 * has zero DOM/xterm dependency so it can be unit tested (and, later,
 * reused by ticket 6's transport) without a browser.
 */

export const FRAME_OUTPUT = 0x6f // 'o' — pty stdout/stderr, server -> client
export const FRAME_INPUT = 0x69 // 'i' — keystrokes/paste, client -> server
export const FRAME_RESIZE = 0x72 // 'r' — 4-byte cols/rows, client -> server
export const FRAME_ERROR = 0x65 // 'e' — human-readable error, server -> client

export type FrameType =
  | typeof FRAME_OUTPUT
  | typeof FRAME_INPUT
  | typeof FRAME_RESIZE
  | typeof FRAME_ERROR

/** Prepends a type byte to data. Pure; used on every keystroke and resize. */
export function encodeFrame(type: FrameType, data: Uint8Array): Uint8Array {
  const out = new Uint8Array(data.length + 1)
  out[0] = type
  out.set(data, 1)
  return out
}

/** Splits a raw websocket message back into its type byte and payload. */
export function decodeFrame(msg: Uint8Array): { type: number; data: Uint8Array } {
  if (msg.length === 0) {
    throw new Error('empty frame')
  }
  return { type: msg[0], data: msg.subarray(1) }
}

/** Packs a terminal size as the FRAME_RESIZE payload: cols, rows as big-endian uint16s. */
export function encodeResize(cols: number, rows: number): Uint8Array {
  return new Uint8Array([
    (cols >> 8) & 0xff,
    cols & 0xff,
    (rows >> 8) & 0xff,
    rows & 0xff,
  ])
}

const textEncoder = new TextEncoder()
const textDecoder = new TextDecoder()

/** Encodes a JS string as an input frame ready to send over the ws. */
export function encodeInput(text: string): Uint8Array {
  return encodeFrame(FRAME_INPUT, textEncoder.encode(text))
}

/** Encodes a resize as a ready-to-send frame. */
export function encodeResizeFrame(cols: number, rows: number): Uint8Array {
  return encodeFrame(FRAME_RESIZE, encodeResize(cols, rows))
}

/** Decodes an error frame's payload back to a human-readable string. */
export function decodeErrorMessage(data: Uint8Array): string {
  return textDecoder.decode(data)
}
