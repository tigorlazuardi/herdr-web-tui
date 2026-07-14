import { describe, expect, it } from 'vitest'
import { clampFontSize, readStoredFontSize } from './terminal'

// Only the font +/- lever's clamp logic is pure/testable without a DOM +
// WebSocket (see clampFontSize's doc comment in terminal.ts); the rest of
// createTerminalBridge is exercised manually/on-device per tickets.md ("3.
// Mobile/touch hardening" acceptance: "Verified on-device").
describe('clampFontSize', () => {
  it('increases by delta within range', () => {
    expect(clampFontSize(15, 1)).toBe(16)
  })

  it('decreases by delta within range', () => {
    expect(clampFontSize(15, -1)).toBe(14)
  })

  it('floors at MIN_FONT_SIZE (8)', () => {
    expect(clampFontSize(8, -1)).toBe(8)
  })

  it('ceilings at MAX_FONT_SIZE (32)', () => {
    expect(clampFontSize(32, 1)).toBe(32)
  })
})

describe('readStoredFontSize', () => {
  it('accepts a valid in-range value', () => {
    expect(readStoredFontSize('20')).toBe(20)
  })

  it('falls back to default when null (nothing stored yet)', () => {
    expect(readStoredFontSize(null)).toBe(15)
  })

  it('falls back to default when out of range', () => {
    expect(readStoredFontSize('100')).toBe(15)
    expect(readStoredFontSize('0')).toBe(15)
  })

  it('falls back to default on garbage input', () => {
    expect(readStoredFontSize('not-a-number')).toBe(15)
  })
})
