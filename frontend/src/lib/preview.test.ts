import { describe, expect, it, vi } from 'vitest'
import { createPanePreviewClient } from './preview'

describe('createPanePreviewClient', () => {
  it('uses URL-path session and returns raw text', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ text: '  # raw\n  value\n' }), { status: 200 }),
    )
    const result = await createPanePreviewClient(fetcher).read('work', new AbortController().signal)

    expect(fetcher).toHaveBeenCalledWith('/api/pane-preview/work', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(result).toEqual({ ok: true, text: '  # raw\n  value\n' })
  })

  it('preserves response error and correlation reference', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: 'preview failed: socket closed' }), {
        status: 500,
        headers: { 'X-Request-Id': 'req_123' },
      }),
    )
    const result = await createPanePreviewClient(fetcher).read('', new AbortController().signal)

    expect(result).toEqual({ ok: false, error: 'preview failed: socket closed', refID: 'req_123' })
  })

  it('returns network error when fetch rejects', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('offline'))

    const result = await createPanePreviewClient(fetcher).read('work', new AbortController().signal)

    expect(result).toEqual({ ok: false, error: 'network error: offline' })
  })

  it('returns HTTP failure and correlation reference for non-JSON response', async () => {
    const fetcher = vi.fn().mockResolvedValue(
      new Response('bad gateway', { status: 502, headers: { 'X-Request-Id': 'req_502' } }),
    )

    const result = await createPanePreviewClient(fetcher).read('work', new AbortController().signal)

    expect(result).toEqual({ ok: false, error: 'preview failed: HTTP 502', refID: 'req_502' })
  })
})
