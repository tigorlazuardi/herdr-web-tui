# Decide artifact type scope + agent consumption

Type: grilling
Status: resolved
Blocked by: —

## Question

- Which artifact types are allowed — images only, or any file (pdf, txt, csv, ...)?
- How does the target agent (claude / codex / others) consume a dropped path — does it just read the file from the path, or does any type need special handling (e.g. images vs text)?
- Is per-agent handling in scope, or is the contract strictly "drop the path, agent reads it" and anything else is the agent's problem?

## Answer

- **Any file type** may be uploaded. No allow-list.
- Consumption is **strictly drop-path**: the app injects the `/tmp/...` path; the agent reads the file itself. No per-type handling server-side; anything else is the agent's problem.
- The pre-upload attachment-list gets rich UI treatment (thumbnails / mime badges / preview) — that is UX and moved to ticket 04, not scope. See 04's "Attachment picker" section.
