/**
 * Terminal bridge: owns one xterm.js instance and its websocket connection
 * to /ws. This module is intentionally NOT a Svelte component or store —
 * pty output can arrive dozens of times per second during a fast command,
 * and routing every byte through Svelte's reactivity (a store update, a
 * component re-render) would put the framework in the hot path for no
 * benefit, since xterm already does its own efficient terminal rendering.
 * `connect()` returns a plain object; App.svelte calls it once in
 * `onMount` and never touches xterm's internals reactively again.
 *
 * ctx-lifecycle gotcha: there is no AbortController/ctx.Done() on the
 * browser side the way there is in Go, so this module's equivalent is
 * `TerminalBridge.close()` — call it from `onDestroy`/`beforeNavigate` or
 * the ws's own `onclose`/`onerror` handlers won't fire again and the pty
 * on the server will keep running until the TCP connection times out.
 * Always pair `connect()` with a `close()` on unmount.
 */

import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import '@xterm/xterm/css/xterm.css'
import {
  FRAME_ERROR,
  FRAME_OUTPUT,
  decodeErrorMessage,
  decodeFrame,
  encodeInput,
  encodeResizeFrame,
} from './frames'
import { sessionFromPath } from './session'
import type { StickyModifiers } from './keybar'

export interface TerminalBridge {
  /** Mounts xterm into el and starts sizing/streaming. */
  attach(el: HTMLElement): void
  /** Tears down the websocket and disposes the xterm instance. Idempotent. */
  close(): void
  /** True once the initial ws connection has been accepted. */
  readonly connected: boolean
  /** Subscribes to connection-state changes (for a "reconnecting…" banner). */
  onStateChange(cb: (state: ConnectionState) => void): () => void
  /**
   * Nudges xterm's font size by delta px (clamped to [8, 32]) and re-fits,
   * which recomputes cols/rows for the new cell size and — via the
   * existing term.onResize -> ws resize-frame wiring below — sends the
   * pty a SIGWINCH at the new size. This is the "optional font +/- lever"
   * from the design doc: a bigger font makes each cell wider, so *fewer*
   * columns fit in the same viewport width — the escape hatch to force
   * Herdr's single-column mobile layout (`mobile_width_threshold`,
   * default 64 cols) on a physically large phone/tablet whose viewport
   * alone doesn't measure narrow enough. Smaller font goes the other way
   * (more columns fit); both directions are exposed since this is a
   * general readability control, not a mobile-only trigger.
   */
  adjustFontSize(delta: number): void
  /**
   * Sends `text` over the same FRAME_INPUT path as a physical keystroke
   * (term.onData below) — ticket 4's accessory key bar calls this instead
   * of forking its own ws-send logic, so a key-bar tap and a hardware
   * keypress are indistinguishable to the server. No-op while disconnected
   * (mirrors term.onData's own readyState guard).
   */
  sendInput(text: string): void
  /** Refocuses xterm's hidden textarea, e.g. after a key-bar button steals focus. */
  focus(): void
  /** Opens mobile IME from a synchronous user-gesture handler. */
  openKeyboard(): void
  /**
   * Dismisses the soft keyboard by blurring xterm's hidden textarea — used
   * when switching to keys mode so the accessory bar isn't fighting the
   * on-screen keyboard for space.
   */
  blur(): void
  /**
   * Sets `inputmode="none"` on xterm's hidden textarea so the browser
   * keeps the on-screen keyboard closed even while the element is
   * focused. The terminal is mouse-first (every pane/tab/border is
   * clickable), so a one-shot blur() on entering keys mode isn't enough —
   * the next tap on a pane re-focuses the textarea (xterm's own click
   * handling) and a plain focus would normally re-summon the soft
   * keyboard. inputmode="none" keeps that click-to-focus behaviour (so
   * pane switching by tap still works) while telling the keyboard to stay
   * shut. Call on entering keys mode; pair with restoreKeyboard() on the
   * way back out.
   */
  suppressKeyboard(): void
  /**
   * Removes the `inputmode="none"` set by suppressKeyboard(), restoring
   * the browser's default keyboard-on-focus behaviour — call on returning
   * to promptbox mode so tapping the terminal directly still opens the
   * soft keyboard for typing.
   */
  restoreKeyboard(): void
}

const DEFAULT_FONT_SIZE = 15
const MIN_FONT_SIZE = 8
const MAX_FONT_SIZE = 32

/**
 * Pure clamp for the font +/- lever, split out from adjustFontSize so it's
 * testable without constructing a Terminal/WebSocket (xterm.js needs no
 * DOM to construct, but a real bridge still opens a socket in attach()) —
 * see terminal.test.ts.
 */
