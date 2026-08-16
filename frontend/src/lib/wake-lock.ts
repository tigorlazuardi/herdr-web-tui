export interface WakeLockStatus {
  readonly state: 'active' | 'inactive' | 'unavailable'
  readonly message: string
}

export function holdPWAScreenAwake(onStatus: (status: WakeLockStatus) => void): () => void {
  if (!matchMedia('(display-mode: standalone)').matches) {
    onStatus({ state: 'inactive', message: 'Available in installed PWA only' })
    return () => {}
  }
  if (!('wakeLock' in navigator)) {
    onStatus({ state: 'unavailable', message: 'Screen Wake Lock unsupported' })
    return () => {}
  }

  let lock: WakeLockSentinel | undefined
  let pending = false
  let stopped = false

  const acquire = async () => {
    if (stopped || pending || lock || document.visibilityState !== 'visible') return
    pending = true
    try {
      const next = await navigator.wakeLock.request('screen')
      if (stopped) {
        await next.release()
        return
      }
      lock = next
      onStatus({ state: 'active', message: 'Screen stays awake' })
      next.addEventListener('release', () => {
        if (lock !== next) return
        lock = undefined
        onStatus({ state: 'inactive', message: 'Wake lock released; reacquiring…' })
        queueMicrotask(() => void acquire())
      }, { once: true })
    } catch (error) {
      onStatus({
        state: 'unavailable',
        message: `Wake lock failed: ${error instanceof Error ? error.message : String(error)}`,
      })
    } finally {
      pending = false
    }
  }
  const onVisibilityChange = () => {
    if (document.visibilityState !== 'visible') {
      onStatus({ state: 'inactive', message: 'Wake lock paused while app is hidden' })
      return
    }
    void acquire()
  }

  document.addEventListener('visibilitychange', onVisibilityChange)
  void acquire()

  return () => {
    stopped = true
    document.removeEventListener('visibilitychange', onVisibilityChange)
    void lock?.release()
  }
}
