<script lang="ts">
  import { Button, Dialog } from 'sve-ui'
  import X from '@lucide/svelte/icons/x'
  import type { WakeLockStatus } from '../lib/wake-lock'

  let { wakeLockStatus, onOpen }: { wakeLockStatus: WakeLockStatus; onOpen?: () => void } = $props()
  let open = $state(false)

  const installed = matchMedia('(display-mode: standalone)').matches
  const notifications = 'Notification' in window ? Notification.permission : 'unsupported'
  const push = 'serviceWorker' in navigator && 'PushManager' in window ? 'Supported' : 'Unsupported'

  function openDialog() {
    onOpen?.()
    open = true
  }
</script>

<Button variant="outline" size="sm" onclick={openDialog}>PWA permissions</Button>

<Dialog.Root bind:open>
  <Dialog.Content class="pwa-status" aria-labelledby="pwa-status-title">
    <header>
      <div>
        <Dialog.Title id="pwa-status-title">PWA status</Dialog.Title>
        <Dialog.Description>Browser capabilities and permissions</Dialog.Description>
      </div>
      <Dialog.Close>
        {#snippet child({ props })}
          <Button {...props} variant="ghost" size="sm" aria-label="Close PWA status">
            <X size={16} aria-hidden="true" />
            Close
          </Button>
        {/snippet}
      </Dialog.Close>
    </header>

    <dl>
      <div><dt>App mode</dt><dd>{installed ? 'Installed PWA' : 'Browser tab'}</dd></div>
      <div><dt>Secure context</dt><dd>{isSecureContext ? 'Yes' : 'No'}</dd></div>
      <div><dt>Wake lock</dt><dd class:warning={wakeLockStatus.state !== 'active'}>{wakeLockStatus.message}</dd></div>
      <div><dt>Notifications</dt><dd>{notifications}</dd></div>
      <div><dt>Push API</dt><dd>{push}</dd></div>
    </dl>
  </Dialog.Content>
</Dialog.Root>

<style>
  :global(.pwa-status.sve-dialog-content) {
    width: min(30rem, calc(100vw - 2rem));
    padding: 0;
    overflow: hidden;
    border: 1px solid #57534e;
    background: #1c1917;
    color: #f5f5f4;
  }
  header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: 1rem 1.25rem; border-bottom: 1px solid #44403c; }
  :global(.pwa-status .sve-dialog-title), :global(.pwa-status .sve-dialog-description) { margin: 0; }
  :global(.pwa-status .sve-dialog-title) { color: #f5f5f4; font-size: 1.1rem; }
  :global(.pwa-status .sve-dialog-description) { color: #a8a29e; font-size: 0.85rem; }
  dl { display: grid; gap: 0; margin: 0; padding: 0.5rem 1.25rem 1.25rem; }
  dl div { display: flex; justify-content: space-between; gap: 1rem; padding: 0.75rem 0; border-bottom: 1px solid #292524; }
  dl div:last-child { border-bottom: 0; }
  dt { color: #a8a29e; }
  dd { margin: 0; text-align: right; }
  .warning { color: #fca5a5; }
</style>