export function clampFontSize(current: number, delta: number): number {
  return Math.min(MAX_FONT_SIZE, Math.max(MIN_FONT_SIZE, current + delta))
}

const FONT_SIZE_KEY = 'herdr-web-tui:fontSize'

/**
 * Pure parse+clamp for the font size persisted in localStorage, split out
 * so it's testable without touching localStorage/DOM — see
 * terminal.test.ts. A stored value that's missing, non-numeric, or
 * outside [MIN_FONT_SIZE, MAX_FONT_SIZE] (e.g. hand-edited devtools
 * storage, or a size from a future build with different bounds) falls
 * back to DEFAULT_FONT_SIZE rather than feeding xterm a bad initial size.
 */
export function readStoredFontSize(raw: string | null): number {
  if (raw === null) return DEFAULT_FONT_SIZE
  const parsed = Number.parseInt(raw, 10)
  if (!Number.isFinite(parsed) || parsed < MIN_FONT_SIZE || parsed > MAX_FONT_SIZE) {
    return DEFAULT_FONT_SIZE
  }
  return parsed
}

/**
 * Restores text input, then re-focuses after Chromium applies inputmode.
 * `inputmode="none"` is cached for the current focus transition on mobile;
 * focusing in the same task leaves the IME hidden. Termux mode proves this
 * browser permits async focus, so one animation frame is the minimal reset.
 */
export function openKeyboard(
  textarea: Pick<HTMLTextAreaElement, 'setAttribute' | 'blur' | 'focus'> | undefined,
  schedule: (cb: () => void) => number = requestAnimationFrame,
) {
  if (!textarea) return
  textarea.setAttribute('inputmode', 'text')
  schedule(() => {
    textarea.blur()
    textarea.focus()
  })
}

/**
 * Reads the persisted font size, guarded because localStorage can throw
 * (private/incognito mode, disabled storage, quota errors) — a readability
 * preference must never crash terminal construction.
 */
function loadFontSize(): number {
  try {
    return readStoredFontSize(localStorage.getItem(FONT_SIZE_KEY))
  } catch {
    return DEFAULT_FONT_SIZE
  }
}

/**
 * Persists the font size so it survives a reload (per-device readability
 * preference, see adjustFontSize's doc comment) — guarded for the same
 * reason as loadFontSize; a failed write just means the next reload falls
 * back to DEFAULT_FONT_SIZE, which is harmless.
 */
function saveFontSize(size: number) {
  try {
    localStorage.setItem(FONT_SIZE_KEY, String(size))
  } catch {
    // ignore — see saveFontSize's doc comment
  }
}

export type ConnectionState = 'connecting' | 'open' | 'closed'

// asSendable narrows Uint8Array<ArrayBufferLike> (our encode helpers'
// return type) to the ArrayBuffer-backed view WebSocket.send's TS types
// demand. The underlying bytes are always a plain, non-shared ArrayBuffer
// (they come from `new Uint8Array(n)`), so this is a type-level cast only,
// not a runtime copy.
function asSendable(data: Uint8Array): ArrayBufferView<ArrayBuffer> {
  return data as ArrayBufferView<ArrayBuffer>
}

/**
 * Builds the /ws URL from the current page location: ws(s):// + same host
 * + /ws?session=<name>. The session query param is this ticket's routing
 * mechanism (design doc: "Multi-session concurrency" — URL path selects
 * the Herdr session for both the render pty and the inject daemon): the
 * backend re-derives and re-sanitizes the name itself (sanitizeSession),
 * so a missing/invalid value here is not a client-side validation bug, it
 * just falls back to "default" server-side.
 */
function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const session = sessionFromPath(location.pathname)
  const query = session ? `?session=${encodeURIComponent(session)}` : ''
  return `${proto}//${location.host}/ws${query}`
}

/**
 * Creates a TerminalBridge. Does not connect until attach() is called,
 * since xterm needs a DOM element to open into and the resize handshake
 * (see internal/server/pty.go's readInitialSize) needs a measured
 * FitAddon.proposeDimensions() before the first frame can be sent.
 *
 * `sticky` is the SAME StickyModifiers instance KeyBar.svelte toggles/
 * reads for its UI highlight — passed in rather than created here so a
 * tap-Ctrl-then-type-c on the soft keyboard (term.onData below) and a
 * tap-Ctrl-then-tap-C on the key bar consume the identical latch. Without
 * this, the two input paths would each own a dead latch of their own and
 * Ctrl/Alt/Fn taps would silently do nothing for soft-keyboard typing.
 */
