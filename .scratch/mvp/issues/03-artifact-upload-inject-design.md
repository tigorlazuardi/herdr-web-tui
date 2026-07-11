# Design artifact upload + prompt injection

Type: prototype
Status: open
Blocked by: 02

## Question

How should the artifact promptbox look and behave? Proposed flow (from the user, to be
prototyped and refined):

- A dedicated promptbox in the web UI that accepts a file (drag/drop or picker).
- File is uploaded full to the server as **multipart form data** (or the recommended
  equivalent), written to `/tmp/<ns>/<file>` as a blob, extension preserved as a type hint.
- The server returns the path; the UI **injects that path into the focused pane's prompt** via
  Herdr send-text / send-keys (mechanism depends on ticket 02's transport choice).

Open sub-questions to resolve here: namespace `<ns>` scheme; does the box just paste the path
or compose a full prompt + path; send Enter or not; single vs multiple files; how the user
knows the upload landed. Make a cheap prototype (stub UI + fake path) to react to.

Deliverable: a linked prototype + the agreed injection behavior.
