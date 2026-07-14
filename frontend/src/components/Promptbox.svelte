<script lang="ts">
  /**
   * The artifact promptbox: a native <textarea> holding a plain-text
   * template with positional `§N` markers, plus an ordered attachment list
   * and the atomic all-or-nothing send flow. See the design doc's
   * "Artifact promptbox" section for the wire contract this implements
   * (lib/segments.ts's serialize()).
   *
   * This replaces an earlier contenteditable/inline-pill editor: on a real
   * phone that model flickered and dropped the caret on every keystroke
   * (Svelte rewriting DOM text nodes while the mobile IME was mid-
   * composition), and backspace could corrupt segment state. A textarea
   * gets normal, native backspace/selection/IME handling for free and
   * lets us turn autocorrect off outright (this is a command box, not
   * prose) — the tradeoff is that "attachments" are no longer inline DOM
   * nodes, just `§N` text tokens the user must not hand-type (see
   * lib/markers.ts's doc comment on why that's a non-issue in practice).
   *
   * text (the textarea's raw value) and attachments (the ordered file
   * list) are independent state; lib/markers.ts's parseTemplate() is the
   * only place that reconciles them, and only at send time.
   */
  import { tick } from 'svelte'
  import { flip } from 'svelte/animate'
  import Paperclip from '@lucide/svelte/icons/paperclip'
  import Send from '@lucide/svelte/icons/send'
  import Copy from '@lucide/svelte/icons/copy'
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle'
  import AttachmentPreview from './AttachmentPreview.svelte'
  import MimeBadge from './MimeBadge.svelte'
  import { parseTemplate, removeMarker, type Attachment } from '../lib/markers'
  import { resolveMime } from '../lib/sniff'
  import { createHttpUploadClient, type UploadClient } from '../lib/transport'
  import { sendSegments } from '../lib/send'
  import { reportClientError } from '../lib/logger'

  // hidden: keys/termux mode hides this component visually via CSS rather
  // than App.svelte unmounting it. Unmounting would destroy `text` and
  // `attachments` (including the uploaded File objects) every time the
  // user switches modes, forcing a re-upload on the way back — see
  // App.svelte's doc comment on why Promptbox stays mounted across all
  // three inputMode values.
  let {
    client = createHttpUploadClient(),
    hidden = false,
  }: { client?: UploadClient; hidden?: boolean } = $props()

  // Ticket 2 (not yet built) will route the URL path to a Herdr session;
  // reading location.pathname here today costs nothing and already agrees
  // with that future contract (and with session.go's own empty->"default"
  // fallback) without this component needing to change later.
  const session = location.pathname.replace(/^\//, '') || 'default'

  let textarea: HTMLTextAreaElement
  let fileInput: HTMLInputElement
  let text = $state('')
  let attachments = $state<Attachment[]>([])
  let sending = $state(false)
  let error = $state<{ message: string; refID?: string } | null>(null)
  let copied = $state(false)

  /** Inserts `§N` (N = the new attachment count) at the current caret, restoring caret after it. */
  async function insertAttachment(file: File, mime: string) {
    attachments = [...attachments, { id: crypto.randomUUID(), file, mime }]
    const marker = `§${attachments.length}`
    const caret = textarea?.selectionStart ?? text.length
    text = text.slice(0, caret) + marker + text.slice(caret)
    await tick()
    if (textarea) {
      const pos = caret + marker.length
      textarea.setSelectionRange(pos, pos)
      textarea.focus()
    }
  }

  function removeAttachment(n: number) {
    const result = removeMarker(text, attachments, n)
    text = result.text
    attachments = result.attachments
  }

  async function handlePaste(e: ClipboardEvent) {
    const dt = e.clipboardData
    if (!dt) return
    const fileItem = Array.from(dt.items).find((i) => i.kind === 'file')
    if (!fileItem) return // no file on the clipboard: let the textarea handle a plain text paste natively
    e.preventDefault()
    const file = fileItem.getAsFile()
    if (!file) return
    const mime = await resolveMime(file)
    await insertAttachment(file, mime)
  }

  async function handleFilesChosen(e: Event) {
    const files = Array.from((e.target as HTMLInputElement).files ?? [])
    for (const file of files) {
      const mime = await resolveMime(file)
      await insertAttachment(file, mime)
    }
    ;(e.target as HTMLInputElement).value = ''
  }

  async function handleSend() {
    if (sending) return
    sending = true
    error = null
    const segments = parseTemplate(text, attachments)
    const result = await sendSegments(client, segments, session)
    sending = false

    if (result.ok) {
      text = ''
      attachments = []
      return
    }

    // All-or-nothing: nothing was sent, so text/attachments stay exactly
    // as the user left them — no reset here on the failure path.
    error = { message: result.error, refID: result.refID }
    void reportClientError({ message: result.error, refID: result.refID })
  }

  async function copyRefID() {
    if (!error?.refID) return
    await navigator.clipboard.writeText(error.refID)
    copied = true
    setTimeout(() => (copied = false), 1500)
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      void handleSend()
    }
  }

  function handleInput() {
    if (error) error = null
  }
</script>

