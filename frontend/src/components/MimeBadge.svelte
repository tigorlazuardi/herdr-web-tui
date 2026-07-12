<script lang="ts">
  /**
   * Corner mime badge for an attachment pill/preview (design doc: "corner
   * mime badge ... unknown -> no badge"). Deliberately renders nothing for
   * an unmapped mime rather than an ugly generic label — see lib/mime.ts's
   * mimeBadge() for the mapping this reads from.
   */
  import { mimeBadge } from '../lib/mime'

  let { mime }: { mime: string } = $props()
  let label = $derived(mimeBadge(mime))
</script>

{#if label}
  <span class="badge">{label}</span>
{/if}

<style>
  .badge {
    position: absolute;
    top: -0.35rem;
    right: -0.35rem;
    padding: 0.05rem 0.35rem;
    border-radius: 999px;
    background: var(--badge-bg, #334155);
    color: var(--badge-fg, #f1f5f9);
    font: 700 0.6rem/1.4 system-ui, sans-serif;
    letter-spacing: 0.02em;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.4);
  }
</style>
