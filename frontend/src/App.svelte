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
  let connectionState = $state<ConnectionState>('connecting')

  // Shared between the terminal bridge (consulted on every soft-keyboard
  // keystroke via term.onData) and KeyBar (toggled/read for the UI
  // highlight) — see keybar.ts's StickyModifiers doc for why a tap-Ctrl
  // latch must apply to BOTH input paths, not just key-bar button presses.
  const sticky = createStickyModifiers()
  const bridge = createTerminalBridge(sticky)

  // App owns the input mode (mobile-ux-v2.md "Topbar", extended to three
  // modes): 'promptbox' (the artifact composer), 'keys' (accessory bar,
  // soft keyboard suppressed — tapping the TUI must not summon it), and
  // 'termux' (accessory bar ABOVE an always-open soft keyboard, for
  // sustained typing into e.g. neovim). Only the Topbar toggle cycles this
  // — no auto-switch on tap/keystroke, or the mode would flip out from
  // under the user mid-session. Promptbox stays MOUNTED in all three modes
  // (see its `hidden` prop) so switching away and back never drops
  // in-progress text or uploaded attachments; KeyBar only mounts outside
  // promptbox mode, matching the old exclusive-render behaviour it never
  // needed to keep state across.
  let inputMode = $state<'promptbox' | 'keys' | 'termux'>('promptbox')

  // Local mirror of the sticky-modifier latch, purely so the $effect below
  // can react to it (an $effect can only track $state reads, not an
  // external class's plain-property reads) — see keybar.ts's
  // StickyModifiers doc for why App/KeyBar/terminal.ts all share one
  // instance instead of each keeping a latch of their own.
  let mods = $state(sticky.state)

  // Keyboard/focus policy per mode (design doc: one-shot vs sustained
  // keyboard). Keyed on inputMode AND mods so a modifier latching/
  // clearing mid-keys-mode also re-runs this without a separate handler:
  //
  // - promptbox: keyboard behaves exactly as it does today — restoreKeyboard()
  //   so tapping the terminal (or the promptbox) opens it normally.
  //
  // - termux: keyboard STAYS OPEN the whole time. This mode exists for
  //   sustained typing (neovim etc.), so every plain keystroke must reach
  //   the TUI without an extra tap to reopen the keyboard first.
  //   restoreKeyboard() + focus() unconditionally, regardless of mods —
  //   the accessory bar renders above the keyboard here, both on screen at
  //   once, which is the whole point of this mode.
  //
  // - keys: the keyboard is a ONE-SHOT popup for a modifier's follow-up
  //   character, not a permanent fixture — tapping the TUI in this mode
  //   must never summon it (that's the mode's entire reason to exist).
  //   While no modifier is latched: suppressKeyboard() + blur() (both the
  //   bridge's hidden textarea and whatever else has focus — covers the
  //   terminal and the promptbox, the two places focus/keyboard can be
  //   sitting). The instant a modifier latches (ctrl/alt/fn tap on
  //   KeyBar): restoreKeyboard() + focus() so the soft keyboard pops up
  //   for the follow-up keystroke; that keystroke runs through
  //   term.onData -> sticky.consume() (terminal.ts) which sends the
  //   modified byte AND clears the latch, and this effect re-runs on the
  //   next mods change to suppress the keyboard again. Net effect: tap
  //   Ctrl -> keyboard pops up -> type c -> sends \x03 -> keyboard closes.
  //
  // suppressKeyboard()/restoreKeyboard() (terminal.ts) hold the invariant
  // across taps by toggling inputmode="none" on xterm's textarea: a
  // one-shot blur() alone isn't enough because the terminal is mouse-first
  // (every pane clickable) and the very next tap on a pane would
  // re-focus/re-summon the keyboard otherwise.
  $effect(() => {
    if (inputMode === 'promptbox') {
      bridge.restoreKeyboard()
    } else if (inputMode === 'termux') {
      bridge.restoreKeyboard()
      bridge.focus()
    } else {
      const anyLatch = mods.ctrl || mods.alt || mods.fn
      if (anyLatch) {
        bridge.restoreKeyboard()
        bridge.focus()
      } else {
        bridge.suppressKeyboard()
        bridge.blur()
        ;(document.activeElement as HTMLElement | null)?.blur()
      }
    }
  })

  onMount(() => {
    const unsubStickyMods = sticky.subscribe((s) => (mods = s))
    const unsubscribe = bridge.onStateChange((s) => (connectionState = s))
    bridge.attach(container)
    return () => {
      unsubscribe()
      unsubStickyMods()
    }
  })

  // "Tap TUI while a modifier is latched cancels it" — a genuine tap on the
  // terminal is the user backing out of the one-shot chord, not a
  // follow-up character for it. Deliberately no preventDefault(): a
  // pane-switch click must still reach xterm underneath this handler. The
  // existing touch->wheel shim (terminal.ts) turns a real drag into a
  // scroll rather than a click, so this only ever fires on a genuine tap.
  // Clearing here also closes the soft keyboard again immediately, via the
  // keys-mode branch of the $effect above re-running on the mods change.
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
       accessory keys (keys/termux only) → soft keyboard, top to bottom,
       all in normal flex-column flow. Promptbox always renders (see its
       `hidden` prop's doc comment — mounted-but-hidden, not
       conditionally-mounted, so switching modes never destroys its text/
       attachments); KeyBar renders only outside promptbox mode, so the two
       never visually compete for the same screen space even though both
       can be in the DOM at once. Nothing here is position:fixed over
       another element in this stack — that's what let the old floating
       key bar cover the promptbox (issue #2). -->
  <Topbar {bridge} bind:inputMode {connectionState} />
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <!-- This div isn't a keyboard-operable control; it's xterm's own mount
       point, which already handles its own focus/click semantics
       (mouse-first TUI). onclick here only adds the tap-cancels-latch
       side effect on top of xterm's existing click handling, not a new
       interactive role. -->
  <div class="terminal" bind:this={container} onclick={onTerminalClick}></div>
  <Promptbox hidden={inputMode !== 'promptbox'} />
  {#if inputMode !== 'promptbox'}
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