<div class="promptbox" class:hidden>
  {#if error}
    <div class="error" role="alert">
      <AlertTriangle size={16} aria-hidden="true" />
      <span class="error-message">{error.message}</span>
      {#if error.refID}
        <button type="button" class="ref" onclick={copyRefID} title="Copy correlation id">
          ref: {error.refID}
          <Copy size={12} aria-hidden="true" />
          {#if copied}<span class="copied">copied</span>{/if}
        </button>
      {/if}
    </div>
  {/if}

  {#if attachments.length > 0}
    <div class="attachments">
      {#each attachments as att, i (att.id)}
        <div class="attachment" animate:flip={{ duration: 180 }}>
          <div class="attachment-thumb">
            <AttachmentPreview file={att.file} mime={att.mime} />
            <MimeBadge mime={att.mime} />
          </div>
          <span class="attachment-marker">§{i + 1}</span>
          <button
            type="button"
            class="attachment-remove"
            aria-label={`Remove attachment §${i + 1}`}
            onclick={() => removeAttachment(i + 1)}
          >
            ×
          </button>
        </div>
      {/each}
    </div>
  {/if}

  <div class="row">
    <button
      type="button"
      class="attach"
      aria-label="Attach a file"
      onclick={() => fileInput.click()}
    >
      <Paperclip size={18} aria-hidden="true" />
    </button>
    <input
      bind:this={fileInput}
      type="file"
      multiple
      class="hidden-file-input"
      onchange={handleFilesChosen}
    />

    <textarea
      bind:this={textarea}
      bind:value={text}
      class="editor"
      rows="1"
      placeholder="Type a message…"
      aria-label="Message"
      autocorrect="off"
      autocapitalize="off"
      autocomplete="off"
      spellcheck="false"
      oninput={handleInput}
      onpaste={handlePaste}
      onkeydown={handleKeydown}
    ></textarea>

    <button type="button" class="send" aria-label="Send" disabled={sending} onclick={handleSend}>
      <Send size={18} aria-hidden="true" />
    </button>
  </div>
</div>

<style>
  .promptbox {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    background: var(--promptbox-bg, #0f172a);
    border-top: 1px solid var(--border, #1e293b);
  }

  /* keys/termux mode: hide visually, keep mounted (see the `hidden` prop's
     doc comment) so text/attachments state survives the mode switch. */
  .promptbox.hidden {
    display: none;
  }

  .error {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 0.6rem;
    border-radius: 0.4rem;
    background: var(--error-bg, #450a0a);
    color: var(--error-fg, #fecaca);
    font: 500 0.8rem/1.4 system-ui, sans-serif;
  }

  .error-message {
    flex: 1;
    word-break: break-word;
  }

  .ref {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    border: 0;
    background: rgba(255, 255, 255, 0.08);
    color: inherit;
    border-radius: 0.3rem;
    padding: 0.15rem 0.4rem;
    font: inherit;
    cursor: pointer;
    white-space: nowrap;
  }

  .copied {
    opacity: 0.8;
  }

  .attachments {
    display: flex;
    flex-wrap: wrap;
    /* Generous gap + top/side padding: MimeBadge and the remove button both
       sit in corners just outside the 3rem square (-0.35rem offset), so a
       tight gap would let neighbouring items' corner badges touch/overlap. */
    gap: 0.6rem;
    padding: 0.35rem 0.35rem 0;
  }

  .attachment {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
    width: 3rem;
    flex: none;
  }

  .attachment-thumb {
    position: relative;
    width: 3rem;
    height: 3rem;
    border-radius: 0.5rem;
    overflow: hidden;
    background: var(--muted-bg, #334155);
  }

  .attachment-marker {
    font: 600 0.7rem/1.2 ui-monospace, monospace;
    color: var(--muted-fg, #94a3b8);
  }

  .attachment-remove {
    position: absolute;
    top: -0.35rem;
    left: -0.35rem;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 1.1rem;
    height: 1.1rem;
    border: 0;
    border-radius: 50%;
    background: var(--badge-bg, #334155);
    color: var(--badge-fg, #f1f5f9);
    font: 700 0.75rem/1 system-ui, sans-serif;
    cursor: pointer;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
  }

  .attachment-remove:hover {
    opacity: 0.8;
  }

  .row {
    display: flex;
    align-items: flex-end;
    gap: 0.4rem;
  }

  .attach,
  .send {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 2.25rem;
    height: 2.25rem;
    border: 0;
    border-radius: 0.5rem;
    background: var(--button-bg, #1e293b);
    color: var(--button-fg, #e2e8f0);
    cursor: pointer;
  }

  .send:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .hidden-file-input {
    display: none;
  }

  .editor {
    flex: 1;
    min-height: 2.25rem;
    max-height: 8rem;
    overflow-y: auto;
    padding: 0.4rem 0.6rem;
    border-radius: 0.5rem;
    border: 0;
    resize: none;
    background: var(--editor-bg, #1e293b);
    color: var(--editor-fg, #e2e8f0);
    font: 400 0.9rem/1.5 system-ui, sans-serif;
    /* ponytail: native autogrow (Chrome 123+/Android Chrome); Firefox has
       no field-sizing support yet and falls back to a fixed one-line box
       that still scrolls fine via overflow-y — upgrade path if that ships
       broadly is an oninput scrollHeight-driven resize instead. */
    field-sizing: content;
  }

  .editor:focus {
    outline: 2px solid var(--focus-ring, #38bdf8);
    outline-offset: -1px;
  }
</style>
