<script lang="ts">
  /** Top controls. App owns one input switch: text composer vs accessory rail. */
  import Minus from '@lucide/svelte/icons/minus'
  import Plus from '@lucide/svelte/icons/plus'
  import Keyboard from '@lucide/svelte/icons/keyboard'
  import MessageSquareText from '@lucide/svelte/icons/message-square-text'
  import ScanText from '@lucide/svelte/icons/scan-text'
  import Menu from '@lucide/svelte/icons/menu'
  import X from '@lucide/svelte/icons/x'
  import { Dialog, Toolbar, Toggle } from 'bits-ui'
  import type { ConnectionState, TerminalBridge } from '../lib/terminal'
  import type { WakeLockStatus } from '../lib/wake-lock'
  import PushControl from './PushControl.svelte'
  import PWAStatusDialog from './PWAStatusDialog.svelte'

  type InputMode = 'promptbox' | 'accessory'

  let {
    bridge,
    inputMode = $bindable(),
    connectionState,
    wakeLockStatus,
    onPreview,
  }: {
    bridge: TerminalBridge
    inputMode: InputMode
    connectionState: ConnectionState
    wakeLockStatus: WakeLockStatus
    onPreview: () => void
  } = $props()

  const MODE_LABEL: Record<InputMode, string> = {
    promptbox: 'Text',
    accessory: 'Rail',
  }
  let menuOpen = $state(false)
</script>

<Dialog.Root bind:open={menuOpen}>
<Toolbar.Root class="topbar" aria-label="Terminal controls">
  <Toolbar.Button class="menu" onclick={() => (menuOpen = true)} aria-label="Open app menu" aria-expanded={menuOpen}>
    <Menu size={20} aria-hidden="true" />
  </Toolbar.Button>

  <!-- Direct Bits UI fallback: Toolbar owns roving keyboard focus; Sve UI has no toolbar component. -->
  <div class="font-controls" role="group" aria-label="Terminal font size">
    <Toolbar.Button onclick={() => bridge.adjustFontSize(-1)} aria-label="Decrease font size">
      <Minus size={16} aria-hidden="true" />
    </Toolbar.Button>
    <Toolbar.Button onclick={() => bridge.adjustFontSize(1)} aria-label="Increase font size">
      <Plus size={16} aria-hidden="true" />
    </Toolbar.Button>
  </div>

  <div class="spacer"></div>

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

<Dialog.Portal>
  <!-- ponytail: installed Bits UI dialog owns focus trap, Escape, and light-dismiss; CSS state transitions avoid newer Popover animation APIs. -->
  <Dialog.Overlay forceMount class="drawer-backdrop" />
  <Dialog.Content forceMount preventScroll={menuOpen} class="app-sidebar" aria-labelledby="app-menu-title">
    <header>
      <Dialog.Title id="app-menu-title" class="menu-title">App menu</Dialog.Title>
      <button class="close-menu" onclick={() => (menuOpen = false)} aria-label="Close app menu">
        <X size={20} aria-hidden="true" />
      </button>
    </header>
    <div class="menu-row"><span>Connection</span><strong>{connectionState === 'open' ? 'Connected' : connectionState === 'connecting' ? 'Connecting…' : 'Reconnecting…'}</strong></div>
    <PushControl />
    <PWAStatusDialog {wakeLockStatus} onOpen={() => (menuOpen = false)} />
  </Dialog.Content>
</Dialog.Portal>
</Dialog.Root>

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
  :global(.topbar .toggle),
  :global(.topbar .menu) {
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

  .spacer { flex: 1; min-width: 0; }
  :global(.topbar .menu) { width: 2.5rem; height: 2rem; padding: 0; }

  :global(.app-sidebar) {
    position: fixed;
    z-index: 51;
    inset: 0 auto 0 0;
    display: grid;
    align-content: start;
    gap: 1rem;
    width: min(22rem, 88vw);
    height: 100dvh;
    padding: max(1rem, env(safe-area-inset-top)) 1rem 1rem;
    border: 0;
    border-right: 1px solid #57534e;
    border-radius: 0 1rem 1rem 0;
    outline: none;
    background: #1c1917;
    color: #f5f5f4;
    box-shadow: 1rem 0 2rem rgba(0, 0, 0, 0.45);
    opacity: 0;
    visibility: hidden;
    transform: translateX(-100%);
    transition: opacity 0.25s ease, transform 0.25s ease, visibility 0s linear 0.25s;
  }
  :global(.app-sidebar[data-state='open']) { opacity: 1; visibility: visible; transform: translateX(0); transition-delay: 0s; }
  :global(.drawer-backdrop) { position: fixed; z-index: 50; inset: 0; border: 0; background: rgba(0, 0, 0, 0.5); opacity: 0; visibility: hidden; transition: opacity 0.25s ease, visibility 0s linear 0.25s; }
  :global(.drawer-backdrop[data-state='open']) { opacity: 1; visibility: visible; transition-delay: 0s; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding-bottom: 0.5rem; border-bottom: 1px solid #44403c; }
  :global(.app-sidebar .menu-title) { margin: 0; font: 600 1.1rem/1.2 system-ui, sans-serif; }
  .close-menu { display: inline-flex; align-items: center; justify-content: center; width: 2rem; height: 2rem; padding: 0; border: 0; border-radius: 999px; background: rgba(255, 255, 255, 0.1); color: #f5f5f4; }
  .close-menu:focus-visible { outline: 2px solid #38bdf8; outline-offset: 2px; }
  .menu-row { display: flex; justify-content: space-between; gap: 1rem; color: #a8a29e; font: 0.85rem/1.4 system-ui, sans-serif; }
  .menu-row strong { color: #f5f5f4; }

  @media (prefers-reduced-motion: reduce) {
    :global(.app-sidebar), :global(.drawer-backdrop) { transition-duration: 0.01ms; }
  }
</style>
