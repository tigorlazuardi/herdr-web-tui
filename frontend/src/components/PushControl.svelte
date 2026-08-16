<script lang="ts">
  import { onMount } from 'svelte'
  import { consumePaneFocus, initialPushFeedback, isTransientFeedback, registerPushWorker, supportsPush, togglePush, type PaneFocusFeedback, type PushState } from '../lib/push'

  let registration = $state<ServiceWorkerRegistration | null>(null)
  let pushState = $state<PushState>('idle')
  let message = $state('')
  let paneFocus = $state<PaneFocusFeedback | null>(null)
  const transientMessage = $derived(message && isTransientFeedback(pushState) ? message : '')

  $effect(() => {
    const clearMessage = message && isTransientFeedback(pushState)
    const clearPaneFocus = paneFocus && isTransientFeedback(paneFocus.state)
    if (!clearMessage && !clearPaneFocus) return
    const timeout = setTimeout(() => {
      if (clearMessage) message = ''
      if (clearPaneFocus) paneFocus = null
    }, 4000)
    return () => clearTimeout(timeout)
  })

  /** Mount owns worker registration plus initial server state; every failure becomes visible. */
  onMount(() => {
    const stopFocus = consumePaneFocus((feedback) => (paneFocus = feedback))
    if (!supportsPush()) { pushState = 'unsupported'; message = 'Background notifications unsupported'; return stopFocus }
    void (async () => {
      try {
        registration = await registerPushWorker()
        if (!registration) throw new Error('Push worker registration unavailable')
        const feedback = await initialPushFeedback(registration)
        pushState = feedback.state; message = feedback.message
      } catch (error) { pushState = 'error'; message = error instanceof Error ? error.message : String(error) }
    })()
    return stopFocus
  })

  /** Toggle keeps lookup inside state machine so browser API rejection cannot escape visible feedback. */
  async function toggle() {
    if (!registration) return
    pushState = 'pending'; message = 'Checking push status…'
    const feedback = await togglePush(registration)
    pushState = feedback.state; message = feedback.message
  }
</script>

<div class="push-section">
  <div class="push-control">
    <span>Background notifications</span>
    <button onclick={toggle} disabled={!registration || pushState === 'pending' || pushState === 'unsupported' || pushState === 'denied'} aria-pressed={pushState === 'enabled'}>
      {pushState === 'pending' ? 'Push…' : pushState === 'enabled' ? 'Push on' : 'Push off'}
    </button>
  </div>
  {#if message && !transientMessage}<p class:error={pushState === 'error' || pushState === 'denied'}>{message}</p>{/if}
</div>
{#if paneFocus || transientMessage}
  <div class="toast-stack" aria-live="polite">
    {#if paneFocus}<span class:error={paneFocus.state === 'error'}>{paneFocus.message}</span>{/if}
    {#if transientMessage}<span class:error={pushState === 'error' || pushState === 'denied'}>{transientMessage}</span>{/if}
  </div>
{/if}

<style>
  .push-section { display: grid; gap: 0.5rem; }
  .push-control { display: flex; align-items: center; justify-content: space-between; gap: 1rem; min-width: 0; color: #a8a29e; font: 0.85rem/1.4 system-ui, sans-serif; }
  p { margin: 0; color: #a8a29e; font: 0.75rem/1.4 system-ui, sans-serif; }
  p.error { color: #fca5a5; }
  button { min-height: 2rem; border: 1px solid #57534e; border-radius: 0.5rem; background: #171717; color: #eee; padding: 0 0.75rem; }
  button[aria-pressed='true'] { border-color: #47c982; color: #8cebb5; }
  .toast-stack { position: fixed; z-index: 100; inset: auto 1rem 5rem; display: grid; justify-items: center; gap: 0.5rem; pointer-events: none; }
  .toast-stack span { max-width: min(28rem, calc(100vw - 2rem)); padding: 0.75rem 1rem; border: 1px solid #57534e; border-radius: 0.75rem; background: #292524; color: #f5f5f4; box-shadow: 0 0.75rem 1.5rem rgba(0, 0, 0, 0.4); font: 0.85rem/1.4 system-ui, sans-serif; }
  .toast-stack span.error { border-color: #ef4444; color: #fecaca; }
</style>
