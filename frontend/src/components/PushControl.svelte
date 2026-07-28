<script lang="ts">
  import { onMount } from 'svelte'
  import { consumePaneFocus, initialPushFeedback, registerPushWorker, supportsPush, togglePush, type PaneFocusFeedback, type PushState } from '../lib/push'

  let registration = $state<ServiceWorkerRegistration | null>(null)
  let pushState = $state<PushState>('idle')
  let message = $state('')
  let paneFocus = $state<PaneFocusFeedback | null>(null)

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

<div class="push-control">
  <button type="button" onclick={toggle} disabled={!registration || pushState === 'pending' || pushState === 'unsupported' || pushState === 'denied'} aria-pressed={pushState === 'enabled'}>
    {pushState === 'pending' ? 'Push…' : pushState === 'enabled' ? 'Push on' : 'Push off'}
  </button>
  {#if paneFocus}<span role="status" class:error={paneFocus.state === 'error'}>{paneFocus.message}</span>{/if}
  {#if message}<span role="status" class:error={pushState === 'error' || pushState === 'denied'}>{message}</span>{/if}
</div>

<style>
  .push-control { display: flex; align-items: center; gap: .5rem; min-width: 0; }
  button { min-height: 2rem; border: 1px solid #555; border-radius: .25rem; background: #171717; color: #eee; padding: 0 .5rem; }
  button[aria-pressed='true'] { border-color: #47c982; color: #8cebb5; }
  span { color: #b7b7b7; font-size: .75rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  span.error { color: #ff9d9d; }
</style>
