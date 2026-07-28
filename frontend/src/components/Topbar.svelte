<script lang="ts">
  /** Top controls. App owns one input switch: Text composer vs accessory rail. */
  import type { ConnectionState, TerminalBridge } from '../lib/terminal'
  import PushControl from './PushControl.svelte'

  type InputMode = 'promptbox' | 'accessory'

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
    promptbox: 'accessory',
    accessory: 'promptbox',
  }

  const MODE_ICON: Record<InputMode, string> = {
    promptbox: '✎',
    accessory: '⌨',
  }

  const MODE_LABEL: Record<InputMode, string> = {
    promptbox: 'Text',
    accessory: 'Rail',
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

  <!-- Single button names current mode; tap swaps Text composer and accessory rail. -->
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
