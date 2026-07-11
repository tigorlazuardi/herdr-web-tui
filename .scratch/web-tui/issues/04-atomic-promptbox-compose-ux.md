# Design the atomic promptbox compose + UX

Type: prototype
Status: resolved
Blocked by: 03 (resolved)

## Question

How does the web promptbox compose one atomic bundle and inject+submit it as a single message?

### Compose model — positional template (decided)

The promptbox is a **parametrized template**: attachments are **positional placeholders**
embedded in the text at the point the user wants them, resolved to their uploaded
`/tmp` path on send. This makes it work for **any pane, not just an agent chatbox** —
e.g. a shell command with a file argument.

```
user types:   imgcat [File 1]
injected:     imgcat /tmp/<prefix>-<uid>/<uuid>.ext
```

### Editor model (decided) — segment list, not string parsing

The promptbox is an **ordered list of segments**: text runs + file **pills**. Do NOT
parse a `[File N]` string — the pill *is* the reference.

- Pill = an **atomic chip** (`contenteditable=false` inside a contenteditable host, or the stack's equivalent) so **backspace deletes the whole pill natively** — like Claude Code's attachment chips. No custom delete logic.
- Attach a file → pill inserted at the caret immediately (shows filename + thumbnail/mime badge, per ticket 06). Files stay **local** until send (flexible, cheap).
- On send → the client serializes segments (in order) into a template + ordered files and posts **one atomic multipart request** (see ticket 07). The **daemon** saves files, resolves each marker to its `/tmp` path, and injects the whole line via `herdr pane run` (text + Enter atomic) into the focused pane. (Verified live: focus is server-wide, so "focused pane" = whatever the browser user last navigated to — the picker is genuinely unnecessary.) **Ordering is inherent in pill position**; path resolution is server-side (client never sees `/tmp` paths); no numbering, no "placeholder without a file" case. `[File N]` survives only as a fallback text label.
- **Atomic**: any file-save or inject failure → nothing is injected.

### Clipboard paste (wishlist / enhancement)

On `paste`, read `clipboardData.items`; a file item (e.g. a screenshot) → insert a
pill instead of pasting text. Guess type from the item's MIME first; magic-byte sniff
only as fallback. This reproduces Herdr's remote clipboard-image-paste — but in the
browser, closing the SSH-first gap directly in the promptbox.

Remaining:
- How many attachments per submit (cap?).
- Where the promptbox sits in the web UI relative to the ttyd terminal (side panel? bottom bar?).
- Submit semantics: upload → get path(s) → compose → `pane.send_input` text → send enter. Atomic (all-or-nothing) — what happens on upload failure.

Prototype the UI + compose flow. Depends on the inject mechanism locked in ticket 03.

## Answer (design complete)

All compose/UX decisions are locked: positional segment editor with atomic file pills
(native backspace-deletes-pill), clipboard-paste inserts a pill, ordering inherent in
pill position, one atomic multipart request to the daemon (ticket 07) which resolves
paths server-side and injects via `herdr pane run` into the server-wide focused pane
(ticket 03), attachment-list UI with thumbnails / mime badges / lucide
fallback (ticket 06).

Residual = **build-time frontend-design calls, not blocking decisions**:
- Attachment count cap: no hard app cap — the gateway's upload size limit governs (`# ponytail: add a count cap only if it bites`).
- Promptbox placement (bottom bar vs side panel): decide during the build with the `frontend-design` skill; leaning a bottom bar (chat-like).

The actual UI build is execution beyond this map's destination (a spec/design doc).

## Attachment picker / list UI (decided in ticket 06)

Pre-upload, still pure-local (browser File objects) — keep it lightweight. Frontend
work: **load the `frontend-design` skill** before building this.

- **Image** attachment → render a **local thumbnail** preview (object URL, cheap).
- **PDF** → preview if feasible (pdf.js, optional — nice-to-have).
- **Corner badge** = canonical short mime type on the thumbnail: `TXT`, `JPEG`, `PDF`, `PNG`, `ZIP`, `TAR/GZIP`, etc. **Unknown type → no badge.** The badge alone distinguishes types — no per-type border.
- **Fallback** when there is no image preview → a **lucide** icon.

Still open here: exact injected string format, multi-attachment ordering, promptbox placement, atomic-submit failure handling (upload fail).
