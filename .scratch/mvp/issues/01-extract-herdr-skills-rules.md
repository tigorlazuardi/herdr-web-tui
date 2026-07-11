# Extract Herdr skills + rules for this repo

Type: research
Status: open
Blocked by:

## Question

What durable Herdr knowledge from the agent guide + socket API + CLI reference should be
captured as `.pi/rules` and `.pi/skills` in THIS repo, so every future session already knows
how Herdr's concept model and control surface work (session/workspace/tab/pane, socket API,
`pane.read` / `pane.send_input`, event subscriptions, CLI wrappers)?

Deliverable: `.pi/rules/<name>.md` (path-scoped) and/or `.pi/skills/<name>/SKILL.md`
(intent-triggered) distilling the Herdr control surface relevant to building this web-tui.
Sources: https://herdr.dev/agent-guide.md , https://herdr.dev/docs/socket-api/ ,
https://herdr.dev/docs/cli-reference/ , https://raw.githubusercontent.com/ogulcancelik/herdr/master/SKILL.md

Trivia filter: capture only durable concepts (concept model, socket methods, injection
mechanism, gotchas). User already requested this — permission to write `.pi/` granted.
