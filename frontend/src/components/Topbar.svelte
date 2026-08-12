<script lang="ts">
  /** Top controls. App owns one input switch: text composer vs accessory rail. */
  import Minus from '@lucide/svelte/icons/minus'
  import Plus from '@lucide/svelte/icons/plus'
  import Keyboard from '@lucide/svelte/icons/keyboard'
  import MessageSquareText from '@lucide/svelte/icons/message-square-text'
  import ScanText from '@lucide/svelte/icons/scan-text'
  import { Toolbar, Toggle } from 'bits-ui'
  import type { ConnectionState, TerminalBridge } from '../lib/terminal'
  import PushControl from './PushControl.svelte'

  type InputMode = 'promptbox' | 'accessory'

  let {
    bridge,
    inputMode = $bindable(),
    connectionState,
    wakeLockUnavailable,
    onPreview,
  }: {
    bridge: TerminalBridge
    inputMode: InputMode
    connectionState: ConnectionState
    wakeLockUnavailable: boolean
    onPreview: () => void
  } = $props()

  const MODE_LABEL: Record<InputMode, string> = {
    promptbox: 'Text',
    accessory: 'Rail',
  }
</script>

<Toolbar.Root class="topbar" aria-label="Terminal controls">
  <!-- Direct Bits UI fallback: Toolbar owns roving keyboard focus; Sve UI has no toolbar component. -->
  <div class="font-controls" role="group" aria-label="Terminal font size">
    <Toolbar.Button onclick={() => bridge.adjustFontSize(-1)} aria-label="Decrease font size">
      <Minus size={16} aria-hidden="true" />
    </Toolbar.Button>
    <Toolbar.Button onclick={() => bridge.adjustFontSize(1)} aria-label="Increase font size">
      <Plus size={16} aria-hidden="true" />
    </Toolbar.Button>
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

  <Toolbar.Button class="preview" onclick={onPreview} aria-label="Preview focused pane">
    <ScanText size={16} aria-hidden="true" />
    Preview
  </Toolbar.Button>

  <!-- Toolbar child props add roving focus to Toggle's pressed-state button. -->
  <Toolbar.Button>
    {#snippet child({ props })}
      <Toggle.Root
        {...props}
        class="toggle"
        pressed={inputMode === 'accessory'}
        aria-label={`Input mode: ${MODE_LABEL[inputMode]} (tap to switch)`}
        onPressedChange={(pressed) => (inputMode = pressed ? 'accessory' : 'promptbox')}
      >
        {#if inputMode === 'promptbox'}
          <MessageSquareText size={17} aria-hidden="true" />
        {:else}
          <Keyboard size={17} aria-hidden="true" />
        {/if}
        <span>{MODE_LABEL[inputMode]}</span>
      </Toggle.Root>
    {/snippet}
  </Toolbar.Button>
</Toolbar.Root>

<style>
  :global(.topbar) {
    flex: none;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.5rem;
    padding-top: max(0.4rem, env(safe-area-inset-top));
    background: #1c1917;
    border-bottom: 1px solid #292524;
  }

  .font-controls { display: flex; gap: 0.25rem; }

  .font-controls :global(button),
  :global(.topbar .preview),
  :global(.topbar .toggle) {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
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

  .font-controls :global(button) { width: 2rem; padding: 0; }
  :global(.topbar button:focus-visible) { outline: 2px solid #38bdf8; outline-offset: 2px; }
  :global(.topbar .toggle[data-state='on']) { background: #44403c; }

  .spacer { flex: 1; display: flex; justify-content: center; min-width: 0; }
  .status { padding: 0.2rem 0.65rem; border-radius: 999px; background: #f59e0b; color: #1c1917; font: 500 0.75rem/1.4 system-ui, sans-serif; }
  .warning { background: #fca5a5; }
</style>
