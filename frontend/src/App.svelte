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

  // App owns the accessory key bar's visibility (mobile-ux-v2.md
  // "Topbar"): KeyBar used to toggle itself, which meant its toggle button
  // and this same show/hide decision lived only inside KeyBar. Now Topbar
  // renders the toggle button and mutates this via `bind:`, KeyBar just
  // reads it as a prop — single owner, two controlled children. Default
  // hidden on a device with a real (fine-pointer) mouse, since a hardware
  // keyboard is the near-certain companion of a mouse and the bar would
  // just eat screen space; the toggle always wins over this heuristic
  // afterwards.
  const coarsePointer =
    typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches
  let keybarVisible = $state(coarsePointer)

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
  <!-- Layout A (mobile-ux-v2.md): topbar → terminal (flex:1) → promptbox
       → accessory keys (when visible) → soft keyboard, top to bottom, all
       in normal flex-column flow. Nothing here is position:fixed over
       another element in this stack — that's what let the old floating
       key bar cover the promptbox (issue #2). -->
  <Topbar {bridge} bind:keybarVisible connectionState={state} />
  <div class="terminal" bind:this={container}></div>
  <Promptbox />
  <KeyBar {bridge} {sticky} visible={keybarVisible} />
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
