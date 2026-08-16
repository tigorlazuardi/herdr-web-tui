/** Fresh focused-pane preview transport. */
export interface PreviewSuccess {
  readonly ok: true
  readonly text: string
}

export interface PreviewFailure {
  readonly ok: false
  readonly error: string
  readonly refID?: string
}

export type PreviewResult = PreviewSuccess | PreviewFailure

export interface PanePreviewClient {
  read(session: string, signal: AbortSignal): Promise<PreviewResult>
}

type Fetcher = typeof fetch

export interface PreviewTextSegment {
  readonly text: string
  readonly href?: string
}

/** Splits plain terminal text into safe HTTP(S) links while preserving every character. */
export function splitPreviewLinks(text: string): PreviewTextSegment[] {
  const segments: PreviewTextSegment[] = []
  const urlPattern = /https?:\/\/[^\s<>"']+/gi
  let cursor = 0

  for (const match of text.matchAll(urlPattern)) {
    const start = match.index
    const raw = match[0]
    // ponytail: trim common prose punctuation; add OSC 8 parsing if terminal-native hyperlinks become required.
    const href = raw.replace(/[),.;:!?\]}]+$/, '')
    if (start > cursor) segments.push({ text: text.slice(cursor, start) })
    segments.push({ text: href, href })
    if (href.length < raw.length) segments.push({ text: raw.slice(href.length) })
    cursor = start + raw.length
  }

  if (cursor < text.length) segments.push({ text: text.slice(cursor) })
  return segments
}

/** Uses URL-path session routing; every call is one uncached browser request. */
export function createPanePreviewClient(fetcher: Fetcher = fetch): PanePreviewClient {
  return {
    async read(session, signal) {
      let response: Response
      try {
        response = await fetcher(`/api/pane-preview/${encodeURIComponent(session)}`, { signal })
      } catch (err) {
        return { ok: false, error: `network error: ${(err as Error).message}` }
      }

      const refID = response.headers.get('X-Request-Id') ?? undefined
      let body: { text?: unknown; error?: unknown } | undefined
      try {
        body = (await response.json()) as { text?: unknown; error?: unknown }
      } catch {
        return { ok: false, error: `preview failed: HTTP ${response.status}`, refID }
      }
      if (response.ok && typeof body.text === 'string') return { ok: true, text: body.text }
      return {
        ok: false,
        error: typeof body.error === 'string' ? body.error : `preview failed: HTTP ${response.status}`,
        refID,
      }
    },
  }
}
