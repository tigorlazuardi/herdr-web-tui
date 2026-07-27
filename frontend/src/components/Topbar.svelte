<script lang="ts">
  /**
   * Fixed top bar (mobile-ux-v2.md, "Topbar"). Replaces two floating
   * bottom-right control clusters that used to collide: App.svelte's font
   * +/- lever and KeyBar's own show/hide toggle (issue #3). This
   * component owns neither the connection status text nor the accessory
   * key bar's own content — it only renders the three controls that used
   * to float, plus a spacer slot for the connecting/reconnecting badge so
   * that badge doesn't end up sharing the same corner as the toggle
   * button (it used to be fixed top-right while the toggle was fixed
   * bottom-right; now both live in the same bar).
   *
   * The toggle button now CYCLES through three input modes (promptbox ->
   * keys -> termux -> promptbox) rather than switching between two —
   * `inputMode` is `$bindable` so this component can both display and
   * advance it, but the state itself lives in App.svelte (the single
   * owner; Promptbox/KeyBar read it as a plain controlled prop) — see
   * App.svelte's doc comment. The button shows an icon + a short text
   * label naming the CURRENT mode (not the mode a tap switches to, unlike
   * the old two-mode label) since a bare glyph doesn't scale to
   * disambiguating three states at a glance.
   */
  import type { ConnectionState, TerminalBridge } from '../lib/terminal'
  import PushControl from './PushControl.svelte'

  type InputMode = 'promptbox' | 'keys' | 'termux'

  let {
    bridge,
    inputMode = $bindable(),
    connectionState,
    wakeLockUnavailable,
  }: {
    bridge: TerminalBridge
    inputMode: InputMode
    connectionState: ConnectionState
    wakeLockUnavailable: boolean
  } = $props()

  const NEXT_MODE: Record<InputMode, InputMode> = {
    promptbox: 'keys',
    keys: 'termux',
    termux: 'promptbox',
  }

  const MODE_ICON: Record<InputMode, string> = {
    promptbox: '✎',
    keys: '⌨',
    termux: '▣',
  }

  const MODE_LABEL: Record<InputMode, string> = {
    promptbox: 'Text',
    keys: 'Keys',
    termux: 'Termux',
  }
</script>

<div class="topbar" role="toolbar" aria-label="Terminal controls">
  <!-- Font +/− lever (design doc, "Frontend requirements", optional): the
       only client-side way to force a lower column count than the
       device's viewport gives — useful on a large-screen phone/tablet
       whose width alone doesn't drop below Herdr's mobile_width_threshold. -->
  <div class="font-controls" role="group" aria-label="Terminal font size">
    <button type="button" onclick={() => bridge.adjustFontSize(-1)} aria-label="Decrease font size">
      −
    </button>
    <button type="button" onclick={() => bridge.adjustFontSize(1)} aria-label="Increase font size">
      +
    </button>
  </div>

  <div class="spacer">
    <PushControl />
    {#if connectionState !== 'open'}
      <span class="status" role="status">
        {connectionState === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
      </span>
    {:else if wakeLockUnavailable}
      <span class="status warning" role="status">Screen may sleep</span>
    {/if}
  </div>

  <!-- Single button, three-way cycle: icon + label name the CURRENT mode
       (a bare glyph can't disambiguate three states as fast as two), and
       aria-label describes the tap action rather than the state so a
       screen reader hears what pressing it does. -->
  <button
    class="toggle"
    type="button"
    aria-label={`Input mode: ${MODE_LABEL[inputMode]} (tap to switch)`}
    onclick={() => (inputMode = NEXT_MODE[inputMode])}
  >
    <span class="toggle-icon" aria-hidden="true">{MODE_ICON[inputMode]}</span>
    <span class="toggle-label">{MODE_LABEL[inputMode]}</span>
  </button>
</div>

<style>
  .topbar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.5rem;
    padding-top: max(0.4rem, env(safe-area-inset-top));
    background: #1c1917;
    border-bottom: 1px solid #292524;
  }

  .font-controls {
    display: flex;
    gap: 0.25rem;
  }

  .font-controls button {
    width: 2rem;
    height: 2rem;
    border: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.1);
    color: #e7e5e4;
    font: 600 1rem/1 system-ui, sans-serif;
  }

  /* Auto-width pill, not the old fixed 2rem circle: a text label beside
     the icon needs to fit three different word lengths (Text/Keys/Termux)
     without truncating or forcing a fixed width sized for the longest. */
  .toggle {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.3rem;
    height: 2rem;
    padding: 0 0.65rem;
    border: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.1);
    color: #e7e5e4;
    font: 600 0.8rem/1 system-ui, sans-serif;
    white-space: nowrap;
  }

  .toggle-icon {
    font-size: 1.1rem;
  }

  .spacer {
    flex: 1;
    display: flex;
    justify-content: center;
    min-width: 0;
  }

  .status {
    padding: 0.2rem 0.65rem;
    border-radius: 999px;
    background: #f59e0b;
    color: #1c1917;
    font: 500 0.75rem/1.4 system-ui, sans-serif;
  }

  .warning {
    background: #fca5a5;
  }
</style>
