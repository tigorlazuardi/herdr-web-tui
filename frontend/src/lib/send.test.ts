import { describe, expect, it } from 'vitest'
import { sendSegments } from './send'
import type { Segment } from './segments'
import type { SendResult, UploadClient } from './transport'

function fakeClient(result: SendResult): UploadClient & { calls: unknown[] } {
  const calls: unknown[] = []
  return {
    calls,
    async send(req, session) {
      calls.push({ req, session })
      return result
    },
  }
}

describe('sendSegments', () => {
  it('serializes segments and forwards to the client with the session', async () => {
    const client = fakeClient({ ok: true })
    const segments: Segment[] = [{ kind: 'text', text: 'hello' }]
    const result = await sendSegments(client, segments, 'work')

    expect(result).toEqual({ ok: true })
    expect(client.calls).toEqual([
      { req: { template: { segments: [{ text: 'hello' }] }, files: [] }, session: 'work' },
    ])
  })

  it('surfaces a transport failure verbatim, including the ref id', async () => {
    const client = fakeClient({ ok: false, error: 'inject failed: boom', refID: 'req_abc' })
    const result = await sendSegments(client, [{ kind: 'text', text: 'x' }], 'default')

    expect(result).toEqual({ ok: false, error: 'inject failed: boom', refID: 'req_abc' })
  })
})
