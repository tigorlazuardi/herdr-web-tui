/**
 * Small client-side error logger. Posts to POST /clientlog
 * (internal/server/send.go's clientlogHandler) so a frontend-only failure
 * (or a /send failure the user saw) lands in the same backend log stream as
 * everything else, tagged with the same correlation ref id the promptbox
 * showed the user — the design doc's "FE and BE errors reconcile in one
 * place". Best-effort: a /clientlog POST failing must never itself surface
 * an error to the user (nothing to retry, nowhere to show it), so failures
 * here are swallowed after one console.error for local dev visibility.
 */
export interface ClientLogReport {
  message: string
  stack?: string
  /** The correlation id shown to the user for the failure being reported, if any. */
  refID?: string
}

export async function reportClientError(report: ClientLogReport): Promise<void> {
  try {
    await fetch('/clientlog', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        message: report.message,
        stack: report.stack,
        url: location.href,
        ref_id: report.refID,
      }),
    })
  } catch (err) {
    console.error('reportClientError: failed to post /clientlog', err)
  }
}
