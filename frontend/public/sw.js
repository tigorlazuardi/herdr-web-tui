const validPaneId = (value) => typeof value === 'string' && /^[A-Za-z0-9:_-]{1,128}$/.test(value)

self.addEventListener('push', (event) => {
  event.waitUntil((async () => {
    const clients = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    if (clients.some((client) => client.visibilityState === 'visible' || client.focused)) return
    const data = event.data?.json()
    if (!data || typeof data.title !== 'string' || typeof data.body !== 'string' || !validPaneId(data.pane_id)) return
    await self.registration.showNotification(data.title, {
      body: data.body,
      data: { pane_id: data.pane_id },
      tag: `herdr-agent-${typeof data.state === 'string' ? data.state : 'update'}`,
    })
  })())
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil((async () => {
    const paneId = event.notification.data?.pane_id
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true })
    const existing = windows.find((client) => new URL(client.url).origin === self.location.origin)
    if (existing) {
      if (validPaneId(paneId)) existing.postMessage({ type: 'herdr-pane-focus', pane_id: paneId })
      return existing.focus()
    }
    const url = new URL('/', self.location.origin)
    if (validPaneId(paneId)) url.searchParams.set('pane_id', paneId)
    return self.clients.openWindow(url.href)
  })())
})
