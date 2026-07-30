import { describe, expect, it, vi } from 'vitest'
import fs from 'node:fs'
import vm from 'node:vm'

function loadWorker(windows = []) {
  const handlers = {}
  const registration = { showNotification: vi.fn() }
  const clients = { matchAll: vi.fn().mockResolvedValue(windows), openWindow: vi.fn() }
  const self = { addEventListener: (name, handler) => handlers[name] = handler, clients, registration, location: { origin: 'https://app.example' } }
  vm.runInNewContext(fs.readFileSync(new URL('./public/sw.js', import.meta.url), 'utf8'), { self, URL })
  return { handlers, registration, clients }
}
describe('push service worker', () => {
  it('suppresses notification while visible client exists', async () => {
    const { handlers, registration } = loadWorker([{ visibilityState: 'visible' }])
    let promise
    handlers.push({ data: { json: () => ({ title: 'Done', body: 'agent' }) }, waitUntil: (p) => promise = p })
    await promise
    expect(registration.showNotification).not.toHaveBeenCalled()
  })
  it('shows pane-only payload and messages existing same-origin window on click', async () => {
    const window = { visibilityState: 'hidden', url: 'https://app.example/x', postMessage: vi.fn(), focus: vi.fn() }
    const { handlers, registration } = loadWorker([window])
    let promise
    handlers.push({ data: { json: () => ({ title: 'Done', body: 'agent', pane_id: 'w1:p2', url: 'https://evil.example' }) }, waitUntil: (p) => promise = p })
    await promise
    expect(registration.showNotification).toHaveBeenCalledWith('Done', expect.objectContaining({
      data: { pane_id: 'w1:p2' },
      tag: 'herdr-agent-w1:p2-update',
      renotify: true,
      requireInteraction: true,
      silent: false,
      vibrate: [200, 100, 200],
    }))
    handlers.notificationclick({ notification: { close: vi.fn(), data: { pane_id: 'w1:p2', url: 'https://evil.example' } }, waitUntil: (p) => promise = p })
    await promise
    expect(window.postMessage).toHaveBeenCalledWith({ type: 'herdr-pane-focus', pane_id: 'w1:p2' })
    expect(window.focus).toHaveBeenCalled()
  })
  it('opens bounded same-origin cold URL and rejects arbitrary URL payload', async () => {
    const { handlers, clients } = loadWorker()
    let promise
    handlers.notificationclick({ notification: { close: vi.fn(), data: { pane_id: 'w1:p2', url: 'https://evil.example' } }, waitUntil: (p) => promise = p })
    await promise
    expect(clients.openWindow).toHaveBeenCalledWith('https://app.example/?pane_id=w1%3Ap2')
  })
  it('contains no fetch or offline cache handler', () => {
    const source = fs.readFileSync(new URL('./public/sw.js', import.meta.url), 'utf8')
    expect(source).not.toContain("addEventListener('fetch'")
    expect(source).not.toContain('caches.')
  })
})
