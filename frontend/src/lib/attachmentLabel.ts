/**
 * Generic, mime-derived attachment labels for the promptbox pill. The
 * operator doesn't care about the real filename — the server resolves the
 * uploaded /tmp path itself, so a filename shown in the pill is purely
 * decorative, and a truncated camera/screenshot filename is noise, not
 * help. MimeBadge already shows the specific format (JPEG/PDF/ZIP/…), so
 * this stays deliberately generic and short.
 */
import { mimeBadge } from './mime'

const ARCHIVE_BADGES = new Set(['ZIP', 'GZIP', 'TAR'])

/** One generic label bucket per mime: Image / PDF / Text / Archive / File. */
export function attachmentLabel(mime: string): string {
  const m = mime.toLowerCase()
  if (m.startsWith('image/')) return 'Image'
  if (m === 'application/pdf') return 'PDF'
  if (m.startsWith('text/')) return 'Text'
  // Reuse mime.ts's own archive mapping rather than duplicating the mime
  // list here — ZIP/GZIP/TAR are exactly the archive badges it defines.
  const badge = mimeBadge(m)
  if (badge && ARCHIVE_BADGES.has(badge)) return 'Archive'
  return 'File'
}

/**
 * Labels a full attachment list, suffixing a 1-based index only when more
 * than one attachment shares a label (e.g. two images -> "Image 1" /
 * "Image 2"); a lone attachment of a kind stays plain ("Image").
 */
export function labelAttachments(mimes: readonly string[]): string[] {
  const labels = mimes.map(attachmentLabel)
  const counts = new Map<string, number>()
  for (const l of labels) counts.set(l, (counts.get(l) ?? 0) + 1)

  const seen = new Map<string, number>()
  return labels.map((l) => {
    if ((counts.get(l) ?? 0) <= 1) return l
    const next = (seen.get(l) ?? 0) + 1
    seen.set(l, next)
    return `${l} ${next}`
  })
}
