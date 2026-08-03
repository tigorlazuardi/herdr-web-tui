<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import {
    ACCESSORY_CHIPS,
    ALTERNATES,
    type AccessoryChip,
    type StickyModifiers,
  } from '../lib/keybar'
  import type { TerminalBridge } from '../lib/terminal'

  let { bridge, sticky }: { bridge: TerminalBridge; sticky: StickyModifiers } = $props()

  let mods = $state({ ctrl: false, alt: false, fn: false })
  let suppressClick = false
  let pressTimer: ReturnType<typeof setTimeout> | null = null
  let pointerStart: { x: number; y: number } | null = null

  onMount(() => {
    mods = sticky.state
    return sticky.subscribe((state) => (mods = state))
  })

  function press(chip: AccessoryChip) {
    if (suppressClick) {
      suppressClick = false
    } else if (chip.kind === 'modifier') {
      sticky.toggle(chip.key as 'ctrl' | 'alt' | 'fn')
    } else {
      bridge.sendInput(chip.kind === 'text' ? chip.key : sticky.consume(chip.key))
    }
    bridge.openKeyboard()
  }

  // ponytail: browser owns horizontal swipe/scroll. Only punctuation keeps long-press alternate.
  function startAlternate(event: PointerEvent, chip: AccessoryChip) {
    const alternate = ALTERNATES[chip.key]
    if (!alternate) return
    pointerStart = { x: event.clientX, y: event.clientY }
    pressTimer = setTimeout(() => {
      suppressClick = true
      bridge.sendInput(sticky.consume(alternate))
    }, 500)
  }

  function movePointer(event: PointerEvent) {
    if (!pointerStart) return
    if (Math.abs(event.clientX - pointerStart.x) + Math.abs(event.clientY - pointerStart.y) > 8) {
      cancelAlternate()
    }
  }

  function cancelAlternate() {
    if (pressTimer) clearTimeout(pressTimer)
    pressTimer = null
    pointerStart = null
  }

  onDestroy(cancelAlternate)
</script>

<div class="keybar" role="toolbar" aria-label="Accessory key rail">
  {#each ACCESSORY_CHIPS as chip (chip.id)}
    <!-- Keep xterm focused: mobile IME closes when a rail button takes focus before click. -->
    <button
      type="button"
      class:mod={chip.kind === 'modifier'}
      aria-pressed={chip.kind === 'modifier'
        ? mods[chip.key as 'ctrl' | 'alt' | 'fn']
        : undefined}
      title={ALTERNATES[chip.key] ? `long-press for ${ALTERNATES[chip.key]}` : undefined}
      onmousedown={(event) => event.preventDefault()}
      onpointerdown={(event) => startAlternate(event, chip)}
      onpointermove={movePointer}
      onpointerup={cancelAlternate}
      onpointercancel={cancelAlternate}
      onclick={() => press(chip)}
    >
      {chip.label}
    </button>
  {/each}
</div>

<style>
  .keybar {
    flex: none;
    display: flex;
    gap: 0.25rem;
    padding: 0.375rem;
    padding-bottom: max(0.375rem, env(safe-area-inset-bottom));
    overflow-x: auto;
    overscroll-behavior-x: contain;
    scrollbar-width: thin;
    touch-action: pan-x;
    -webkit-overflow-scrolling: touch;
    background: #1c1917;
    border-top: 1px solid #292524;
  }

  .keybar button {
    flex: 0 0 auto;
    min-width: 2.75rem;
    height: 2.5rem;
    padding: 0 0.55rem;
    border: none;
    border-radius: 0.5rem;
    background: #292524;
    color: #e7e5e4;
    font: 600 0.8rem/1 system-ui, sans-serif;
    white-space: nowrap;
    user-select: none;
  }

  .keybar button:active {
    background: #44403c;
  }

  .keybar button.mod[aria-pressed='true'] {
    background: #f59e0b;
    color: #1c1917;
  }
</style>