export function createTerminalBridge(sticky: StickyModifiers): TerminalBridge {
  const term = new Terminal({
    cursorBlink: true,
    scrollback: 5000,
    allowProposedApi: true,
    fontSize: loadFontSize(),
  })
  const fit = new FitAddon()
  term.loadAddon(fit)

  let ws: WebSocket | null = null
  let state: ConnectionState = 'connecting'
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let closed = false
  let el: HTMLElement | null = null
  const listeners = new Set<(s: ConnectionState) => void>()

  function setState(s: ConnectionState) {
    state = s
    for (const cb of listeners) cb(s)
  }

  /**
   * Disables mobile IME autocorrect/autocapitalize/spellcheck on xterm's
   * hidden textarea. A terminal consumes raw keystrokes — every byte
   * xterm's onData below forwards goes straight to the pty — but a stock
   * soft keyboard treats the textarea like a prose field: it silently
   * composes and, on a misspelling, fires a delete-the-word-then-retype
   * correction. xterm has no idea that burst wasn't typed by a human, so
   * it forwards the deletes and the replacement as ordinary onData bytes
   * and corrupts whatever was on the command line. These attributes are a
   * standing hint to the browser/IME, independent of and unconditional
   * across both inputMode ('promptbox'/'keys') states — the terminal never
   * wants autocorrect, whereas inputmode="none" (suppressKeyboard/
   * restoreKeyboard above) is the separate, mode-dependent toggle for
   * whether the keyboard shows at all. Applied once per attach(), right
   * after term.open() creates the textarea.
   */
  function hardenTextareaAutocorrect() {
    const ta = term.textarea
    if (!ta) return
    ta.setAttribute('autocorrect', 'off')
    ta.setAttribute('autocapitalize', 'off')
    ta.setAttribute('autocomplete', 'off')
    ta.setAttribute('spellcheck', 'false')
    ta.setAttribute('enterkeyhint', 'enter')
  }

  function tryLoadWebgl() {
    try {
      term.loadAddon(new WebglAddon())
    } catch {
      // WebGL unavailable (e.g. software-rendered browser, some mobile
      // GPUs blocklisted by the browser): xterm falls back to its default
      // DOM/canvas renderer automatically, so this is not fatal — just
      // slower on very fast output.
    }
  }

  function open() {
    setState('connecting')
    ws = new WebSocket(wsURL())
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      setState('open')
      fit.fit()
      // The very first message must be a resize frame — see
      // internal/server/pty.go readInitialSize — so the pty is spawned at
      // the right size instead of a default guess.
      sendResize()
    }

    ws.onmessage = (ev) => {
      const { type, data } = decodeFrame(new Uint8Array(ev.data as ArrayBuffer))
      if (type === FRAME_OUTPUT) {
        term.write(data)
      } else if (type === FRAME_ERROR) {
        term.write(`\r\n\x1b[31m[herdr-web-tui] ${decodeErrorMessage(data)}\x1b[0m\r\n`)
      }
    }

    ws.onclose = () => {
      setState('closed')
      scheduleReconnect()
    }
    ws.onerror = () => {
      // onclose always follows onerror for a WebSocket, so reconnect is
      // scheduled there; this handler exists only so browsers don't log an
      // "uncaught" console error for the expected reconnect-on-drop case.
    }
  }

  function scheduleReconnect() {
    if (closed || reconnectTimer) return
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null
      if (!closed) open()
    }, 1000)
  }

  function sendResize() {
    if (ws?.readyState !== WebSocket.OPEN) return
    const dims = fit.proposeDimensions()
    const cols = dims?.cols ?? term.cols
    const rows = dims?.rows ?? term.rows
    term.resize(cols, rows)
    ws.send(asSendable(encodeResizeFrame(cols, rows)))
  }

  term.onData((data) => {
    if (ws?.readyState === WebSocket.OPEN) {
      // Every physical/soft-keyboard keystroke must consult the sticky
      // latch (see keybar.ts's StickyModifiers doc) so a tap-Ctrl-then-
      // type-c on the soft keyboard sends \x03, same as a key-bar tap.
      // xterm can deliver multiple characters in one onData call (e.g. a
      // paste); consume() is run per-character so the latch, if armed,
      // only ever applies to the first and clears immediately after —
      // consume() on subsequent already-unarmed chars is a no-op passthrough.
      const out = Array.from(data)
        .map((ch) => sticky.consume(ch))
        .join('')
      ws.send(asSendable(encodeInput(out)))
    }
  })
  term.onResize(({ cols, rows }) => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(asSendable(encodeResizeFrame(cols, rows)))
    }
  })

  let resizeObserver: ResizeObserver | null = null

  /**
   * Touch-drag-to-scroll shim. Two facts collide without it: the pane's
   * `touch-action: none` (App.svelte) kills the browser's native pinch/
   * scroll so it doesn't fight xterm's own pointer handling, but xterm.js
   * itself never turns a touch-drag into scroll — it only understands
   * mouse WHEEL events. Herdr is a mouse-first TUI (mouse-tracking mode
   * forwards wheel events to whatever pane/app is under the cursor, e.g.
   * a pager), so a finger dragging up/down on a phone must be translated
   * into synthetic wheel events for xterm/Herdr to see any scroll at all.
   * A ~8px slop distinguishes an intentional drag from a tap, so pane/tab
   * switching (mouse-first, tap-to-click) keeps working: below slop we
   * never preventDefault or dispatch, so the browser synthesizes its own
   * click as usual.
   */
  let touchActive = false
  let touchMoved = false
  let touchStartY = 0
  let touchLastY = 0
  const TOUCH_SLOP = 8

  function onTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) {
      // Multi-touch = pinch or other gesture, not our scroll shim's job —
      // let the browser/xterm handle it untouched.
      touchActive = false
      return
    }
    touchActive = true
    touchMoved = false
    touchStartY = e.touches[0].clientY
    touchLastY = touchStartY
  }

  function onTouchMove(e: TouchEvent) {
    if (!touchActive || e.touches.length !== 1) return
    const touch = e.touches[0]
    const dy = touch.clientY - touchLastY
    touchLastY = touch.clientY
    if (!touchMoved && Math.abs(touch.clientY - touchStartY) > TOUCH_SLOP) {
      touchMoved = true
    }
    if (!touchMoved) return
    // Past slop: this is a scroll drag, not a tap — steal the gesture from
    // the browser and forward it to xterm as wheel. Finger-down (dy > 0)
    // means "pull content toward history", i.e. scroll up, i.e. negative
    // deltaY — hence the sign flip.
    e.preventDefault()
    const target = e.target instanceof EventTarget ? e.target : el
    target?.dispatchEvent(
      new WheelEvent('wheel', {
        deltaY: -dy,
        deltaMode: 0,
        clientX: touch.clientX,
        clientY: touch.clientY,
        bubbles: true,
        cancelable: true,
      }),
    )
  }

  function onTouchEnd() {
    touchActive = false
    touchMoved = false
  }

  return {
    get connected() {
      return state === 'open'
    },
    onStateChange(cb) {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    adjustFontSize(delta: number) {
      const current = term.options.fontSize ?? DEFAULT_FONT_SIZE
      const next = clampFontSize(current, delta)
      if (next === current) return
      term.options.fontSize = next
      fit.fit()
      saveFontSize(next)
    },
    sendInput(text: string) {
      if (ws?.readyState === WebSocket.OPEN) {
        ws.send(asSendable(encodeInput(text)))
      }
    },
    focus() {
      term.focus()
    },
    openKeyboard() {
      openKeyboard(term.textarea)
    },
    blur() {
      term.blur()
    },
    suppressKeyboard() {
      term.textarea?.setAttribute('inputmode', 'none')
    },
    restoreKeyboard() {
      term.textarea?.removeAttribute('inputmode')
    },
    attach(target: HTMLElement) {
      el = target
      term.open(el)
      tryLoadWebgl()
      fit.fit()
      hardenTextareaAutocorrect()

      // Mouse pass-through + no browser context menu, per the design doc's
      // "Frontend requirements": Herdr is mouse-first (clickable panes/
      // tabs/borders), so the browser's own right-click menu must never
      // shadow it.
      el.addEventListener('contextmenu', (e) => e.preventDefault())

      // { passive: false } is required so preventDefault() in onTouchMove
      // can actually suppress the browser's default touch handling once a
      // drag is detected — see the touch-scroll shim's doc comment above.
      el.addEventListener('touchstart', onTouchStart, { passive: false })
      el.addEventListener('touchmove', onTouchMove, { passive: false })
      el.addEventListener('touchend', onTouchEnd, { passive: false })
      el.addEventListener('touchcancel', onTouchEnd, { passive: false })

      resizeObserver = new ResizeObserver(() => {
        fit.fit()
      })
      resizeObserver.observe(el)

      open()
    },
    close() {
      closed = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      resizeObserver?.disconnect()
      el?.removeEventListener('touchstart', onTouchStart)
      el?.removeEventListener('touchmove', onTouchMove)
      el?.removeEventListener('touchend', onTouchEnd)
      el?.removeEventListener('touchcancel', onTouchEnd)
      ws?.close()
      term.dispose()
    },
  }
}
