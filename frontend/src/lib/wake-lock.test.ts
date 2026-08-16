import { afterEach, describe, expect, test, vi } from 'vitest'
import { holdPWAScreenAwake } from './wake-lock'

afterEach(() => vi.unstubAllGlobals())

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('holdPWAScreenAwake', () => {
  test('holds standalone PWA awake, reacquires when visible, and releases on cleanup', async () => {
    const documentEvents = new EventTarget()
    const fakeDocument = {
      visibilityState: 'visible',
      addEventListener: documentEvents.addEventListener.bind(documentEvents),
      removeEventListener: documentEvents.removeEventListener.bind(documentEvents),
    }
    const locks = Array.from({ length: 2 }, () => {
      const events = new EventTarget()
      return {
        released: false,
        addEventListener: events.addEventListener.bind(events),
        release: vi.fn(async function (this: { released: boolean }) {
          this.released = true
          events.dispatchEvent(new Event('release'))
        }),
      }
    })
    const request = vi.fn().mockResolvedValueOnce(locks[0]).mockResolvedValueOnce(locks[1])
    vi.stubGlobal('document', fakeDocument)
    vi.stubGlobal('navigator', { wakeLock: { request } })
    vi.stubGlobal('matchMedia', () => ({ matches: true }))
    const onStatus = vi.fn()

    const cleanup = holdPWAScreenAwake(onStatus)
    await flush()
    expect(request).toHaveBeenCalledTimes(1)
    expect(onStatus).toHaveBeenLastCalledWith({ state: 'active', message: 'Screen stays awake' })

    await locks[0].release()
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    await flush()
    expect(request).toHaveBeenCalledTimes(2)

    cleanup()
    await flush()
    expect(locks[1].released).toBe(true)
  })

  test('reacquires when the old lock releases after visibility already returned', async () => {
    const documentEvents = new EventTarget()
    const fakeDocument = {
      visibilityState: 'visible',
      addEventListener: documentEvents.addEventListener.bind(documentEvents),
      removeEventListener: documentEvents.removeEventListener.bind(documentEvents),
    }
    const lockEvents = new EventTarget()
    const lock = {
      addEventListener: lockEvents.addEventListener.bind(lockEvents),
      release: vi.fn(),
    }
    const request = vi.fn().mockResolvedValue(lock)
    vi.stubGlobal('document', fakeDocument)
    vi.stubGlobal('navigator', { wakeLock: { request } })
    vi.stubGlobal('matchMedia', () => ({ matches: true }))

    holdPWAScreenAwake(vi.fn())
    await flush()
    documentEvents.dispatchEvent(new Event('visibilitychange'))
    lockEvents.dispatchEvent(new Event('release'))
    await flush()

    expect(request).toHaveBeenCalledTimes(2)
  })

  test('does nothing outside standalone display mode', () => {
    const request = vi.fn()
    vi.stubGlobal('navigator', { wakeLock: { request } })
    vi.stubGlobal('matchMedia', () => ({ matches: false }))

    holdPWAScreenAwake(vi.fn())()
    expect(request).not.toHaveBeenCalled()
  })

  test('reports missing wake-lock support in standalone mode', () => {
    vi.stubGlobal('navigator', {})
    vi.stubGlobal('matchMedia', () => ({ matches: true }))
    const onStatus = vi.fn()

    holdPWAScreenAwake(onStatus)()
    expect(onStatus).toHaveBeenCalledWith({ state: 'unavailable', message: 'Screen Wake Lock unsupported' })
  })
})
