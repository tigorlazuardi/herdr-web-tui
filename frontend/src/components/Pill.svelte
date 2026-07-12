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

  let {
    id,
    file,
    mime,
    onRemove,
  }: { id: string; file: File; mime: string; onRemove: () => void } = $props()
</script>

<span class="pill" contenteditable="false" data-pill-id={id}>
  <span class="thumb-wrap">
    <AttachmentPreview {file} {mime} />
  </span>
  <span class="name">{file.name}</span>
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
    max-width: 10rem;
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
