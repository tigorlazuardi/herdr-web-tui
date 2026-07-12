/**
 * Middle-ellipsis a file name down to `max` characters, keeping the
 * extension intact (mobile-ux-v2.md issue #1: Pill.svelte was rendering
 * the raw camera/screenshot filename — often 40+ chars of timestamp —
 * inside a fixed-width chip whose mime badge already conveys the file
 * type, so the visible name only needs to stay short and recognisable,
 * not complete). Truncating the middle rather than just the tail keeps
 * both a recognisable prefix and suffix (e.g. a date-stamped name still
 * shows its trailing sequence number) instead of a run of identical
 * "Screenshot_2026-…" prefixes when several are attached at once.
 */
export function shortName(name: string, max: number): string {
  if (name.length <= max) return name

  const dot = name.lastIndexOf('.')
  // No extension, or the dot is the first char (dotfile) or the last char
  // (trailing dot): nothing worth preserving separately from the stem.
  const hasExt = dot > 0 && dot < name.length - 1
  const ext = hasExt ? name.slice(dot) : ''
  const stem = hasExt ? name.slice(0, dot) : name

  const budget = max - ext.length - 1 // 1 char reserved for the ellipsis
  if (budget <= 0) return `…${ext}`.slice(0, max)

  const head = Math.ceil(budget / 2)
  const tail = Math.floor(budget / 2)
  return `${stem.slice(0, head)}…${stem.slice(stem.length - tail)}${ext}`
}
