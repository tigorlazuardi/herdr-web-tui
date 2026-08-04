<script lang="ts">
  import { createPanePreviewClient } from '../lib/preview'
  import { sessionFromPath } from '../lib/session'

  const client = createPanePreviewClient()
  let dialog: HTMLDialogElement
  let controller: AbortController | undefined
  let text = $state('')
  let previewState = $state<'loading' | 'ready' | 'error'>('loading')
  let error = $state('')
  let copyStatus = $state('')

  /** Opens a native modal and reads the focused pane anew; no snapshot cache exists. */
  export async function open() {
    controller?.abort()
    const next = new AbortController()
    controller = next
    text = ''
    error = ''
    copyStatus = ''
    previewState = 'loading'
    if (!dialog.open) dialog.showModal()

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

  function close() {
    controller?.abort()
    controller = undefined
    if (dialog.open) dialog.close()
  }

  function onClose() {
    controller?.abort()
    controller = undefined
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

<dialog bind:this={dialog} aria-labelledby="pane-preview-title" onclose={onClose}>
  <section class="preview">
    <header>
      <div>
        <h2 id="pane-preview-title">Focused pane preview</h2>
        <p>Visible terminal snapshot</p>
      </div>
      <button type="button" onclick={close}>Close</button>
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
      <button type="button" disabled={previewState !== 'ready'} onclick={copyAll}>Copy all</button>
    </footer>
  </section>
</dialog>

<style>
  dialog {
    width: min(72rem, calc(100vw - 2rem));
    max-width: none;
    height: min(80dvh, 52rem);
    padding: 0;
    border: 1px solid #57534e;
    border-radius: 0.75rem;
    background: #1c1917;
    color: #f5f5f4;
    box-shadow: 0 1.5rem 4rem rgb(0 0 0 / 0.55);
  }

  dialog::backdrop { background: rgb(0 0 0 / 0.62); }

  .preview { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  header, footer { display: flex; align-items: center; gap: 1rem; padding: 1rem 1.25rem; }
  header { justify-content: space-between; border-bottom: 1px solid #44403c; }
  h2, p { margin: 0; }
  h2 { font: 600 1.1rem/1.2 system-ui, sans-serif; }
  header p, .message, .copy-status { font: 0.85rem/1.4 system-ui, sans-serif; color: #d6d3d1; }
  button { border: 0; border-radius: 0.4rem; padding: 0.45rem 0.8rem; background: #44403c; color: inherit; font: 600 0.85rem/1 system-ui, sans-serif; }
  button:disabled { opacity: 0.5; }
  pre { flex: 1; min-height: 0; overflow: auto; margin: 0; padding: 1.25rem; white-space: pre-wrap; overflow-wrap: normal; tab-size: 4; user-select: text; font: 0.85rem/1.45 ui-monospace, SFMono-Regular, Menlo, monospace; }
  .message { padding: 1.5rem 1.25rem; }
  .error { color: #fca5a5; }
  footer { justify-content: space-between; border-top: 1px solid #44403c; }
  .copy-status { min-height: 1.2rem; }

  @media (max-width: 40rem) {
    dialog { width: calc(100vw - 1rem); height: 88dvh; }
    header, footer { padding: 0.75rem 1rem; }
    pre { padding: 1rem; }
  }
</style>
