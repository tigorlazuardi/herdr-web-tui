/**
 * Pure, DOM-free marker parsing for the promptbox's plain-textarea model
 * (replaces the old contenteditable pill editor — see Promptbox.svelte's
 * doc comment for why: heavy flicker + caret jumps fighting the mobile
 * IME on every keystroke). A marker is the literal token `§N` (U+00A7
 * SECTION SIGN followed by 1-based digits) that the attach button/paste
 * handler inserts at the caret. `§` is awkward to type on a phone
 * keyboard, which is the point: the token is only ever produced
 * programmatically, so normal prose typed by the user — even mangled by
 * autocorrect — will essentially never produce `§` followed by digits by
 * accident. No collision, no escaping needed.
 *
 * This module never touches the DOM or File contents (only File
 * references), which is what makes it cheaply unit-testable and safe to
 * run with no browser at all — same shape as segments.ts's own doc
 * comment on why that module is pure.
 */
import type { Segment } from './segments'

/** One ordered attachment slot; `id` is a client-only stable key (never sent to the server). */
export interface Attachment {
  readonly id: string
  readonly file: File
  readonly mime: string
}

/**
 * Splits `text` on `§N` tokens into the ordered Segment[] serialize()
 * expects (segments.ts): each literal run between tokens becomes a
 * TextSegment (verbatim, empty runs dropped), each in-range `§N` becomes
 * the pill for `attachments[N-1]`. A `§N` whose N has no attachment
 * (out of range — the user deleted that attachment, or is mid-typing a
 * number that isn't a real marker yet) is left as literal text rather
 * than dropped, so nothing the user typed silently disappears.
 *
 * Referencing the same attachment twice (`"§1 ... §1"`) is allowed and
 * produces two pill segments wrapping the same File — serialize() assigns
 * them distinct field names (file0, file1, ...) and uploads the file
 * twice, which is wasteful but positionally correct; not worth a dedup
 * pass for what is expected to be a rare, harmless case.
 */
export function parseTemplate(text: string, attachments: readonly Attachment[]): Segment[] {
  const segments: Segment[] = []
  let lastIndex = 0
  for (const match of text.matchAll(/§(\d+)/g)) {
    const attachment = attachments[Number(match[1]) - 1]
    if (!attachment) continue // out of range: leave as literal text, keep scanning
    const index = match.index
    if (index > lastIndex) segments.push({ kind: 'text', text: text.slice(lastIndex, index) })
    segments.push({ kind: 'pill', id: attachment.id, file: attachment.file, mime: attachment.mime })
    lastIndex = index + match[0].length
  }
  if (lastIndex < text.length) segments.push({ kind: 'text', text: text.slice(lastIndex) })
  return segments
}

/**
 * Removes attachment N (1-based) from `attachments` and rewrites `text`:
 * deletes every `§N` token that referenced it (all occurrences, per the
 * duplicate-reference note above), then renumbers the remaining
 * attachments — and their surviving `§N` tokens — contiguously to 1..k so
 * on-screen markers always match the attachment list's display order. N
 * out of range is a no-op (returns the inputs unchanged).
 */
export function removeMarker(
  text: string,
  attachments: readonly Attachment[],
  n: number,
): { text: string; attachments: Attachment[] } {
  if (n < 1 || n > attachments.length) return { text, attachments: [...attachments] }
  const nextAttachments = attachments.filter((_, i) => i !== n - 1)
  const withoutToken = text.replace(new RegExp(`§${n}(?!\\d)`, 'g'), '')
  return { text: renumberMarkers(withoutToken, n), attachments: nextAttachments }
}

/** Shifts every `§N` token with N > removedN down by one; N <= removedN is untouched. */
function renumberMarkers(text: string, removedN: number): string {
  return text.replace(/§(\d+)/g, (full, digits: string) => {
    const n = Number(digits)
    return n > removedN ? `§${n - 1}` : full
  })
}
