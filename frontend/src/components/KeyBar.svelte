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
  //
  // No `visible` prop: App.svelte now renders this component only while
  // inputMode === 'keys' (mobile-ux-v2.md "Topbar" + the exclusive-mode
  // rework) — presence in the tree IS the visibility, single owner in the
  // parent, so there's no separate flag to keep in sync here.
  let { bridge, sticky }: { bridge: TerminalBridge; sticky: StickyModifiers } = $props()

  let mods = $state(sticky.state)
  onMount(() => sticky.subscribe((s) => (mods = s)))

  // key: the logical name (SPECIAL_KEYS key, e.g. "ArrowUp") for anything
  // that isn't a literal printable character sent as-is. Deliberately does
  // NOT call bridge.focus(): sendInput() writes straight to the ws, so the
  // key still reaches the terminal without touching xterm's hidden
  // textarea. Refocusing it here used to pop the soft keyboard back open
  // on every tap, which fought App.svelte's blur() on entry into keys mode
  // and defeated the whole point of a dedicated keys mode.
  function press(key: string) {
    bridge.sendInput(sticky.consume(key))
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

  // Termux-style, but split across two always-visible rows instead of one:
  // cramming all 11 essential items (Esc/Ctrl/Alt/Tab/Fn/^B + 4 arrows +
  // toggle) into a single row forced them to shrink-to-fit on a phone,
  // leaving tiny, hard-to-tap targets. Row A (modifiers) and Row B (arrows
  // + toggle) each hold <=6 items, so every button stays a comfortable tap
  // target with no shrink-cram. Row C (Home/End/PgUp/PgDn, punctuation,
  // F-keys) is rarely used and stays hidden behind this toggle, matching
  // Termux's own extra-keys row behavior.
  let expanded = $state(false)
</script>

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
  </div>
  <div class="row">
    <button type="button" onclick={() => press('ArrowLeft')}>←</button>
    <button type="button" onclick={() => press('ArrowDown')}>↓</button>
    <button type="button" onclick={() => press('ArrowUp')}>↑</button>
    <button type="button" onclick={() => press('ArrowRight')}>→</button>
    <button
      type="button"
      class="toggle"
      aria-label={expanded ? 'Hide extra keys' : 'Show extra keys'}
      aria-expanded={expanded}
      onclick={() => (expanded = !expanded)}
    >
      {expanded ? '∧' : '∨'}
    </button>
  </div>
  {#if expanded}
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
  {/if}
</div>

<style>
  .keybar {
    /* Layout A (mobile-ux-v2.md): normal flow, NOT position:fixed. This
       bar used to float over the whole viewport bottom, which is exactly
       what let it cover the promptbox (issue #2). As a flex item after
       Promptbox in App.svelte's column, it naturally sits below the
       promptbox and above the soft keyboard (index.html's
       interactive-widget=resizes-content shrinks the layout viewport on
       keyboard-open, so the whole column — including this bar — rides
       above it without any extra positioning here). */
    flex: none;
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
    /* Both always-visible rows top out at 6 items (row A: Esc/Ctrl/Alt/
       Tab/Fn/^B; row B: 4 arrows + toggle), so a sensible min-width fits
       comfortably on a phone with no shrink-cram — unlike the old single
       11-item row this replaces. Row 2's F-keys/punctuation still exceed
       one screen width even at this floor, which is exactly what drives
       its horizontal scroll below instead of ever compressing to fit. */
    flex: 1 1 auto;
    min-width: 2.25rem;
    height: 2.25rem;
    padding: 0 0.4rem;
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

  .row button.toggle {
    /* Doesn't grow with the other row-1 keys — it's a fixed-width chevron,
       not a key, so it shouldn't compete for width. */
    flex: none;
  }
</style>
