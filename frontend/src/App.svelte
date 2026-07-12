<script lang="ts">
  // Renders the live Herdr TUI. The terminal is driven entirely by
  // lib/terminal.ts, deliberately outside Svelte's reactivity (see that
  // module's doc comment for why) — this component's only job is to give
  // it a DOM element on mount and call close() on unmount so the ws/pty on
  // the server always tears down when the tab closes or navigates away.
  import { onDestroy, onMount } from 'svelte'
  import KeyBar from './components/KeyBar.svelte'
  import { createStickyModifiers } from './lib/keybar'
  import { createTerminalBridge, type ConnectionState } from './lib/terminal'

  let container: HTMLDivElement
  let state = $state<ConnectionState>('connecting')

  // Shared between the terminal bridge (consulted on every soft-keyboard
  // keystroke via term.onData) and KeyBar (toggled/read for the UI
  // highlight) — see keybar.ts's StickyModifiers doc for why a tap-Ctrl
  // latch must apply to BOTH input paths, not just key-bar button presses.
  const sticky = createStickyModifiers()
  const bridge = createTerminalBridge(sticky)

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
  {#if state !== 'open'}
    <div class="status" role="status">
      {state === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
    </div>
  {/if}
  <div class="terminal" bind:this={container}></div>
  <KeyBar {bridge} {sticky} />
</main>

<style>
  main {
    display: flex;
    flex-direction: column;
    height: 100dvh;
    background: #000;
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

  .status {
    position: fixed;
    top: 0.5rem;
    right: 0.5rem;
    z-index: 10;
    padding: 0.25rem 0.75rem;
    border-radius: 999px;
    background: #f59e0b;
    color: #1c1917;
    font: 500 0.8rem/1.4 system-ui, sans-serif;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
  }
</style>
