<script lang="ts">
  // Renders the live Herdr TUI. The terminal is driven entirely by
  // lib/terminal.ts, deliberately outside Svelte's reactivity (see that
  // module's doc comment for why) — this component's only job is to give
  // it a DOM element on mount and call close() on unmount so the ws/pty on
  // the server always tears down when the tab closes or navigates away.
  import { onDestroy, onMount } from 'svelte'
  import KeyBar from './components/KeyBar.svelte'
  import Topbar from './components/Topbar.svelte'
  import { createStickyModifiers } from './lib/keybar'
  import { createTerminalBridge, type ConnectionState } from './lib/terminal'
  import { holdPWAScreenAwake } from './lib/wake-lock'
  import Promptbox from './components/Promptbox.svelte'

  let container: HTMLDivElement
  let connectionState = $state<ConnectionState>('connecting')
  let wakeLockUnavailable = $state(false)

  // Shared between the terminal bridge (consulted on every soft-keyboard
  // keystroke via term.onData) and KeyBar (toggled/read for the UI
  // highlight) — see keybar.ts's StickyModifiers doc for why a tap-Ctrl
  // latch must apply to BOTH input paths, not just key-bar button presses.
  const sticky = createStickyModifiers()
  const bridge = createTerminalBridge(sticky)

  // App owns one explicit switch: prompt composer vs accessory rail. The
  // old keys/termux split made users choose between "keys" and "keyboard";
  // the unified rail follows Termux instead: chips stay above the soft
  // keyboard, and sticky modifiers still make one-shot chords possible.
  let inputMode = $state<'promptbox' | 'accessory'>('promptbox')

  $effect(() => {
    bridge.restoreKeyboard()
    if (inputMode === 'accessory') bridge.focus()
  })

  onMount(() => {
    // ponytail: manifest is existing server-owned app config; reuse it instead of adding another endpoint.
    void fetch('/manifest.webmanifest')
      .then((response) => (response.ok ? response.json() : null))
      .then((manifest: { name?: unknown } | null) => {
        if (typeof manifest?.name === 'string' && manifest.name) document.title = manifest.name
      })
      .catch(() => {})

    const unsubscribe = bridge.onStateChange((s) => (connectionState = s))
    const releaseWakeLock = holdPWAScreenAwake((unavailable) => (wakeLockUnavailable = unavailable))
    bridge.attach(container)
    return () => {
      releaseWakeLock()
      unsubscribe()
    }
  })

  // "Tap TUI while a modifier is latched cancels it" — a genuine tap on the
  // terminal is the user backing out of the one-shot chord, not a
  // follow-up character for it. Deliberately no preventDefault(): a
  // pane-switch click must still reach xterm underneath this handler. The
  // existing touch->wheel shim (terminal.ts) turns a real drag into a
  // scroll rather than a click, so this only ever fires on a genuine tap.
  function onTerminalClick() {
    if (sticky.state.ctrl || sticky.state.alt || sticky.state.fn) {
      sticky.clear()
    }
  }

  onDestroy(() => {
    bridge.close()
  })
</script>

<main>
  <!-- Layout A (mobile-ux-v2.md): topbar → terminal (flex:1) → promptbox →
       accessory rail → soft keyboard, top to bottom,
       all in normal flex-column flow. Promptbox always renders (see its
       `hidden` prop's doc comment — mounted-but-hidden, not
       conditionally-mounted, so switching modes never destroys its text/
       attachments); KeyBar renders only outside promptbox mode, so the two
       never visually compete for the same screen space even though both
       can be in the DOM at once. Nothing here is position:fixed over
       another element in this stack — that's what let the old floating
       key bar cover the promptbox (issue #2). -->
  <Topbar {bridge} bind:inputMode {connectionState} {wakeLockUnavailable} />
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- This div isn't a keyboard-operable control; it's xterm's own mount
       point, which already handles its own focus/click semantics
       (mouse-first TUI). onclick here only adds the tap-cancels-latch
       side effect on top of xterm's existing click handling, not a new
       interactive role. -->
  <div class="terminal" bind:this={container} onclick={onTerminalClick}></div>
  <Promptbox hidden={inputMode !== 'promptbox'} />
  {#if inputMode === 'accessory'}
    <KeyBar {bridge} {sticky} />
  {/if}
</main>

<style>
  main {
    display: flex;
    flex-direction: column;
    height: 100dvh;
    background: #000;
    /* No accidental pull-to-refresh reload on a phone: a downward drag at
       the top of the terminal must reach xterm's scrollback, not the
       browser's native overscroll gesture (design doc, "Frontend
       requirements"). */
    overscroll-behavior: none;
  }

  .terminal {
    flex: 1;
    min-height: 0;
    /* Herdr is mouse-first (whole UI clickable): let clicks/drags reach
       xterm's own handling instead of the browser's touch gestures
       (scroll/pinch-zoom), which would otherwise fight pane/tab clicks on
       a touch device. */
    touch-action: none;
  }

</style>
