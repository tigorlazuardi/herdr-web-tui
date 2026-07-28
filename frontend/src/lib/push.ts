export type PushState = 'unsupported' | 'idle' | 'pending' | 'enabled' | 'denied' | 'error'
export type PushFeedback = { state: PushState; message: string }
export type PaneFocusFeedback = { state: 'pending' | 'success' | 'error'; message: string }

const validPaneId = (value: unknown): value is string => typeof value === 'string' && /^[A-Za-z0-9:_-]{1,128}$/.test(value)

export function supportsPush(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

function applicationServerKey(value: string): Uint8Array<ArrayBuffer> {
  const padded = value.padEnd(Math.ceil(value.length / 4) * 4, '=')
  const bytes = Uint8Array.from(atob(padded.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0))
  return bytes as Uint8Array<ArrayBuffer>
}

/** Converts bounded backend response text plus correlation header into visible operator feedback. */
export async function responseError(response: Response, action: string): Promise<Error> {
  const raw = (await response.text()).trim()
  const contentType = response.headers.get('Content-Type')?.toLowerCase() ?? ''
  const body = contentType.includes('text/html') ? 'upstream returned an HTML error' : raw.slice(0, 2048)
  const ref = response.headers.get('X-Request-Id') ?? response.headers.get('X-Correlation-Id')
  return new Error(`${action} (${response.status})${body ? `: ${body}` : ''}${ref ? ` [ref: ${ref}]` : ''}`)
}

/** Registers native worker only where browser Push APIs exist. */
export async function registerPushWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!supportsPush()) return null
  return navigator.serviceWorker.register('/sw.js', { scope: '/' })
}

/** Reads server push capability, preserving backend error detail and correlation reference. */
export async function pushConfig(): Promise<{ publicKey?: string; enabled: boolean }> {
  const response = await fetch('/api/push/config')
  if (!response.ok) throw await responseError(response, 'Push configuration failed')
  return await response.json() as { publicKey?: string; enabled: boolean }
}

/** Upserts current browser subscription, creating and rolling back one only when absent. */
export async function enablePush(registration: ServiceWorkerRegistration): Promise<void> {
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new Error(permission === 'denied' ? 'Notification permission denied' : 'Notification permission not granted')
  const config = await pushConfig()
  if (!config.publicKey) throw new Error('Push is not configured on server')
  const existing = await registration.pushManager.getSubscription()
  const subscription = existing ?? await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: applicationServerKey(config.publicKey) })
  const response = await fetch('/api/push/subscription', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(subscription) })
  if (!response.ok) {
    if (!existing) await subscription.unsubscribe()
    throw await responseError(response, 'Saving push subscription failed')
  }
}

/** Deletes only current browser endpoint before native unsubscribe; absent local subscription is no-op. */
export async function disablePush(registration: ServiceWorkerRegistration): Promise<void> {
  const subscription = await registration.pushManager.getSubscription()
  if (!subscription) return
  const response = await fetch('/api/push/subscription', {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint: subscription.endpoint })
  })
  if (!response.ok) throw await responseError(response, 'Disabling push failed')
  await subscription.unsubscribe()
}

/** Resolves local browser state while config reports server capability only. */
export async function initialPushFeedback(registration: ServiceWorkerRegistration): Promise<PushFeedback> {
  try {
    const [config, subscription] = await Promise.all([pushConfig(), registration.pushManager.getSubscription()])
    if (subscription) return { state: 'enabled', message: config.enabled ? '' : 'Push is not configured on server' }
    if (!config.enabled) return { state: 'idle', message: 'Push is not configured on server' }
    if (Notification.permission === 'denied') return { state: 'denied', message: 'Notification permission denied' }
    return { state: 'idle', message: '' }
  } catch (error) {
    return { state: 'error', message: error instanceof Error ? error.message : String(error) }
  }
}

/** Focuses one strictly validated pane through authenticated same-origin API. */
export async function focusPane(paneId: string): Promise<void> {
  if (!validPaneId(paneId)) throw new Error('Invalid notification pane target')
  const response = await fetch('/api/push/focus', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pane_id: paneId })
  })
  if (!response.ok) throw await responseError(response, 'Focusing notification pane failed')
}

/** Consumes query or one bounded worker message once; caller receives visible lifecycle feedback. */
export function consumePaneFocus(onFeedback: (feedback: PaneFocusFeedback) => void): () => void {
  let consumed = false
  const consume = async (paneId: unknown) => {
    if (consumed || !validPaneId(paneId)) return
    consumed = true
    onFeedback({ state: 'pending', message: 'Opening notification pane…' })
    try {
      await focusPane(paneId)
      onFeedback({ state: 'success', message: 'Notification pane opened' })
    } catch (error) {
      onFeedback({ state: 'error', message: error instanceof Error ? error.message : String(error) })
    }
  }
  const url = new URL(window.location.href)
  const queryPaneId = url.searchParams.get('pane_id')
  if (queryPaneId !== null) {
    url.searchParams.delete('pane_id')
    history.replaceState(history.state, '', `${url.pathname}${url.search}${url.hash}`)
    void consume(queryPaneId)
  }
  const message = (event: MessageEvent) => {
    if (event.origin !== window.location.origin && event.origin !== '') return
    const data = event.data as { type?: unknown; pane_id?: unknown } | null
    if (data?.type === 'herdr-pane-focus') void consume(data.pane_id)
  }
  navigator.serviceWorker?.addEventListener('message', message)
  return () => navigator.serviceWorker?.removeEventListener('message', message)
}

/** Owns browser lookup and mutation failure conversion used by component's visible state. */
export async function togglePush(registration: ServiceWorkerRegistration): Promise<PushFeedback> {
  try {
    const subscription = await registration.pushManager.getSubscription()
    if (subscription) {
      await disablePush(registration)
      return { state: 'idle', message: 'Background notifications disabled' }
    }
    await enablePush(registration)
    return { state: 'enabled', message: 'Background notifications enabled' }
  } catch (error) {
    return {
      state: Notification.permission === 'denied' ? 'denied' : 'error',
      message: error instanceof Error ? error.message : String(error)
    }
  }
}
