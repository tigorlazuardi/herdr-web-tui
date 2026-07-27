export function holdPWAScreenAwake(onUnavailable: (unavailable: boolean) => void): () => void {
  if (!matchMedia('(display-mode: standalone)').matches) return () => {}
  if (!('wakeLock' in navigator)) {
    onUnavailable(true)
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
      onUnavailable(false)
      next.addEventListener('release', () => lock === next && (lock = undefined), { once: true })
    } catch {
      onUnavailable(true)
    } finally {
      pending = false
    }
  }
  const onVisibilityChange = () => void acquire()

  document.addEventListener('visibilitychange', onVisibilityChange)
  void acquire()

  return () => {
    stopped = true
    document.removeEventListener('visibilitychange', onVisibilityChange)
    void lock?.release()
  }
}
