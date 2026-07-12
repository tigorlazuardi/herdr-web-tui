import { describe, expect, it } from 'vitest'
import { sniffMime } from './sniff'

describe('sniffMime', () => {
  it('detects PNG by signature', () => {
    expect(sniffMime(new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0]))).toBe(
      'image/png',
    )
  })

  it('detects PDF by the %PDF magic bytes', () => {
    expect(sniffMime(new TextEncoder().encode('%PDF-1.7 ...'))).toBe('application/pdf')
  })

  it('detects WEBP via the split RIFF/WEBP signature', () => {
    const bytes = new Uint8Array(16)
    bytes.set([0x52, 0x49, 0x46, 0x46], 0) // "RIFF"
    bytes.set([0x57, 0x45, 0x42, 0x50], 8) // "WEBP"
    expect(sniffMime(bytes)).toBe('image/webp')
  })

  it('returns empty string for unrecognized bytes', () => {
    expect(sniffMime(new Uint8Array([1, 2, 3, 4]))).toBe('')
  })

  it('returns empty string for input shorter than any signature', () => {
    expect(sniffMime(new Uint8Array([0x89]))).toBe('')
  })
})
