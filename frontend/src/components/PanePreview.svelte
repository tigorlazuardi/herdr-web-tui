<script lang="ts">
  import X from '@lucide/svelte/icons/x'
  import Copy from '@lucide/svelte/icons/copy'
  import { Button, Dialog } from 'sve-ui'
  import { createPanePreviewClient } from '../lib/preview'
  import { sessionFromPath } from '../lib/session'

  const client = createPanePreviewClient()
  let controller: AbortController | undefined
  let openState = $state(false)
  let text = $state('')
  let previewState = $state<'loading' | 'ready' | 'error'>('loading')
  let error = $state('')
  let copyStatus = $state('')

  /** Opens Bits UI dialog and reads focused pane anew; no snapshot cache exists. */
  export async function open() {
    controller?.abort()
    const next = new AbortController()
    controller = next
    text = ''
    error = ''
    copyStatus = ''
    previewState = 'loading'
    openState = true

    const result = await client.read(sessionFromPath(location.pathname), next.signal)
    if (controller !== next) return
    if (result.ok) {
      text = result.text
      previewState = 'ready'
    } else {
      error = result.error + (result.refID ? ` (ref: ${result.refID})` : '')
      previewState = 'error'
    }
  }

  function setOpen(next: boolean) {
    openState = next
    if (!next) {
      controller?.abort()
      controller = undefined
    }
  }

  async function copyAll() {
    copyStatus = ''
    try {
      await navigator.clipboard.writeText(text)
      copyStatus = 'Copied full snapshot.'
    } catch (err) {
      copyStatus = `Copy failed: ${(err as Error).message}`
    }
  }
</script>

<Dialog.Root open={openState} onOpenChange={setOpen}>
  <Dialog.Content class="pane-preview" aria-labelledby="pane-preview-title">
    <section class="preview">
      <header>
        <div>
          <Dialog.Title id="pane-preview-title">Focused pane preview</Dialog.Title>
          <Dialog.Description>Visible terminal snapshot</Dialog.Description>
        </div>
        <Dialog.Close>
          {#snippet child({ props })}
            <Button {...props} variant="ghost" size="sm" aria-label="Close focused pane preview">
              <X size={16} aria-hidden="true" />
              Close
            </Button>
          {/snippet}
        </Dialog.Close>
      </header>

      {#if previewState === 'loading'}
        <p class="message" role="status">Loading focused pane…</p>
      {:else if previewState === 'error'}
        <p class="message error" role="alert">{error}</p>
      {:else if text === ''}
        <p class="message">Focused pane has no visible text.</p>
      {:else}
        <!-- ponytail: native pre keeps raw selection/copy. Add formatter only if semantic rendering becomes required. -->
        <pre aria-label="Focused pane text">{text}</pre>
      {/if}

      <footer>
        <span class="copy-status" role="status">{copyStatus}</span>
        <Button variant="outline" size="sm" disabled={previewState !== 'ready'} onclick={copyAll}>
          <Copy size={15} aria-hidden="true" />
          Copy all
        </Button>
      </footer>
    </section>
  </Dialog.Content>
</Dialog.Root>

<style>
  :global(.pane-preview.sve-dialog-content) {
    width: min(72rem, calc(100vw - 2rem));
    max-width: none;
    height: min(80dvh, 52rem);
    max-height: none;
    padding: 0;
    overflow: hidden;
    border: 1px solid #57534e;
    background: #1c1917;
    color: #f5f5f4;
  }

  .preview { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  header, footer { display: flex; align-items: center; gap: 1rem; padding: 1rem 1.25rem; }
  header { justify-content: space-between; border-bottom: 1px solid #44403c; }
  :global(.pane-preview .sve-dialog-title), :global(.pane-preview .sve-dialog-description), p { margin: 0; }
  :global(.pane-preview .sve-dialog-title) { color: #f5f5f4; font-size: 1.1rem; }
  :global(.pane-preview .sve-dialog-description), .message, .copy-status { font: 0.85rem/1.4 system-ui, sans-serif; color: #d6d3d1; }
  pre { flex: 1; min-height: 0; overflow: auto; margin: 0; padding: 1.25rem; white-space: pre-wrap; overflow-wrap: normal; tab-size: 4; user-select: text; font: 0.85rem/1.45 ui-monospace, SFMono-Regular, Menlo, monospace; }
  .message { padding: 1.5rem 1.25rem; }
  .error { color: #fca5a5; }
  footer { justify-content: space-between; border-top: 1px solid #44403c; }
  .copy-status { min-height: 1.2rem; }

  @media (max-width: 40rem) {
    :global(.pane-preview.sve-dialog-content) { width: calc(100vw - 1rem); height: 88dvh; }
    header, footer { padding: 0.75rem 1rem; }
    pre { padding: 1rem; }
  }
</style>
