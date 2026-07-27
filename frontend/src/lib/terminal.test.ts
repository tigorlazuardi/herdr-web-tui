import { describe, expect, it } from 'vitest'
import {
  clampFontSize,
  initializeAfterTerminalFont,
  openKeyboard,
  readStoredFontSize,
  TERMINAL_FONT_FAMILY,
  TERMINAL_FONT_SAMPLE,
} from './terminal'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve
    reject = onReject
  })
  return { promise, resolve, reject }
}

// Only the font +/- lever's clamp logic is pure/testable without a DOM +
// WebSocket (see clampFontSize's doc comment in terminal.ts); the rest of
// createTerminalBridge is exercised manually/on-device per tickets.md ("3.
// Mobile/touch hardening" acceptance: "Verified on-device").
describe('terminal font family', () => {
  it('keeps system monospace primary and Symbols Nerd Font Mono as PUA fallback', () => {
    expect(TERMINAL_FONT_FAMILY).toContain('ui-monospace')
    expect(TERMINAL_FONT_FAMILY).toContain('"Symbols Nerd Font Mono"')
    expect(TERMINAL_FONT_FAMILY.indexOf('ui-monospace')).toBeLessThan(
      TERMINAL_FONT_FAMILY.indexOf('"Symbols Nerd Font Mono"'),
    )
  })
})

describe('terminal font gate', () => {
  it('requests Symbols Nerd Font Mono with BMP and supplementary PUA samples', async () => {
    const requests: Array<[string, string | undefined]> = []
    await initializeAfterTerminalFont(
      { load: (font, text) => { requests.push([font, text]); return Promise.resolve([]) } },
      () => false,
      () => {},
    )
    expect(requests).toEqual([[ '15px "Symbols Nerd Font Mono"', TERMINAL_FONT_SAMPLE ]])
    expect(TERMINAL_FONT_SAMPLE).toBe('\uE0B0\u{F0001}')
  })

  it('holds initialization until font load resolves', async () => {
    const load = deferred<FontFace[]>()
    let initialized = false
    const pending = initializeAfterTerminalFont({ load: () => load.promise }, () => false, () => { initialized = true })
    expect(initialized).toBe(false)
    load.resolve([])
    await pending
    expect(initialized).toBe(true)
  })

  it('initializes after resolved or rejected font load', async () => {
    let resolved = 0
    await initializeAfterTerminalFont({ load: () => Promise.resolve([]) }, () => false, () => { resolved++ })
    let rejected = 0
    await initializeAfterTerminalFont({ load: () => Promise.reject(new Error('blocked')) }, () => false, () => { rejected++ })
    expect([resolved, rejected]).toEqual([1, 1])
  })

  it('does not initialize when closed before font load resolves', async () => {
    const load = deferred<FontFace[]>()
    let closed = false
    let initialized = false
    const pending = initializeAfterTerminalFont({ load: () => load.promise }, () => closed, () => { initialized = true })
    closed = true
    load.resolve([])
    await pending
    expect(initialized).toBe(false)
  })

})

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

describe('openKeyboard', () => {
  it('applies text input mode before resetting focus on the next frame', () => {
    const calls: string[] = []
    let scheduled: (() => void) | undefined
    openKeyboard(
      {
        setAttribute: (name, value) => calls.push(`set:${name}=${value}`),
        blur: () => calls.push('blur'),
        focus: () => calls.push('focus'),
      },
      (cb) => {
        scheduled = cb
        return 1
      },
    )
    expect(calls).toEqual(['set:inputmode=text'])
    scheduled?.()
    expect(calls).toEqual(['set:inputmode=text', 'blur', 'focus'])
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
