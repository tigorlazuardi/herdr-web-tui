import { describe, expect, it } from 'vitest'
import {
  ACCESSORY_CHIPS,
  applyModifiers,
  createStickyModifiers,
  mergeAccessoryOrder,
  moveAccessoryChip,
} from './keybar'

// Table-driven: [description, key, mods, expected bytes]. Expected bytes
// are cross-checked against xterm.js's own evaluateKeyboardEvent output
// for the equivalent hardware keypress (see keybar.ts's doc comment) so a
// key-bar tap is byte-identical to a real Ctrl/Alt combo.
const cases: Array<[string, string, { ctrl?: boolean; alt?: boolean; fn?: boolean }, string]> = [
  ['plain Escape', 'Escape', {}, '\x1b'],
  ['Alt+Escape doubles it', 'Escape', { alt: true }, '\x1b\x1b'],
  ['plain Tab', 'Tab', {}, '\t'],
  ['Alt+Tab', 'Tab', { alt: true }, '\x1b\t'],
  ['plain arrow up', 'ArrowUp', {}, '\x1b[A'],
  ['Ctrl+arrow up', 'ArrowUp', { ctrl: true }, '\x1b[1;5A'],
  ['Alt+arrow left', 'ArrowLeft', { alt: true }, '\x1b[1;3D'],
  ['Ctrl+Alt+arrow right', 'ArrowRight', { ctrl: true, alt: true }, '\x1b[1;7C'],
  ['plain Home', 'Home', {}, '\x1b[H'],
  ['plain End', 'End', {}, '\x1b[F'],
  ['plain PageUp', 'PageUp', {}, '\x1b[5~'],
  ['Ctrl+PageDown', 'PageDown', { ctrl: true }, '\x1b[6;5~'],
  ['plain F1', 'F1', {}, '\x1bOP'],
  ['Ctrl+F1', 'F1', { ctrl: true }, '\x1b[1;5P'],
  ['plain F5', 'F5', {}, '\x1b[15~'],
  ['Ctrl+F5', 'F5', { ctrl: true }, '\x1b[15;5~'],
  ['plain F12', 'F12', {}, '\x1b[24~'],
  ['ctrl+c -> ETX', 'c', { ctrl: true }, '\x03'],
  ['ctrl+b (prefix) -> STX', 'b', { ctrl: true }, '\x02'],
  ['ctrl on uppercase letter', 'C', { ctrl: true }, '\x03'],
  ['plain punctuation passes through', '/', {}, '/'],
  ['ctrl+punctuation is ignored (no universal meaning)', '/', { ctrl: true }, '/'],
  ['alt+letter prefixes ESC', 'x', { alt: true }, '\x1bx'],
  ['fn+arrow remaps to PageUp/Down/Home/End', 'ArrowDown', { fn: true }, '\x1b[6~'],
  ['fn+digit remaps to F-key', '1', { fn: true }, '\x1bOP'],
  ['fn takes precedence over ctrl/alt', 'ArrowUp', { fn: true, ctrl: true }, '\x1b[5~'],
]

describe('applyModifiers', () => {
  for (const [name, key, mods, expected] of cases) {
    it(name, () => {
      expect(applyModifiers(key, { ctrl: !!mods.ctrl, alt: !!mods.alt, fn: !!mods.fn })).toBe(
        expected,
      )
    })
  }
})

describe('accessory rail order', () => {
  it('keeps valid stored ids, drops junk and appends new defaults', () => {
    const order = mergeAccessoryOrder(['right', 'missing', 'right', 'ctrl'])
    expect(order.slice(0, 2)).toEqual(['right', 'ctrl'])
    expect(order).toHaveLength(ACCESSORY_CHIPS.length)
    expect(new Set(order).size).toBe(ACCESSORY_CHIPS.length)
  })

  it('moves dragged chip before drop target without losing ids', () => {
    expect(moveAccessoryChip(['esc', 'ctrl', 'tab'], 'tab', 'ctrl')).toEqual([
      'esc',
      'tab',
      'ctrl',
    ])
  })
})

describe('createStickyModifiers', () => {
  it('latches ctrl then applies it to the next key and clears', () => {
    const sticky = createStickyModifiers()
    sticky.toggle('ctrl')
    expect(sticky.state).toEqual({ ctrl: true, alt: false, fn: false })

    expect(sticky.consume('c')).toBe('\x03')
    expect(sticky.state).toEqual({ ctrl: false, alt: false, fn: false })
  })

  it('does not latch across an unrelated consume once cleared', () => {
    const sticky = createStickyModifiers()
    sticky.toggle('ctrl')
    sticky.consume('c') // consumes + clears
    expect(sticky.consume('d')).toBe('d') // plain, no ctrl this time
  })

  it('toggle twice un-latches (tap Ctrl, tap Ctrl again = cancel)', () => {
    const sticky = createStickyModifiers()
    sticky.toggle('ctrl')
    sticky.toggle('ctrl')
    expect(sticky.state.ctrl).toBe(false)
    expect(sticky.consume('c')).toBe('c')
  })

  it('multiple modifiers can be latched together', () => {
    const sticky = createStickyModifiers()
    sticky.toggle('ctrl')
    sticky.toggle('alt')
    expect(sticky.state).toEqual({ ctrl: true, alt: true, fn: false })
    expect(sticky.consume('ArrowRight')).toBe('\x1b[1;7C')
  })

  it('notifies subscribers on toggle and on a clearing consume', () => {
    const sticky = createStickyModifiers()
    const seen: boolean[] = []
    const unsubscribe = sticky.subscribe((s) => seen.push(s.ctrl))
    sticky.toggle('ctrl')
    sticky.consume('c')
    unsubscribe()
    expect(seen).toEqual([true, false])
  })

  it('clear() un-latches a live modifier and notifies (tap TUI cancels Ctrl)', () => {
    const sticky = createStickyModifiers()
    const seen: boolean[] = []
    const unsubscribe = sticky.subscribe((s) => seen.push(s.ctrl))
    sticky.toggle('ctrl')
    sticky.clear()
    unsubscribe()
    expect(sticky.state).toEqual({ ctrl: false, alt: false, fn: false })
    expect(seen).toEqual([true, false])
  })

  it('clear() with nothing latched is a silent no-op (no spurious notify)', () => {
    const sticky = createStickyModifiers()
    const seen: boolean[] = []
    const unsubscribe = sticky.subscribe((s) => seen.push(s.ctrl))
    sticky.clear()
    unsubscribe()
    expect(sticky.state).toEqual({ ctrl: false, alt: false, fn: false })
    expect(seen).toEqual([])
  })
})
