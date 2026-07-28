import { afterEach, describe, expect, it, vi } from 'vitest'
import { consumePaneFocus, disablePush, enablePush, focusPane, initialPushFeedback, pushConfig, registerPushWorker, responseError, togglePush } from './push'

afterEach(() => vi.unstubAllGlobals())

describe('push worker registration', () => {
  it('registers worker with root scope and returns registration on supported platforms', async () => {
    const registration = {} as ServiceWorkerRegistration
    const register = vi.fn().mockResolvedValue(registration)
    vi.stubGlobal('navigator', { serviceWorker: { register } })
    vi.stubGlobal('window', { PushManager: class {}, Notification: class {} })

    await expect(registerPushWorker()).resolves.toBe(registration)
    expect(register).toHaveBeenCalledExactlyOnceWith('/sw.js', { scope: '/' })
  })

  it('returns null without registering on unsupported platforms', async () => {
    const register = vi.fn()
    vi.stubGlobal('navigator', { serviceWorker: { register } })
    vi.stubGlobal('window', {})

    await expect(registerPushWorker()).resolves.toBeNull()
    expect(register).not.toHaveBeenCalled()
  })
})

describe('push lifecycle', () => {
  it('subscribes and saves native subscription', async () => {
    const subscription = { unsubscribe: vi.fn(), toJSON: () => ({ endpoint: 'https://push.example/x' }) }
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue(null), subscribe: vi.fn().mockResolvedValue(subscription) } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('Notification', { requestPermission: vi.fn().mockResolvedValue('granted') })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ publicKey: 'YQ', enabled: false }))).mockResolvedValueOnce(new Response(null, { status: 204 })))
    await enablePush(registration)
    expect(registration.pushManager.subscribe).toHaveBeenCalled()
    expect(fetch).toHaveBeenLastCalledWith('/api/push/subscription', expect.objectContaining({ method: 'PUT' }))
  })

  it('upserts an existing local subscription without unsubscribing it', async () => {
    const subscription = { endpoint: 'https://push.example/current', unsubscribe: vi.fn() }
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue(subscription), subscribe: vi.fn() } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('Notification', { requestPermission: vi.fn().mockResolvedValue('granted') })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ publicKey: 'YQ', enabled: true }))).mockResolvedValueOnce(new Response(null, { status: 204 })))
    await enablePush(registration)
    expect(registration.pushManager.subscribe).not.toHaveBeenCalled()
    expect(subscription.unsubscribe).not.toHaveBeenCalled()
  })

  it('surfaces denied permission', async () => {
    vi.stubGlobal('Notification', { requestPermission: vi.fn().mockResolvedValue('denied') })
    await expect(enablePush({} as ServiceWorkerRegistration)).rejects.toThrow('permission denied')
  })

  it('surfaces config body and correlation reference in initial visible feedback', async () => {
    vi.stubGlobal('Notification', { permission: 'default' })
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(new Response('authentication required', { status: 401, headers: { 'X-Request-Id': 'req-123' } }))))
    await expect(pushConfig()).rejects.toThrow('Push configuration failed (401): authentication required [ref: req-123]')
    const registration = { pushManager: { getSubscription: vi.fn() } } as unknown as ServiceWorkerRegistration
    await expect(initialPushFeedback(registration)).resolves.toEqual({ state: 'error', message: 'Push configuration failed (401): authentication required [ref: req-123]' })
  })

  it('surfaces save body and fallback correlation reference then rolls browser subscription back', async () => {
    const unsubscribe = vi.fn()
    const subscription = { unsubscribe }
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue(null), subscribe: vi.fn().mockResolvedValue(subscription) } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('Notification', { requestPermission: vi.fn().mockResolvedValue('granted') })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ publicKey: 'YQ', enabled: false }))).mockResolvedValueOnce(new Response('disk failed', { status: 500, headers: { 'X-Correlation-Id': 'corr-9' } })))
    await expect(enablePush(registration)).rejects.toThrow('Saving push subscription failed (500): disk failed [ref: corr-9]')
    expect(unsubscribe).toHaveBeenCalled()
  })

  it('surfaces delete body and correlation without unsubscribing browser', async () => {
    const unsubscribe = vi.fn()
    const subscription = { endpoint: 'https://push.example/current', unsubscribe }
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue(subscription) } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('disk failed', { status: 500, headers: { 'X-Request-Id': 'req-500' } })))
    await expect(disablePush(registration)).rejects.toThrow('Disabling push failed (500): disk failed [ref: req-500]')
    expect(unsubscribe).not.toHaveBeenCalled()
  })

  it('deletes endpoint-specific server subscription before browser subscription', async () => {
    const unsubscribe = vi.fn()
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue({ endpoint: 'https://push.example/current', unsubscribe }) } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    await disablePush(registration)
    expect(fetch).toHaveBeenCalledWith('/api/push/subscription', expect.objectContaining({ method: 'DELETE', body: '{"endpoint":"https://push.example/current"}' }))
    expect(unsubscribe).toHaveBeenCalled()
  })

  it('uses local subscription for initial enabled state', async () => {
    vi.stubGlobal('Notification', { permission: 'granted' })
    const getSubscription = vi.fn().mockResolvedValue({ endpoint: 'https://push.example/current' })
    const registration = { pushManager: { getSubscription } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ publicKey: 'YQ', enabled: true }))))
    await expect(initialPushFeedback(registration)).resolves.toEqual({ state: 'enabled', message: '' })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ enabled: false }))))
    await expect(initialPushFeedback(registration)).resolves.toEqual({ state: 'enabled', message: 'Push is not configured on server' })
    expect(getSubscription).toHaveBeenCalledTimes(2)
  })

  it('does nothing when browser has no local subscription', async () => {
    const registration = { pushManager: { getSubscription: vi.fn().mockResolvedValue(null) } } as unknown as ServiceWorkerRegistration
    vi.stubGlobal('fetch', vi.fn())
    await disablePush(registration)
    expect(fetch).not.toHaveBeenCalled()
  })

  it('turns subscription lookup rejection into visible error feedback', async () => {
    vi.stubGlobal('Notification', { permission: 'granted' })
    const registration = { pushManager: { getSubscription: vi.fn().mockRejectedValue(new Error('browser store unavailable')) } } as unknown as ServiceWorkerRegistration
    await expect(togglePush(registration)).resolves.toEqual({ state: 'error', message: 'browser store unavailable' })
  })

  it('posts strict pane-only focus request and surfaces stale pane error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(new Response(null, { status: 204 })).mockResolvedValueOnce(new Response('pane no longer exists', { status: 404 })))
    await focusPane('w1:p2')
    expect(fetch).toHaveBeenCalledWith('/api/push/focus', expect.objectContaining({ method: 'POST', body: '{"pane_id":"w1:p2"}' }))
    await expect(focusPane('stale')).rejects.toThrow('pane no longer exists')
    await expect(focusPane('https://evil.example')).rejects.toThrow('Invalid notification pane target')
  })

  it('consumes query once, removes it, and emits visible focus feedback', async () => {
    const listeners: Record<string, (event: MessageEvent) => void> = {}
    vi.stubGlobal('navigator', { serviceWorker: { addEventListener: (name: string, fn: (event: MessageEvent) => void) => listeners[name] = fn, removeEventListener: vi.fn() } })
    vi.stubGlobal('window', { location: { href: 'https://app.example/?pane_id=w1%3Ap2', origin: 'https://app.example' } })
    vi.stubGlobal('history', { state: null, replaceState: vi.fn() })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    const feedback: string[] = []
    consumePaneFocus((value) => feedback.push(`${value.state}:${value.message}`))
    await vi.waitFor(() => expect(feedback).toEqual(['pending:Opening notification pane…', 'success:Notification pane opened']))
    expect(history.replaceState).toHaveBeenCalled()
    listeners.message(new MessageEvent('message', { data: { type: 'herdr-pane-focus', pane_id: 'other' }, origin: 'https://app.example' }))
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('bounds response bodies before rendering', async () => {
    const error = await responseError(new Response('x'.repeat(3000), { status: 500 }), 'Failed')
    expect(error.message.length).toBeLessThan(2100)
  })
})
