# Build artifact promptbox

Type: task
Status: open
Blocked by: 03, 04

## Question

Implement the artifact promptbox on top of the rendering MVP: multipart upload endpoint →
write `/tmp/<ns>/<file>` blob → inject the path into the focused pane per the behavior agreed
in ticket 03.

Done when: from the browser, upload a file and see its `/tmp/<ns>/<file>` path land in the
focused pane's prompt, ready for the agent to reference.

Deliverable: working upload + inject, end to end.
