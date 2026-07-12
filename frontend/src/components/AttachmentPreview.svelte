<script lang="ts">
  /**
   * Renders the best-effort preview for one pill's File: an image
   * thumbnail (native <img> over an object URL), a lazily-loaded pdf.js
   * render for a PDF, or a lucide icon fallback for anything else (design
   * doc: "no-preview -> lucide icon"). pdf.js (~1-2 MB with its worker) is
   * imported with a dynamic import() so it never lands in the main bundle
   * for users who attach no PDFs — see the design doc's "lazy-load the
   * heavy bits" stack decision.
   */
  import { onDestroy } from 'svelte'
  import FileIcon from '@lucide/svelte/icons/file'
  import { isImageMime, isPdfMime } from '../lib/mime'

  let { file, mime }: { file: File; mime: string } = $props()

  let objectURL = $state<string | null>(null)
  let pdfCanvas: HTMLCanvasElement | undefined = $state()
  let pdfError = $state<string | null>(null)

  $effect(() => {
    if (isImageMime(mime)) {
      const url = URL.createObjectURL(file)
      objectURL = url
      return () => URL.revokeObjectURL(url)
    }
    objectURL = null
  })

  $effect(() => {
    if (!isPdfMime(mime) || !pdfCanvas) return
    let cancelled = false
    renderPdfThumbnail(file, pdfCanvas).catch((err: unknown) => {
      if (!cancelled) pdfError = err instanceof Error ? err.message : String(err)
    })
    return () => {
      cancelled = true
    }
  })

  async function renderPdfThumbnail(f: File, canvas: HTMLCanvasElement) {
    // Vite's worker-URL suffix gives pdf.js a bundleable worker script
    // instead of relying on its own CDN/global-lookup default, which would
    // otherwise silently break once this app is served from somewhere
    // other than pdf.js's expected path.
    const [pdfjs, workerURL] = await Promise.all([
      import('pdfjs-dist'),
      import('pdfjs-dist/build/pdf.worker.mjs?url'),
    ])
    pdfjs.GlobalWorkerOptions.workerSrc = workerURL.default

    const buf = await f.arrayBuffer()
    const doc = await pdfjs.getDocument({ data: buf }).promise
    const page = await doc.getPage(1)
    const viewport = page.getViewport({ scale: 0.5 })
    canvas.width = viewport.width
    canvas.height = viewport.height
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('canvas 2d context unavailable')
    await page.render({ canvasContext: ctx, viewport, canvas }).promise
  }

  onDestroy(() => {
    if (objectURL) URL.revokeObjectURL(objectURL)
  })
</script>

<div class="preview">
  {#if isImageMime(mime) && objectURL}
    <img src={objectURL} alt={file.name} />
  {:else if isPdfMime(mime)}
    <canvas bind:this={pdfCanvas} class:hidden={!!pdfError}></canvas>
    {#if pdfError}
      <FileIcon size={28} aria-hidden="true" />
    {/if}
  {:else}
    <FileIcon size={28} aria-hidden="true" />
  {/if}
</div>

<style>
  .preview {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    color: var(--muted-fg, #94a3b8);
  }

  img,
  canvas {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .hidden {
    display: none;
  }
</style>
