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
   * The toggle button now switches between two mutually exclusive input
   * modes rather than showing/hiding the key bar independently of the
   * promptbox — `inputMode` is `$bindable` so this component can both
   * display and flip it, but the state itself lives in App.svelte (the
   * single owner; Promptbox/KeyBar read it as a plain controlled prop) —
   * see App.svelte's doc comment.
   */
  import type { ConnectionState, TerminalBridge } from '../lib/terminal'

  let {
    bridge,
    inputMode = $bindable(),
    connectionState,
  }: {
    bridge: TerminalBridge
    inputMode: 'promptbox' | 'keys'
    connectionState: ConnectionState
  } = $props()
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
    {#if connectionState !== 'open'}
      <span class="status" role="status">
        {connectionState === 'connecting' ? 'Connecting…' : 'Reconnecting…'}
      </span>
    {/if}
  </div>

  <!-- Single button, two mutually-exclusive modes: aria-pressed reflects
       "keys mode is active" and the label always names the mode a tap
       would switch TO, not the current one. -->
  <button
    class="toggle"
    type="button"
    aria-pressed={inputMode === 'keys'}
    aria-label={inputMode === 'keys' ? 'Switch to text input' : 'Switch to keys'}
    onclick={() => (inputMode = inputMode === 'keys' ? 'promptbox' : 'keys')}
  >
    {inputMode === 'keys' ? '✎' : '⌨'}
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

  .font-controls button,
  .toggle {
    width: 2rem;
    height: 2rem;
    border: none;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.1);
    color: #e7e5e4;
    font: 600 1rem/1 system-ui, sans-serif;
  }

  .toggle {
    flex: none;
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
</style>
