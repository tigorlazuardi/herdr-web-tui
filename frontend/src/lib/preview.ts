/** Fresh focused-pane preview transport. Raw text is deliberately not parsed or rendered. */
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
