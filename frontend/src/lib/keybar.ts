/**
 * Sticky-modifier key encoding for the accessory key bar (ticket 4).
 *
 * Why this exists: a touch keyboard has no Ctrl/Alt/Fn keys and cannot
 * chord (hold Ctrl while tapping C). The accessory bar fakes chording with
 * "sticky" one-shot modifiers — tap Ctrl, it latches, then the NEXT
 * keystroke (from either another key-bar button or the soft keyboard
 * itself, via xterm's textarea) gets the modifier applied and the latch
 * clears. This module is the pure encoding half of that: given a logical
 * key and a modifier state, produce the exact byte sequence a real
 * hardware Ctrl/Alt/Fn combo would send. It has zero DOM dependency so it
 * is unit-testable without a browser (see keybar.test.ts) — the primary
 * testable seam for this ticket, same pattern as frames.ts.
 *
 * The escape sequences below are lifted from xterm.js's own
 * evaluateKeyboardEvent (the code that runs for a REAL hardware keypress),
 * so a key-bar tap and a physical Ctrl+C key down produce byte-identical
 * output — Herdr can't tell them apart.
 */

/** Which sticky modifiers are currently latched. */
export interface ModifierState {
  ctrl: boolean
  alt: boolean
  fn: boolean
}

const ARROW_FINAL: Record<string, string> = {
  ArrowUp: 'A',
  ArrowDown: 'B',
  ArrowRight: 'C',
  ArrowLeft: 'D',
  Home: 'H',
  End: 'F',
}

const PAGE_NUM: Record<string, string> = {
  PageUp: '5',
  PageDown: '6',
}

const F1_4_FINAL: Record<string, string> = {
  F1: 'P',
  F2: 'Q',
  F3: 'R',
  F4: 'S',
}

// xterm's own numeric codes for F5-F12 (not contiguous — an xterm/VT
// quirk, not a typo).
const F5_12_NUM: Record<string, string> = {
  F5: '15',
  F6: '17',
  F7: '18',
  F8: '19',
  F9: '20',
  F10: '21',
  F11: '23',
  F12: '24',
}

/**
 * Unmodified byte sequence for every named (non-printable) key the bar
 * exposes. Row 2's punctuation keys (`/`, `-`, `|`, `~`, `:`) are plain
 * printable characters and deliberately not listed here — they fall
 * through applyModifiers' printable-literal branch below.
 */
export const SPECIAL_KEYS: Record<string, string> = {
  Escape: '\x1b',
  Tab: '\t',
  ...Object.fromEntries(Object.entries(ARROW_FINAL).map(([k, f]) => [k, `\x1b[${f}`])),
  ...Object.fromEntries(Object.entries(PAGE_NUM).map(([k, n]) => [k, `\x1b[${n}~`])),
  ...Object.fromEntries(Object.entries(F1_4_FINAL).map(([k, f]) => [k, `\x1bO${f}`])),
  ...Object.fromEntries(Object.entries(F5_12_NUM).map(([k, n]) => [k, `\x1b[${n}~`])),
}

const FN_ARROW: Record<string, string> = {
  ArrowUp: 'PageUp',
  ArrowDown: 'PageDown',
  ArrowLeft: 'Home',
  ArrowRight: 'End',
}

const FN_DIGIT: Record<string, string> = {
  '1': 'F1',
  '2': 'F2',
  '3': 'F3',
  '4': 'F4',
  '5': 'F5',
  '6': 'F6',
  '7': 'F7',
  '8': 'F8',
  '9': 'F9',
  '0': 'F10',
}

/**
 * Termux-style Fn remap: Fn+arrow -> Home/End/PgUp/PgDn, Fn+digit ->
 * F1-F10. Chosen because the design doc explicitly calls this bar
 * "Termux-style" and this is Termux's own Fn convention. Fn takes
 * precedence over Ctrl/Alt (a remapped key's own modified forms aren't
 * reachable from the bar) — a real narrowing, not a bug: add
 * Fn+Ctrl/Fn+Alt combos here if a future key ever needs them.
 */
function fnRemap(key: string): string | null {
  const arrow = FN_ARROW[key]
  if (arrow) return SPECIAL_KEYS[arrow]
  const digit = FN_DIGIT[key]
  if (digit) return SPECIAL_KEYS[digit]
  return null
}

/**
 * Encodes `key` (either a SPECIAL_KEYS name like "ArrowUp"/"F5", or a
 * single printable character typed on the soft keyboard / a row-2
 * punctuation button) as the byte sequence Herdr should receive, given
 * the latched modifiers. Pure — no state, no I/O, safe to call from a
 * hot path (every physical keystroke) or from a key-bar button handler.
 *
 * Ctrl is only meaningful for letters (A-Z -> 0x01-0x1a, matching real
 * terminals); Ctrl on punctuation/other printables is silently ignored
 * rather than guessing — a deliberate gap, not every Ctrl+punct combo has
 * a universal terminal meaning.
 */
