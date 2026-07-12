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

export interface TerminalBridge {
  /** Mounts xterm into el and starts sizing/streaming. */
  attach(el: HTMLElement): void
  /** Tears down the websocket and disposes the xterm instance. Idempotent. */
  close(): void
  /** True once the initial ws connection has been accepted. */
  readonly connected: boolean
  /** Subscribes to connection-state changes (for a "reconnecting…" banner). */
  onStateChange(cb: (state: ConnectionState) => void): () => void
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
 */
export function createTerminalBridge(): TerminalBridge {
  const term = new Terminal({
    cursorBlink: true,
    scrollback: 5000,
    allowProposedApi: true,
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
      ws.send(asSendable(encodeInput(data)))
    }
  })
  term.onResize(({ cols, rows }) => {
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(asSendable(encodeResizeFrame(cols, rows)))
    }
  })

  let resizeObserver: ResizeObserver | null = null

  return {
    get connected() {
      return state === 'open'
    },
    onStateChange(cb) {
      listeners.add(cb)
      return () => listeners.delete(cb)
    },
    attach(target: HTMLElement) {
      el = target
      term.open(el)
      tryLoadWebgl()
      fit.fit()

      // Mouse pass-through + no browser context menu, per the design doc's
      // "Frontend requirements": Herdr is mouse-first (clickable panes/
      // tabs/borders), so the browser's own right-click menu must never
      // shadow it.
      el.addEventListener('contextmenu', (e) => e.preventDefault())

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
      ws?.close()
      term.dispose()
    },
  }
}
