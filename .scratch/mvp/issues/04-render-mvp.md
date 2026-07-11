# Render MVP: browser attaches Herdr session over http+ws

Type: task
Status: open
Blocked by: 02

## Question

Build the core: a co-located web server that lets one browser attach to one Herdr session and
see it rendered live over HTTP + WebSocket, using the base app + transport chosen in ticket 02.

Done when: open the page in a browser (behind nginx or direct in dev), see the attached Herdr
TUI, and interact with it (keyboard at minimum; mouse per the fidelity decided in fog when it
graduates). No artifact upload yet — that is ticket 05.

Deliverable: running MVP renderer + how to launch it.
