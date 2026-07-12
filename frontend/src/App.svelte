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
  import Promptbox from './components/Promptbox.svelte'

  let container: HTMLDivElement
  let state = $state<ConnectionState>('connecting')

  // Shared between the terminal bridge (consulted on every soft-keyboard
  // keystroke via term.onData) and KeyBar (toggled/read for the UI
  // highlight) — see keybar.ts's StickyModifiers doc for why a tap-Ctrl
  // latch must apply to BOTH input paths, not just key-bar button presses.
  const sticky = createStickyModifiers()
  const bridge = createTerminalBridge(sticky)

  // App owns the input mode (mobile-ux-v2.md "Topbar"): promptbox and the
  // accessory key bar are MUTUALLY EXCLUSIVE, never both on screen, to save
  // vertical space on a phone. Topbar renders the single toggle button and
  // mutates this via `bind:`; Promptbox/KeyBar below just read it to decide
  // whether to render at all — single owner, two controlled children.
  let inputMode = $state<'promptbox' | 'keys'>('promptbox')

  // Entering keys mode must actually dismiss the soft keyboard, or the
  // accessory bar and the on-screen keyboard end up splitting the screen
  // anyway — the exact thing this mode switch exists to prevent. Keyed on
  // inputMode rather than inlined in the toggle handler so it also fires
  // correctly if inputMode is ever driven from somewhere else (and so it
  // runs once for the initial 'promptbox' value too, keeping the textarea
  // attribute in sync with the mode from first render). Blurring both the
  // terminal bridge's hidden textarea AND document.activeElement covers
  // the two places focus (and thus the keyboard) can be sitting: the
  // terminal itself, or the promptbox's contenteditable.
  //
  // blur() alone is a one-shot: the terminal is mouse-first (every pane is
  // clickable), so the very next tap on a pane re-focuses xterm's textarea
  // and Android reopens the keyboard right after this effect dismissed it.
  // suppressKeyboard()/restoreKeyboard() (terminal.ts) hold the invariant
  // across taps by toggling inputmode="none" on that textarea: 'none' in
  // keys mode keeps the terminal clickable/focusable (pane switching still
  // works) without ever summoning the keyboard; removed in promptbox mode
  // so tapping the terminal directly still opens the keyboard for typing.
  $effect(() => {
    if (inputMode === 'keys') {
      bridge.suppressKeyboard()
      bridge.blur()
      ;(document.activeElement as HTMLElement | null)?.blur()
    } else {
      bridge.restoreKeyboard()
    }
  })

  onMount(() => {
    const unsubscribe = bridge.onStateChange((s) => (state = s))
    bridge.attach(container)
    return unsubscribe
  })

  onDestroy(() => {
    bridge.close()
  })
</script>

<main>
  <!-- Layout A (mobile-ux-v2.md): topbar → terminal (flex:1) → promptbox OR
       accessory keys → soft keyboard, top to bottom, all in normal
       flex-column flow. Promptbox and KeyBar are exclusive siblings —
       exactly one renders per inputMode, so they never compete for the
       same screen space. Nothing here is position:fixed over another
       element in this stack — that's what let the old floating key bar
       cover the promptbox (issue #2). -->
  <Topbar {bridge} bind:inputMode connectionState={state} />
  <div class="terminal" bind:this={container}></div>
  {#if inputMode === 'promptbox'}
    <Promptbox />
  {:else}
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
