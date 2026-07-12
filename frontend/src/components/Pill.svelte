<script lang="ts">
  /**
   * One atomic file attachment chip. `contenteditable="false"` on the root
   * is the whole trick behind "backspace deletes it whole" (design doc):
   * inside a contenteditable ancestor, a nested element marked
   * non-editable is treated by the browser as a single opaque unit for
   * caret movement and deletion — one Backspace removes the entire node,
   * never one character of its contents. Promptbox.svelte relies on this
   * native behaviour instead of hand-rolling caret/selection logic.
   */
  import MimeBadge from './MimeBadge.svelte'
  import AttachmentPreview from './AttachmentPreview.svelte'
  import { shortName } from '../lib/filename'

  let {
    id,
    file,
    mime,
    onRemove,
  }: { id: string; file: File; mime: string; onRemove: () => void } = $props()

  // mobile-ux-v2.md issue #1: short in EVERY view, not just narrow ones —
  // the mime badge already conveys type, so the full (often 40+ char)
  // camera/screenshot filename doesn't need to fit. 20 chars is the CSS
  // budget below (.name max-width); kept in sync manually since one is a
  // char count and the other a rendered width.
  const displayName = $derived(shortName(file.name, 20))
</script>

<span class="pill" contenteditable="false" data-pill-id={id}>
  <span class="thumb-wrap">
    <AttachmentPreview {file} {mime} />
  </span>
  <span class="name" title={file.name}>{displayName}</span>
  <MimeBadge {mime} />
  <button
    type="button"
    class="remove"
    aria-label={`Remove ${file.name}`}
    onclick={onRemove}
  >
    ×
  </button>
</span>

<style>
  .pill {
    position: relative;
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
    margin: 0 0.15rem;
    padding: 0.15rem 0.5rem 0.15rem 0.15rem;
    border-radius: 999px;
    background: var(--pill-bg, #1e293b);
    color: var(--pill-fg, #e2e8f0);
    font: 500 0.8rem/1.4 system-ui, sans-serif;
    user-select: none;
    vertical-align: middle;
  }

  .thumb-wrap {
    display: block;
    width: 1.3rem;
    height: 1.3rem;
    border-radius: 50%;
    overflow: hidden;
    background: var(--muted-bg, #334155);
    flex: none;
  }

  .name {
    /* Primary truncation is shortName() above (JS, so the extension is
       always preserved); this is just a defensive backstop in case a
       future caller passes a larger max. Must stay wide enough that this
       CSS ellipsis never fires on shortName's own 20-char output — 8rem
       was too tight and clipped the extension shortName kept, producing
       a double-ellipsis ("Screensh…ng-name…" instead of
       "Screensh…ng-name.jpg"); 11rem clears 20 chars at this font size
       with margin. */
    max-width: 11rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .remove {
    border: 0;
    background: transparent;
    color: inherit;
    opacity: 0.6;
    cursor: pointer;
    font: inherit;
    line-height: 1;
    padding: 0 0.1rem;
  }

  .remove:hover {
    opacity: 1;
  }
</style>
