<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import {
    ACCESSORY_CHIPS,
    ALTERNATES,
    mergeAccessoryOrder,
    moveAccessoryChip,
    type AccessoryChip,
    type StickyModifiers,
  } from '../lib/keybar'
  import type { TerminalBridge } from '../lib/terminal'

  let { bridge, sticky }: { bridge: TerminalBridge; sticky: StickyModifiers } = $props()

  const STORAGE_KEY = 'herdr.accessoryRail.order'
  const chipsById = new Map(ACCESSORY_CHIPS.map((chip) => [chip.id, chip]))

  let mods = $state({ ctrl: false, alt: false, fn: false })
  let expanded = $state(false)
  let order = $state(mergeAccessoryOrder(loadOrder()))
  let draggingId = $state<string | null>(null)
  let suppressClick = false
  let pointerStart: { id: string; x: number; y: number } | null = null

  onMount(() => {
    mods = sticky.state
    return sticky.subscribe((s) => (mods = s))
  })

  function loadOrder(): string[] {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      return raw ? JSON.parse(raw) : []
    } catch {
      return []
    }
  }

  function saveOrder(next: string[]) {
    order = next
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  }

  function orderedChips(row: AccessoryChip['row']) {
    return order.map((id) => chipsById.get(id)).filter((chip) => chip?.row === row) as AccessoryChip[]
  }

  function press(chip: AccessoryChip) {
    if (suppressClick) {
      suppressClick = false
      return
    }
    if (chip.kind === 'modifier') {
      sticky.toggle(chip.key as 'ctrl' | 'alt' | 'fn')
      if (sticky.state[chip.key as 'ctrl' | 'alt' | 'fn']) bridge.openKeyboard()
      return
    }
    bridge.sendInput(chip.kind === 'text' ? chip.key : sticky.consume(chip.key))
  }

  // ponytail: reorder only, not drag-to-pane. Add drop-zone actions once reorder proves useful.
  function startDrag(event: PointerEvent, chip: AccessoryChip) {
    pointerStart = { id: chip.id, x: event.clientX, y: event.clientY }
  }

  function moveDrag(event: PointerEvent) {
    if (!pointerStart) return
    const dx = Math.abs(event.clientX - pointerStart.x)
    const dy = Math.abs(event.clientY - pointerStart.y)
    if (dx + dy < 8) return
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
    draggingId = pointerStart.id
  }

  function endDrag(event: PointerEvent) {
    if (!draggingId) {
      pointerStart = null
      return
    }

    const target = document.elementFromPoint(event.clientX, event.clientY) as HTMLElement | null
    const targetId = target?.closest<HTMLElement>('[data-chip-id]')?.dataset.chipId
    if (targetId) saveOrder(moveAccessoryChip(order, draggingId, targetId))
    suppressClick = true
    draggingId = null
    pointerStart = null
  }

  function cancelDrag() {
    draggingId = null
    pointerStart = null
  }

  const LONG_PRESS_MS = 500
  let pressTimer: ReturnType<typeof setTimeout> | null = null
  let longPressFired = false

  function startPress(event: PointerEvent, chip: AccessoryChip) {
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
    startDrag(event, chip)
    longPressFired = false
    const alt = ALTERNATES[chip.key]
    if (!alt) return
    pressTimer = setTimeout(() => {
      longPressFired = true
      bridge.sendInput(sticky.consume(alt))
    }, LONG_PRESS_MS)
  }

  function endPress(event: PointerEvent, chip: AccessoryChip) {
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
    if (draggingId) {
      endDrag(event)
      return
    }
    pointerStart = null
    if (!longPressFired) press(chip)
  }

  function cancelPress() {
    if (pressTimer) {
      clearTimeout(pressTimer)
      pressTimer = null
    }
    cancelDrag()
  }

  onDestroy(cancelPress)
</script>

<div class="keybar" role="toolbar" aria-label="Accessory key rail">
  <div class="row primary">
    {#each orderedChips('primary') as chip (chip.id)}
      <button
        type="button"
        class:mod={chip.kind === 'modifier'}
        class:dragging={draggingId === chip.id}
        aria-pressed={chip.kind === 'modifier'
          ? mods[chip.key as 'ctrl' | 'alt' | 'fn']
          : undefined}
        data-chip-id={chip.id}
        onpointerdown={(event) => startPress(event, chip)}
        onpointermove={moveDrag}
        onpointerup={(event) => endPress(event, chip)}
        onpointercancel={cancelPress}
      >
        {chip.label}
      </button>
    {/each}
    <button
      type="button"
      class="toggle"
      aria-label={expanded ? 'Hide extra keys' : 'Show extra keys'}
      aria-expanded={expanded}
      onclick={() => (expanded = !expanded)}
    >
      {expanded ? '∧' : '∨'}
    </button>
  </div>
  {#if expanded}
    <div class="row extra">
      {#each orderedChips('extra') as chip (chip.id)}
        <button
          type="button"
          class:dragging={draggingId === chip.id}
          data-chip-id={chip.id}
          title={ALTERNATES[chip.key] ? `long-press for ${ALTERNATES[chip.key]}` : undefined}
          onpointerdown={(event) => startPress(event, chip)}
          onpointermove={moveDrag}
          onpointerup={(event) => endPress(event, chip)}
          onpointercancel={cancelPress}
        >
          {chip.label}
        </button>
      {/each}
    </div>
  {/if}
</div>

<style>
  .keybar {
    flex: none;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.375rem;
    padding-bottom: max(0.375rem, env(safe-area-inset-bottom));
    background: #1c1917;
    border-top: 1px solid #292524;
    touch-action: none;
  }

  .row {
    display: flex;
    gap: 0.25rem;
    overflow-x: auto;
    scrollbar-width: thin;
  }

  .row button {
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
    touch-action: none;
  }

  .row button:active {
    background: #44403c;
  }

  .row button.mod[aria-pressed='true'] {
    background: #f59e0b;
    color: #1c1917;
  }

  .row button.dragging {
    opacity: 0.45;
    pointer-events: none;
  }

  .row button.toggle {
    position: sticky;
    right: 0;
    min-width: 2.5rem;
    background: #44403c;
  }
</style>
