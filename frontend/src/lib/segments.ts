/**
 * Pure, DOM-free segment model + serializer for the promptbox editor (see
 * the design doc's "Artifact promptbox" section). This module never touches
 * the DOM or File contents — it only reads File.name/type — which is what
 * makes it cheaply unit-testable (Vitest, no jsdom needed) and safe to run
 * with no browser at all, per the design doc's testing decision ("the
 * ordering/marker logic is pure and DOM-free").
 *
 * The editor component (components/Promptbox.svelte) owns turning
 * contenteditable DOM into a Segment[]; this module owns turning that
 * Segment[] into the exact wire shape POST /send expects.
 */

/** One literal text run typed by the user. */
export interface TextSegment {
  readonly kind: 'text'
  readonly text: string
}

/**
 * One atomic file attachment ("pill"). `id` is a client-only stable key for
 * Svelte keyed blocks / DOM lookups — it is never sent to the server; only
 * array position (which becomes the multipart field name) carries meaning
 * to the backend.
 */
export interface PillSegment {
  readonly kind: 'pill'
  readonly id: string
  readonly file: File
  readonly mime: string
}

export type Segment = TextSegment | PillSegment

/**
 * Wire shape of one segment, matching the Go backend's artifact.Segment
 * exactly (internal/artifact/template.go) — a text run has `text` set, a
 * file marker has `file` set to the multipart form field name carrying that
 * attachment. Never both, never neither.
 */
export interface WireSegment {
  text?: string
  file?: string
}

/** Wire shape of the "template" multipart field: internal/artifact/template.go's Template. */
export interface Template {
  segments: WireSegment[]
}

/** One file part serialize() says the transport must attach, in order. */
export interface SerializedFile {
  readonly fieldName: string
  readonly file: File
}

export interface SerializedRequest {
  readonly template: Template
  readonly files: readonly SerializedFile[]
}

/**
 * serialize turns the editor's ordered segment list into the exact
 * {template, files} shape POST /send's multipart contract expects (see
 * internal/server/send.go's readParts + internal/artifact/template.go's
 * Resolve). It is a straight 1:1 positional map — no filtering, no
 * reordering — because array order alone is the ordering invariant the
 * backend trusts (design doc: "ordering inherent in pill position", no
 * separate index field). Field names are assigned "file0", "file1", ...
 * strictly by pill position among `segments`; call serialize() fresh after
 * any edit or reorder rather than reusing a stale field-name assignment.
 */
export function serialize(segments: readonly Segment[]): SerializedRequest {
  const files: SerializedFile[] = []
  const wireSegments: WireSegment[] = segments.map((seg) => {
    if (seg.kind === 'text') {
      return { text: seg.text }
    }
    const fieldName = `file${files.length}`
    files.push({ fieldName, file: seg.file })
    return { file: fieldName }
  })
  return { template: { segments: wireSegments }, files }
}
