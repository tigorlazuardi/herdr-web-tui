<script lang="ts">
  // Termux-style accessory key bar (ticket 4). All key encoding lives in
  // lib/keybar.ts (pure, unit-tested); this component is thin UI glue: it
  // renders buttons, reads/writes the shared StickyModifiers latch for the
  // pressed/highlighted look, and calls TerminalBridge.sendInput() — the
  // SAME ws path a physical keystroke uses (see terminal.ts's sendInput
  // doc comment), so nothing about the wire format is duplicated here.
  import { onDestroy, onMount } from 'svelte'
  import { ALTERNATES, type StickyModifiers } from '../lib/keybar'
  import type { TerminalBridge } from '../lib/terminal'

  // sticky is owned by App.svelte and shared with the terminal bridge's
  // term.onData (see terminal.ts's createTerminalBridge doc) so a
  // key-bar Ctrl tap and a soft-keyboard keystroke consume the same latch.
  let { bridge, sticky }: { bridge: TerminalBridge; sticky: StickyModifiers } = $props()

  let mods = $state(sticky.state)
  onMount(() => sticky.subscribe((s) => (mods = s)))

  // Hidden by default on a device with a real (fine-pointer) mouse, since
  // a hardware keyboard is the near-certain companion of a mouse and the
  // bar would just eat screen space. `visible` is a manual override so
  // the toggle button always wins over the heuristic in either direction.
  const coarsePointer =
    typeof matchMedia === 'function' && matchMedia('(pointer: coarse)').matches
  let visible = $state(coarsePointer)

  // key: the logical name (SPECIAL_KEYS key, e.g. "ArrowUp") for anything
  // that isn't a literal printable character sent as-is.
  function press(key: string) {
    bridge.sendInput(sticky.consume(key))
    bridge.focus()
  }

  // Long-press -> alternate character (design doc: "long-press for
  // alternates" on row-2 punctuation). 500ms matches common mobile
  // long-press thresholds; not configurable because no caller needs it to
  // be.
  const LONG_PRESS_MS = 500
  let pressTimer: ReturnType<typeof setTimeout> | null = null
  let longPressFired = false

  function startPress(key: string) {
    longPressFired = false
    const alt = ALTERNATES[key]
    if (!alt) return
    pressTimer = setTimeout(() => {
      longPressFired = true
      press(alt)
    }, LONG_PRESS_MS)
  }

  function endPress(key: string) {
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
    if (!longPressFired) press(key)
  }

  function cancelPress() {
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
  }

  onDestroy(cancelPress)

  const row2Punct = ['/', '-', '|', '~', ':']
  const fKeys = ['F1', 'F2', 'F3', 'F4', 'F5', 'F6', 'F7', 'F8', 'F9', 'F10', 'F11', 'F12']
</script>

<button
  class="toggle"
  type="button"
  aria-pressed={visible}
  aria-label={visible ? 'Hide key bar' : 'Show key bar'}
  onclick={() => (visible = !visible)}
>
  {visible ? '⌨︎' : '⌨'}
</button>

{#if visible}
  <div class="keybar" role="toolbar" aria-label="Accessory key bar">
    <div class="row">
      <button type="button" onclick={() => press('Escape')}>Esc</button>
      <button
        type="button"
        class="mod"
        aria-pressed={mods.ctrl}
        onclick={() => sticky.toggle('ctrl')}
      >
        Ctrl
      </button>
      <button
        type="button"
        class="mod"
        aria-pressed={mods.alt}
        onclick={() => sticky.toggle('alt')}
      >
        Alt
      </button>
      <button type="button" onclick={() => press('Tab')}>Tab</button>
      <button
        type="button"
        class="mod"
        aria-pressed={mods.fn}
        onclick={() => sticky.toggle('fn')}
      >
        Fn
      </button>
      <!-- Dedicated prefix button (design doc: "nice-to-have") — sends
           Herdr's ctrl+b prefix directly, without needing to tap Ctrl
           first. Bypasses the sticky latch entirely since it's a fixed
           combo, not a general modifier. -->
      <button type="button" onclick={() => bridge.sendInput('\x02')}>^B</button>
      <button type="button" onclick={() => press('ArrowLeft')}>←</button>
      <button type="button" onclick={() => press('ArrowDown')}>↓</button>
      <button type="button" onclick={() => press('ArrowUp')}>↑</button>
      <button type="button" onclick={() => press('ArrowRight')}>→</button>
    </div>
    <div class="row row2">
      <button type="button" onclick={() => press('Home')}>Home</button>
      <button type="button" onclick={() => press('End')}>End</button>
      <button type="button" onclick={() => press('PageUp')}>PgUp</button>
      <button type="button" onclick={() => press('PageDown')}>PgDn</button>
      {#each row2Punct as char (char)}
        <button
          type="button"
          title={ALTERNATES[char] ? `long-press for ${ALTERNATES[char]}` : undefined}
          onpointerdown={() => startPress(char)}
          onpointerup={() => endPress(char)}
          onpointerleave={cancelPress}
          onpointercancel={cancelPress}
        >
          {char}
        </button>
      {/each}
      {#each fKeys as fk (fk)}
        <button type="button" onclick={() => press(fk)}>{fk}</button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .toggle {
    position: fixed;
    bottom: 0.5rem;
    right: 0.5rem;
    z-index: 11;
    width: 2.25rem;
    height: 2.25rem;
    border-radius: 50%;
    border: none;
    background: #292524;
    color: #e7e5e4;
    font-size: 1.1rem;
    line-height: 1;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.4);
  }

  .keybar {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 10;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.375rem;
    padding-bottom: max(0.375rem, env(safe-area-inset-bottom));
    background: #1c1917;
    border-top: 1px solid #292524;
    /* Same rationale as App.svelte's .terminal: taps here must drive the
       button, never a browser scroll/zoom gesture. */
    touch-action: none;
  }

  .row {
    display: flex;
    gap: 0.25rem;
  }

  .row2 {
    /* "Second row / swipe" per the design doc: F1-F12 + punctuation don't
       fit one screen width, so this row scrolls horizontally rather than
       wrapping (wrapping would grow the bar's height unpredictably and
       eat terminal rows). */
    overflow-x: auto;
  }

  .row button {
    flex: 1 0 2.25rem;
    min-width: 2.25rem;
    height: 2.25rem;
    border: none;
    border-radius: 0.375rem;
    background: #292524;
    color: #e7e5e4;
    font: 500 0.8rem/1 system-ui, sans-serif;
    white-space: nowrap;
  }

  .row button:active {
    background: #44403c;
  }

  .row button.mod[aria-pressed='true'] {
    background: #f59e0b;
    color: #1c1917;
  }
</style>
