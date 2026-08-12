import { describe, expect, it } from 'vitest'
import { sendSegments } from './send'
import type { Segment } from './segments'
import type { SendResult, UploadClient } from './transport'

function fakeClient(result: SendResult): UploadClient & { calls: unknown[] } {
  const calls: unknown[] = []
  return {
    calls,
    async send(req, session, submitKey) {
      calls.push({ req, session, submitKey })
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
      {
        req: { template: { segments: [{ text: 'hello' }] }, files: [] },
        session: 'work',
        submitKey: 'enter',
      },
    ])
  })

  it('forwards alternate submit without changing serialized prompt', async () => {
    const client = fakeClient({ ok: true })
    await sendSegments(client, [{ kind: 'text', text: 'hello' }], 'work', 'ctrl-enter')

    expect(client.calls).toEqual([
      {
        req: { template: { segments: [{ text: 'hello' }] }, files: [] },
        session: 'work',
        submitKey: 'ctrl-enter',
      },
    ])
  })

  it('surfaces a transport failure verbatim, including the ref id', async () => {
    const client = fakeClient({ ok: false, error: 'inject failed: boom', refID: 'req_abc' })
    const result = await sendSegments(client, [{ kind: 'text', text: 'x' }], 'default')

    expect(result).toEqual({ ok: false, error: 'inject failed: boom', refID: 'req_abc' })
  })
})
