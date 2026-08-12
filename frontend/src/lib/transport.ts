/**
 * HTTP transport for the promptbox's atomic send, kept behind an interface
 * (design doc's testing decision: "transport behind an interface, faked in
 * component tests") so Promptbox.svelte's state-management/error-display
 * logic can be unit tested without a real /send server, the same way the
 * Go backend's send.go is tested against a fake HerdrClient rather than a
 * live herdr.
 */
import type { SerializedRequest } from './segments'

/** Successful /send outcome: nothing else to report, the pane already has the text. */
export interface SendSuccess {
  readonly ok: true
}

/**
 * Failed /send outcome. `refID` is the correlation id from the
 * X-Request-Id/X-Correlation-Id response header (internal/correlation) —
 * always present for a request that reached the server, so it can be shown
 * as a "ref: req_xxx" the user can quote when grepping backend logs (design
 * doc: "FE displays the full error message + the ref id"). It is undefined
 * only for failures that never got a server response at all (network
 * error, request aborted before send).
 */
export interface SendFailure {
  readonly ok: false
  readonly error: string
  readonly refID?: string
}

export type SendResult = SendSuccess | SendFailure
export type SubmitKey = 'enter' | 'ctrl-enter' | 'alt-enter'

/**
 * UploadClient is the promptbox's one seam to the network. `send` takes an
 * already-serialized request (see lib/segments.ts's serialize()) and must
 * never throw — every failure (network, 4xx, 5xx, malformed response) is
 * reported through the returned SendResult so callers have one code path
 * for "show the error inline, keep the user's text+pills" regardless of
 * failure kind.
 */
export interface UploadClient {
  send(req: SerializedRequest, session: string, submitKey?: SubmitKey): Promise<SendResult>
}

interface sendResponseBody {
  ok: boolean
  error?: string
}

/**
 * createHttpUploadClient builds the real UploadClient: POSTs {template,
 * files[]} as one multipart/form-data request to /send, matching
 * internal/server/send.go's readParts exactly — a "template" field (JSON),
 * a "session" field, and each file as its own part named by
 * SerializedFile.fieldName (the same name the template's file segments
 * reference). This is the one atomic request the design doc requires: all
 * fields ride in a single fetch, so there is no partial-upload state to
 * reason about on the client.
 */
export function createHttpUploadClient(): UploadClient {
  return {
    async send(req, session, submitKey = 'enter') {
      const form = new FormData()
      form.set('template', JSON.stringify(req.template))
      form.set('session', session)
      form.set('submitKey', submitKey)
      for (const f of req.files) {
        form.set(f.fieldName, f.file, f.file.name)
      }

      let res: Response
      try {
        res = await fetch('/send', { method: 'POST', body: form })
      } catch (err) {
        return { ok: false, error: `network error: ${(err as Error).message}` }
      }

      const refID = res.headers.get('X-Request-Id') ?? undefined

      let body: sendResponseBody | undefined
      try {
        body = (await res.json()) as sendResponseBody
      } catch {
        // Non-JSON body (e.g. a gateway error page) — fall through to the
        // status-based message below rather than failing this parse loudly.
      }

      if (res.ok && body?.ok) {
        return { ok: true }
      }
      const error = body?.error || `send failed: HTTP ${res.status}`
      return { ok: false, error, refID }
    },
  }
}
