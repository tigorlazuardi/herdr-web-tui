/**
 * Orchestrates one promptbox send: serialize() the segment list, hand it to
 * an UploadClient, and return the result. Split out of Promptbox.svelte so
 * the all-or-nothing send flow (and its error handling) is testable against
 * a faked UploadClient without any DOM/contenteditable machinery — the
 * design doc's "transport behind an interface, faked in component tests"
 * seam, minus the higher-effort DOM parts (design doc: pill-editor DOM
 * behaviour is secondary/higher-effort, left to component/e2e tooling).
 */
import { serialize, type Segment } from './segments'
import type { SendResult, SubmitKey, UploadClient } from './transport'

/**
 * sendSegments never throws: every UploadClient failure (network, 4xx, 5xx)
 * already comes back as a SendFailure, so the caller (Promptbox.svelte) has
 * exactly one branch to handle — show the error inline and leave `segments`
 * untouched, since nothing here mutates or clears them. Clearing the editor
 * on success is the caller's job, not this function's, since it is a UI
 * concern.
 */
export function sendSegments(
  client: UploadClient,
  segments: readonly Segment[],
  session: string,
  submitKey: SubmitKey = 'enter',
): Promise<SendResult> {
  return client.send(serialize(segments), session, submitKey)
}
