/**
 * Magic-byte mime sniffing, used only as the fallback when a pasted
 * clipboard item reports no usable `type` (design doc: "mime from item,
 * magic-byte sniff fallback" — browsers usually give a real mime for
 * clipboard files, but some sources, notably X11/Wayland screenshot tools
 * piped through certain clipboard managers, hand back `application/octet-
 * stream` or an empty string). Checks only the handful of formats the
 * attachment UI treats specially (image thumbnail vs PDF preview vs mime
 * badge) — this is a hint for the UI, not a general-purpose file-type
 * library, so it deliberately does not try to cover every format `mime.ts`
 * has a badge for.
 */
interface Signature {
  readonly mime: string
  readonly bytes: readonly number[]
  readonly offset?: number
}

const SIGNATURES: readonly Signature[] = [
  { mime: 'image/png', bytes: [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a] },
  { mime: 'image/jpeg', bytes: [0xff, 0xd8, 0xff] },
  { mime: 'image/gif', bytes: [0x47, 0x49, 0x46, 0x38] },
  { mime: 'application/pdf', bytes: [0x25, 0x50, 0x44, 0x46] }, // "%PDF"
  { mime: 'application/zip', bytes: [0x50, 0x4b, 0x03, 0x04] },
  { mime: 'application/gzip', bytes: [0x1f, 0x8b] },
  // WEBP: "RIFF"....."WEBP" — two disjoint runs, checked separately below.
]

function matches(bytes: Uint8Array, sig: Signature): boolean {
  const offset = sig.offset ?? 0
  if (bytes.length < offset + sig.bytes.length) return false
  return sig.bytes.every((b, i) => bytes[offset + i] === b)
}

/** Sniffs a mime type from the first bytes of a file. Returns '' if nothing matches. */
export function sniffMime(bytes: Uint8Array): string {
  for (const sig of SIGNATURES) {
    if (matches(bytes, sig)) return sig.mime
  }
  if (
    bytes.length >= 12 &&
    bytes[0] === 0x52 &&
    bytes[1] === 0x49 &&
    bytes[2] === 0x46 &&
    bytes[3] === 0x46 &&
    bytes[8] === 0x57 &&
    bytes[9] === 0x45 &&
    bytes[10] === 0x42 &&
    bytes[11] === 0x50
  ) {
    return 'image/webp'
  }
  return ''
}

/** Reads enough of file's head to sniff its mime type (magic-byte fallback path). */
export async function sniffFileMime(file: File): Promise<string> {
  const head = await file.slice(0, 16).arrayBuffer()
  return sniffMime(new Uint8Array(head))
}

/**
 * Resolves the mime type to use for a pasted/attached File: the browser-
 * reported type if it looks real, else a magic-byte sniff, else falls back
 * to the reported type verbatim (possibly empty) so callers always get a
 * string to work with.
 */
export async function resolveMime(file: File): Promise<string> {
  if (file.type && file.type !== 'application/octet-stream') return file.type
  const sniffed = await sniffFileMime(file)
  return sniffed || file.type
}
