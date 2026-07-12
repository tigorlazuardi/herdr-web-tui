import { describe, expect, it } from 'vitest'
import { clampFontSize } from './terminal'

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
