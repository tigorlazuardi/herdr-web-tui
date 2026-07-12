<script lang="ts">
  /**
   * The artifact promptbox: an ordered segment editor (text runs + file
   * pills) over a contenteditable surface, plus the atomic all-or-nothing
   * send flow. See the design doc's "Artifact promptbox" section for the
   * full contract this implements.
   *
   * Editing model — DOM is the source of truth while typing, Svelte owns
   * layout: native contenteditable already gives free, correct behaviour
   * for plain text editing and (via `contenteditable="false"` pills, see
   * Pill.svelte) atomic single-backspace deletion, so this component does
   * not hand-roll a text buffer. Instead:
   *   1. The browser mutates the live DOM directly as the user types
   *      (that's what contenteditable does).
   *   2. On every `input` event, `readSegmentsFromDOM` walks
   *      `container.childNodes` and rebuilds `segments` from what's
   *      actually there, matching pills back to their File objects via
   *      `data-pill-id` and reusing each surviving text span's own
   *      `data-seg-id` as that segment's id.
   *   3. Feeding that rebuilt array back into Svelte's keyed `{#each}` is
   *      safe: Svelte compiles a `{seg.text}` interpolation to a direct
   *      `textNode.data = value` write, not a `textContent` reset, so
   *      handing it back its own just-typed value is a genuine no-op and
   *      the caret never jumps mid-word. Surviving pills keep their DOM
   *      identity too (same key), so only actually-removed pills animate
   *      out.
   *
   * ponytail: the one gap this shortcut leaves is a brand-new gap next to
   * a pill that was never given an initial empty text span (i.e. any path
   * that bypasses insertPillAtCaret's own trailing-empty-segment). The
   * very first keystroke into such a gap lands as a raw, unwrapped DOM
   * text node; the next rebuild replaces it with a managed span and the
   * caret can jump once. insertPillAtCaret always leaves a wrapped empty
   * segment right after an inserted pill specifically to avoid hitting
   * this path in the normal flow. Upgrade path if this bites for real:
   * a dedicated contenteditable model (ProseMirror/Lexical) that owns
   * every node itself instead of reading raw DOM back.
   */
  import { tick } from 'svelte'
  import { flip } from 'svelte/animate'
  import Paperclip from '@lucide/svelte/icons/paperclip'
  import Send from '@lucide/svelte/icons/send'
  import Copy from '@lucide/svelte/icons/copy'
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle'
  import Pill from './Pill.svelte'
  import AttachmentPreview from './AttachmentPreview.svelte'
  import MimeBadge from './MimeBadge.svelte'
  import type { PillSegment, Segment, TextSegment } from '../lib/segments'
  import { resolveMime } from '../lib/sniff'
  import { createHttpUploadClient, type UploadClient } from '../lib/transport'
  import { sendSegments } from '../lib/send'
  import { reportClientError } from '../lib/logger'
  import { labelAttachments } from '../lib/attachmentLabel'

  interface EditorTextSegment extends TextSegment {
    readonly id: string
  }
  type EditorSegment = EditorTextSegment | PillSegment

  function emptyTextSegment(): EditorTextSegment {
    return { kind: 'text', text: '', id: crypto.randomUUID() }
  }

  let { client = createHttpUploadClient() }: { client?: UploadClient } = $props()

  // Ticket 2 (not yet built) will route the URL path to a Herdr session;
  // reading location.pathname here today costs nothing and already agrees
  // with that future contract (and with session.go's own empty->"default"
  // fallback) without this component needing to change later.
  const session = location.pathname.replace(/^\//, '') || 'default'

  let container: HTMLDivElement
  let segments = $state<EditorSegment[]>([emptyTextSegment()])
  let sending = $state(false)
  let error = $state<{ message: string; refID?: string } | null>(null)
  let copied = $state(false)

  const pills = $derived(segments.filter((s): s is PillSegment => s.kind === 'pill'))

  // Generic per-pill label ("Image", "Archive 2", …) — computed here rather
  // than inside Pill.svelte because disambiguating index suffixes need the
  // whole attachment list, not just one pill's own mime.
  const pillLabels = $derived.by(() => {
    const labels = labelAttachments(pills.map((p) => p.mime))
    return new Map(pills.map((p, i) => [p.id, labels[i]]))
  })

  function readSegmentsFromDOM(el: HTMLElement): EditorSegment[] {
    const pillById = new Map(pills.map((p) => [p.id, p]))
    const result: EditorSegment[] = []
    let pendingText = ''
    let pendingId: string | null = null

    function flush() {
      if (pendingText !== '' || pendingId !== null) {
        result.push({ kind: 'text', text: pendingText, id: pendingId ?? crypto.randomUUID() })
      }
      pendingText = ''
      pendingId = null
    }

    for (const node of Array.from(el.childNodes)) {
      const elNode = node instanceof HTMLElement ? node : null
      const pillId = elNode?.dataset.pillId
      const segId = elNode?.dataset.segId

      if (pillId) {
        flush()
        const pill = pillById.get(pillId)
        if (pill) result.push(pill)
        continue
      }
      if (segId) {
        flush()
        result.push({ kind: 'text', text: node.textContent ?? '', id: segId })
        continue
      }
      // Unwrapped node (see the ponytail note above): fold its text into
      // the current run rather than dropping it.
      pendingId ??= crypto.randomUUID()
      pendingText += node.textContent ?? ''
    }
    flush()

    return result.length > 0 ? result : [emptyTextSegment()]
  }

  function handleInput() {
    segments = readSegmentsFromDOM(container)
    if (error) error = null
  }

  async function focusAfterPill(pillId: string) {
    await tick()
    const pillEl = container.querySelector<HTMLElement>(`[data-pill-id="${pillId}"]`)
    const afterEl = pillEl?.nextElementSibling as HTMLElement | null
    const target = afterEl?.firstChild ?? afterEl
    if (!target) return
    const range = document.createRange()
    range.setStart(target, 0)
    range.collapse(true)
    const sel = window.getSelection()
    sel?.removeAllRanges()
    sel?.addRange(range)
    container.focus()
  }

  /**
   * Inserts a pill at the current caret position (splitting the text
   * segment it falls inside into a before/after pair), or appends to the
   * end if there is no caret in this editor (e.g. the attach button was
   * clicked without the editor focused). Always leaves a fresh empty text
   * segment right after the pill — see this component's doc comment on why.
   */
  function insertPillAtCaret(file: File, mime: string) {
    const pill: PillSegment = { kind: 'pill', id: crypto.randomUUID(), file, mime }
    const sel = window.getSelection()

    let splitAt: { segIndex: number; offset: number } | null = null
    if (sel && sel.rangeCount > 0 && container.contains(sel.anchorNode)) {
      const range = sel.getRangeAt(0)
      const node = range.startContainer
      const parentEl = node instanceof HTMLElement ? node : node.parentElement
      const segId = parentEl?.closest<HTMLElement>('[data-seg-id]')?.dataset.segId
      if (segId) {
        const segIndex = segments.findIndex((s) => s.kind === 'text' && s.id === segId)
        if (segIndex !== -1) splitAt = { segIndex, offset: range.startOffset }
      }
    }

    if (splitAt) {
      const seg = segments[splitAt.segIndex] as EditorTextSegment
      const before: EditorTextSegment = { kind: 'text', text: seg.text.slice(0, splitAt.offset), id: seg.id }
      const after: EditorTextSegment = {
        kind: 'text',
        text: seg.text.slice(splitAt.offset),
        id: crypto.randomUUID(),
      }
      segments = [
        ...segments.slice(0, splitAt.segIndex),
        before,
        pill,
        after,
        ...segments.slice(splitAt.segIndex + 1),
      ]
    } else {
      segments = [...segments, pill, emptyTextSegment()]
    }
    void focusAfterPill(pill.id)
  }

  function removePill(id: string) {
    segments = segments.filter((s) => s.id !== id)
    if (segments.length === 0) segments = [emptyTextSegment()]
  }

  async function handlePaste(e: ClipboardEvent) {
    const dt = e.clipboardData
    if (!dt) return
    e.preventDefault()

    const fileItem = Array.from(dt.items).find((i) => i.kind === 'file')
    if (fileItem) {
      const file = fileItem.getAsFile()
      if (!file) return
      const mime = await resolveMime(file)
      insertPillAtCaret(file, mime)
      return
    }
    document.execCommand('insertText', false, dt.getData('text/plain'))
  }

  let fileInput: HTMLInputElement

  async function handleFilesChosen(e: Event) {
    const files = Array.from((e.target as HTMLInputElement).files ?? [])
    for (const file of files) {
      const mime = await resolveMime(file)
      insertPillAtCaret(file, mime)
    }
    ;(e.target as HTMLInputElement).value = ''
  }

  async function handleSend() {
    if (sending) return
    sending = true
    error = null
    const plain: Segment[] = segments.map((s) =>
      s.kind === 'text' ? { kind: 'text', text: s.text } : s,
    )
    const result = await sendSegments(client, plain, session)
    sending = false

    if (result.ok) {
      segments = [emptyTextSegment()]
      if (container) container.textContent = ''
      return
    }

    // All-or-nothing: nothing was sent, so segments/DOM stay exactly as
    // the user left them — no reset here on the failure path.
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
</script>

<div class="promptbox">
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

  {#if pills.length > 0}
    <!-- Compact thumbnail-only strip — the full labeled Pill chip already
         lives inline in the editor below, so repeating it here at 3rem
         square just crammed a wide chip into a tiny box and jumbled the
         text (mobile-ux-v2.md: thumbnail + mime badge, no filename). -->
    <div class="attachments">
      {#each pills as pill (pill.id)}
        <div class="attachment" animate:flip={{ duration: 180 }}>
          <div class="attachment-thumb">
            <AttachmentPreview file={pill.file} mime={pill.mime} />
            <MimeBadge mime={pill.mime} />
          </div>
          <button
            type="button"
            class="attachment-remove"
            aria-label={`Remove ${pill.file.name}`}
            onclick={() => removePill(pill.id)}
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

    <div
      class="editor"
      bind:this={container}
      contenteditable="true"
      role="textbox"
      tabindex="0"
      aria-multiline="true"
      aria-label="Message"
      oninput={handleInput}
      onpaste={handlePaste}
      onkeydown={handleKeydown}
    >
      {#each segments as seg (seg.id)}
        {#if seg.kind === 'text'}
          <span data-seg-id={seg.id}>{seg.text}</span
          >{:else}<Pill
            id={seg.id}
            file={seg.file}
            mime={seg.mime}
            label={pillLabels.get(seg.id) ?? 'File'}
            onRemove={() => removePill(seg.id)}
          />{/if}
      {/each}
    </div>

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
    width: 3rem;
    height: 3rem;
    flex: none;
  }

  .attachment-thumb {
    position: relative;
    width: 100%;
    height: 100%;
    border-radius: 0.5rem;
    overflow: hidden;
    background: var(--muted-bg, #334155);
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
    background: var(--editor-bg, #1e293b);
    color: var(--editor-fg, #e2e8f0);
    font: 400 0.9rem/1.5 system-ui, sans-serif;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .editor:focus {
    outline: 2px solid var(--focus-ring, #38bdf8);
    outline-offset: -1px;
  }
</style>
