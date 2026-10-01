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
  import { onDestroy, tick } from 'svelte'
  import { flip } from 'svelte/animate'
  import Paperclip from '@lucide/svelte/icons/paperclip'
  import Send from '@lucide/svelte/icons/send'
  import Copy from '@lucide/svelte/icons/copy'
  import X from '@lucide/svelte/icons/x'
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle'
  import { Button, Popover } from 'sve-ui'
  import AttachmentPreview from './AttachmentPreview.svelte'
  import MimeBadge from './MimeBadge.svelte'
  import { parseTemplate, removeMarker, type Attachment } from '../lib/markers'
  import { resolveMime } from '../lib/sniff'
  import {
    createHttpUploadClient,
    type SubmitKey,
    type UploadClient,
  } from '../lib/transport'
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
  let submitMenuOpen = $state(false)
  let suppressSendClick = false
  let pressTimer: ReturnType<typeof setTimeout> | null = null
  let suppressResetTimer: ReturnType<typeof setTimeout> | null = null
  let pointerStart: { x: number; y: number } | null = null

  /** Inserts `§N` (N = the new attachment count) at the current caret, restoring caret after it. */
  async function insertAttachment(file: File, mime: string) {
    // ponytail: randomUUID needs a secure context; random bytes keep this UI-only key unique over Tailscale HTTP.
    const id = crypto.randomUUID?.() ?? [...crypto.getRandomValues(new Uint32Array(4))].join('-')
    attachments = [...attachments, { id, file, mime }]
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

  async function handleSend(submitKey: SubmitKey = 'enter') {
    if (sending) return
    submitMenuOpen = false
    sending = true
    error = null
    const segments = parseTemplate(text, attachments)
    const result = await sendSegments(client, segments, session, submitKey)
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

  /** Long press opens alternate submit; suppression lasts only through same gesture's click. */
  function startSubmitPress(event: PointerEvent) {
    if (sending || event.button !== 0) return
    pointerStart = { x: event.clientX, y: event.clientY }
    ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
    pressTimer = setTimeout(() => {
      pressTimer = null
      pointerStart = null
      suppressSendClick = true
      submitMenuOpen = true
    }, 500)
  }

  function moveSubmitPointer(event: PointerEvent) {
    if (!pointerStart) return
    if (Math.abs(event.clientX - pointerStart.x) + Math.abs(event.clientY - pointerStart.y) > 8) {
      resetSubmitPress()
      suppressSendClick = true
    }
  }

  function resetSubmitPress() {
    if (pressTimer) clearTimeout(pressTimer)
    pressTimer = null
    pointerStart = null
  }

  function finishSubmitPress() {
    resetSubmitPress()
    if (!suppressSendClick) return
    suppressResetTimer = setTimeout(() => {
      suppressSendClick = false
      suppressResetTimer = null
    }, 0)
  }

  function cancelSubmitPress() {
    resetSubmitPress()
    suppressSendClick = false
  }

  function handleSendClick(event: MouseEvent, triggerClick?: unknown) {
    if (suppressSendClick) {
      suppressSendClick = false
      if (suppressResetTimer) clearTimeout(suppressResetTimer)
      suppressResetTimer = null
      return
    }
    // Keyboard activation opens choices; textarea Enter remains normal send.
    if (event.detail === 0) {
      if (typeof triggerClick === 'function') triggerClick(event)
      return
    }
    void handleSend()
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !matchMedia('(max-width: 40rem)').matches) {
      e.preventDefault()
      void handleSend()
    }
  }

  function handleInput() {
    if (error) error = null
  }

  onDestroy(() => {
    cancelSubmitPress()
    if (suppressResetTimer) clearTimeout(suppressResetTimer)
  })
</script>

<div class="promptbox" class:hidden>
  {#if error}
    <div class="error" role="alert">
      <AlertTriangle size={16} aria-hidden="true" />
      <span class="error-message">{error.message}</span>
      {#if error.refID}
        <Button variant="ghost" size="sm" class="ref" onclick={copyRefID} title="Copy correlation id">
          ref: {error.refID}
          <Copy size={12} aria-hidden="true" />
          {#if copied}<span class="copied">copied</span>{/if}
        </Button>
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
          <Button
            variant="solid"
            color="default"
            size="sm"
            class="attachment-remove"
            aria-label={`Remove attachment §${i + 1}`}
            onclick={() => removeAttachment(i + 1)}
          >
            <X size={12} aria-hidden="true" />
          </Button>
        </div>
      {/each}
    </div>
  {/if}

  <div class="row">
    <Button
      variant="flat"
      color="default"
      size="sm"
      class="attach"
      aria-label="Attach a file"
      onclick={() => fileInput.click()}
    >
      <Paperclip size={18} aria-hidden="true" />
    </Button>
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
      {...{ autocorrect: 'off' }}
      autocapitalize="off"
      autocomplete="off"
      spellcheck="false"
      oninput={handleInput}
      onpaste={handlePaste}
      onkeydown={handleKeydown}
    ></textarea>

    <Popover.Root bind:open={submitMenuOpen}>
      <Popover.Trigger>
        {#snippet child({ props })}
          <Button
            {...props}
            variant="solid"
            color="primary"
            size="sm"
            class="send"
            aria-label="Send (right-click, long-press, or keyboard activate for alternate submit)"
            title="Send · right-click or long-press for alternate submit"
            disabled={sending}
            onpointerdown={startSubmitPress}
            onpointermove={moveSubmitPointer}
            onpointerup={finishSubmitPress}
            onpointercancel={cancelSubmitPress}
            oncontextmenu={(event) => {
              event.preventDefault()
              submitMenuOpen = true
            }}
            onclick={(event) => handleSendClick(event, props.onclick)}
          >
            <Send size={18} aria-hidden="true" />
          </Button>
        {/snippet}
      </Popover.Trigger>
      <Popover.Content side="top" align="end" sideOffset={8} class="submit-menu">
        <p>Submit with</p>
        <Button variant="ghost" size="sm" onclick={() => void handleSend('ctrl-enter')}>
          Ctrl+Enter
        </Button>
        <Button variant="ghost" size="sm" onclick={() => void handleSend('alt-enter')}>
          Alt+Enter
        </Button>
      </Popover.Content>
    </Popover.Root>
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
    --composer-control-height: 2.25rem;
    display: flex;
    align-items: flex-end;
    gap: 0.4rem;
  }

  :global(.attach.sve-button),
  :global(.send.sve-button) {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--composer-control-height);
    height: var(--composer-control-height);
    border: 0;
    border-radius: 0.5rem;
    background: var(--button-bg, #1e293b);
    color: var(--button-fg, #e2e8f0);
    cursor: pointer;
  }

  :global(.send.sve-button:disabled) {
    opacity: 0.5;
    cursor: default;
  }

  :global(.submit-menu.sve-popover-content) {
    display: grid;
    gap: 0.25rem;
    min-width: 9rem;
    padding: 0.4rem;
    background: #1c1917;
    color: #e7e5e4;
  }

  :global(.submit-menu p) {
    margin: 0;
    padding: 0.3rem 0.55rem;
    color: #a8a29e;
    font: 600 0.7rem/1 system-ui, sans-serif;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .hidden-file-input {
    display: none;
  }

  .editor {
    flex: 1;
    min-height: var(--composer-control-height);
    max-height: calc(3 * 1.35rem + 0.8rem);
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
