/**
 * Canonical short mime badge labels for the attachment list (design doc:
 * "corner mime badge ... TXT/JPEG/PDF/PNG/ZIP/TAR/GZIP ... unknown -> no
 * badge"). Pure lookup, no DOM — kept separate from Pill/MimeBadge.svelte so
 * the mapping itself is trivially testable/extensible without touching a
 * component.
 */
const BADGES: Record<string, string> = {
  'text/plain': 'TXT',
  'image/jpeg': 'JPEG',
  'image/png': 'PNG',
  'image/gif': 'GIF',
  'image/webp': 'WEBP',
  'image/svg+xml': 'SVG',
  'application/pdf': 'PDF',
  'application/zip': 'ZIP',
  'application/x-zip-compressed': 'ZIP',
  'application/gzip': 'GZIP',
  'application/x-gzip': 'GZIP',
  'application/x-tar': 'TAR',
  'application/json': 'JSON',
}

/** Returns the badge label for mime, or undefined for anything unmapped (no badge). */
export function mimeBadge(mime: string): string | undefined {
  return BADGES[mime.toLowerCase()]
}

/** True for any mime this app can render an image thumbnail for (native <img>). */
export function isImageMime(mime: string): boolean {
  return mime.startsWith('image/')
}

/** True for the one mime that gets the lazy pdf.js preview. */
export function isPdfMime(mime: string): boolean {
  return mime === 'application/pdf'
}
