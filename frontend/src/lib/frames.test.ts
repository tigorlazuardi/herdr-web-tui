import { describe, expect, it } from 'vitest'
import {
  FRAME_ERROR,
  FRAME_INPUT,
  FRAME_OUTPUT,
  decodeErrorMessage,
  decodeFrame,
  encodeFrame,
  encodeInput,
  encodeResize,
  encodeResizeFrame,
} from './frames'

describe('encodeFrame / decodeFrame', () => {
  it('round-trips type and payload', () => {
    const data = new TextEncoder().encode('hello')
    const encoded = encodeFrame(FRAME_OUTPUT, data)
    const { type, data: decoded } = decodeFrame(encoded)
    expect(type).toBe(FRAME_OUTPUT)
    expect(decoded).toEqual(data)
  })

  it('round-trips an empty payload', () => {
    const encoded = encodeFrame(FRAME_INPUT, new Uint8Array())
    const { type, data } = decodeFrame(encoded)
    expect(type).toBe(FRAME_INPUT)
    expect(data.length).toBe(0)
  })

  it('throws on an empty message', () => {
    expect(() => decodeFrame(new Uint8Array())).toThrow()
  })
})

describe('encodeResize', () => {
  it('packs cols/rows big-endian and matches the Go side layout', () => {
    // 80x24 -> [0x00,0x50, 0x00,0x18]
    expect(Array.from(encodeResize(80, 24))).toEqual([0x00, 0x50, 0x00, 0x18])
  })

  it('handles values requiring the high byte', () => {
    expect(Array.from(encodeResize(300, 1))).toEqual([0x01, 0x2c, 0x00, 0x01])
  })
})

describe('encodeResizeFrame', () => {
  it('prefixes FRAME_RESIZE and encodes the size', () => {
    const frame = encodeResizeFrame(80, 24)
    expect(frame[0]).toBe(0x72)
    expect(Array.from(frame.subarray(1))).toEqual([0x00, 0x50, 0x00, 0x18])
  })
})

describe('encodeInput', () => {
  it('prefixes FRAME_INPUT and utf8-encodes text', () => {
    const frame = encodeInput('a')
    expect(frame[0]).toBe(FRAME_INPUT)
    expect(Array.from(frame.subarray(1))).toEqual([0x61])
  })
})

describe('decodeErrorMessage', () => {
  it('decodes a FRAME_ERROR payload back to a string', () => {
    const { data } = decodeFrame(encodeFrame(FRAME_ERROR, new TextEncoder().encode('boom')))
    expect(decodeErrorMessage(data)).toBe('boom')
  })
})