export function applyModifiers(key: string, mods: ModifierState): string {
  if (mods.fn) {
    const remapped = fnRemap(key)
    if (remapped) return remapped
  }

  // xterm's own modifier parameter: 1 + shift(1) + alt(2) + ctrl(4). The
  // key bar has no shift, so this only ever adds 2 and/or 4.
  const modCode = 1 + (mods.alt ? 2 : 0) + (mods.ctrl ? 4 : 0)

  if (key in ARROW_FINAL) {
    return modCode > 1 ? `\x1b[1;${modCode}${ARROW_FINAL[key]}` : SPECIAL_KEYS[key]
  }
  if (key in PAGE_NUM) {
    return modCode > 1 ? `\x1b[${PAGE_NUM[key]};${modCode}~` : SPECIAL_KEYS[key]
  }
  if (key in F1_4_FINAL) {
    return modCode > 1 ? `\x1b[1;${modCode}${F1_4_FINAL[key]}` : SPECIAL_KEYS[key]
  }
  if (key in F5_12_NUM) {
    return modCode > 1 ? `\x1b[${F5_12_NUM[key]};${modCode}~` : SPECIAL_KEYS[key]
  }
  if (key === 'Escape') {
    // xterm only special-cases Alt here (Alt+Esc = double Esc); Ctrl+Esc
    // has no distinct terminal meaning and is sent as plain Esc.
    return mods.alt ? '\x1b\x1b' : '\x1b'
  }
  if (key === 'Tab') {
    return mods.alt ? '\x1b\t' : '\t'
  }

  // Printable literal: a soft-keyboard keystroke, or a row-2 punctuation
  // button (/, -, |, ~, :).
  let out = key
  if (mods.ctrl && /^[a-zA-Z]$/.test(key)) {
    out = String.fromCharCode(key.toUpperCase().charCodeAt(0) - 64)
  }
  if (mods.alt) {
    out = '\x1b' + out
  }
  return out
}

/** Long-press alternates for row-2 punctuation keys (design doc: "long-press for alternates"). */
export const ALTERNATES: Record<string, string> = {
  '-': '_',
  ':': ';',
  '|': '\\',
  '~': '`',
  '/': '\\',
}

/**
 * Owns the sticky-modifier latch. This is a plain class, not a Svelte
 * store, so it can be shared between KeyBar.svelte (which toggles/reads
 * it for the UI highlight) and terminal.ts's term.onData hot path (which
 * must consult it on every physical keystroke) without either module
 * depending on Svelte reactivity or the other's internals.
 *
 * State machine: toggle() flips exactly one flag and never clears the
 * others (Ctrl+Alt can be latched together). consume() and clear() are
 * the only ways a latch clears. consume() always resets to all-false
 * after computing the result, whether or not any modifier was actually
 * set, so a stray consume() can never leave a stale latch armed for a
 * later, unrelated keystroke. clear() is the same reset without
 * consuming a keystroke, for "tap the terminal while a modifier is
 * latched" — that tap isn't a character for the modifier to apply to, it's
 * the user cancelling the one-shot chord.
 */
export interface StickyModifiers {
  readonly state: ModifierState
  toggle(name: keyof ModifierState): void
  /** Applies the current latch to `key`, then clears the latch. */
  consume(key: string): string
  /**
   * Force-clears any latched modifier without consuming a keystroke —
   * ticket "tap TUI while Ctrl active cancels Ctrl": a plain tap on the
   * terminal is not a keystroke for consume() to apply the modifier to,
   * it's the user backing out of the one-shot chord entirely. No-ops (and
   * does not notify) when nothing is latched, matching consume()'s own
   * only-notify-on-a-live-latch rule so an unconditional call from a
   * click handler can't spam subscribers on every ordinary tap.
   */
  clear(): void
  /** Notified on every toggle() and every consume()/clear() that cleared a live latch. */
  subscribe(cb: (s: ModifierState) => void): () => void
}

export function createStickyModifiers(): StickyModifiers {
  let state: ModifierState = { ctrl: false, alt: false, fn: false }
  const listeners = new Set<(s: ModifierState) => void>()

  function notify() {
    for (const cb of listeners) cb(state)
  }

  return {
    get state() {
      return state
    },
    toggle(name) {
      state = { ...state, [name]: !state[name] }
      notify()
    },
    consume(key) {
      const result = applyModifiers(key, state)
      if (state.ctrl || state.alt || state.fn) {
        state = { ctrl: false, alt: false, fn: false }
        notify()
      }
      return result
    },
    clear() {
      if (state.ctrl || state.alt || state.fn) {
        state = { ctrl: false, alt: false, fn: false }
        notify()
      }
    },
    subscribe(cb) {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
  }
}
