# Verify the artifact inject mechanism (socket API)

Type: prototype
Status: resolved
Blocked by: —

## Question

Can we inject text into Herdr's **focused pane** and submit it, from a separate process, without stealing focus or disrupting the ttyd stream?

Verify by prototype against a running Herdr:
- `pane.send_input` (or its CLI wrapper) sends arbitrary text to the focused pane.
- How "focused pane" resolves server-side (the active focused pane) and whether omitting `pane_id` targets it.
- How to send the submit/enter key (send-keys) to make the agent chatbox accept the message.
- Confirm it does not fight the ttyd-rendered client (two attachments to the same session).

Lock the exact call(s) and note constraints.

## Answer

Verified against live Herdr 0.7.3 (systemd `herdr-server.service`), socket `~/.config/herdr/herdr.sock`. Tested in a throwaway `--no-focus` workspace (`w15`, cwd /tmp), then closed it.

**Confirmed working:**
- `herdr pane run <pane_id> '<text>'` — types text + Enter **atomically**. `echo INJECT_TEST_ALPHA` → ran, output present. **This is the inject primitive.**
- `herdr pane send-text <pane_id> '<text>'` then `herdr pane send-keys <pane_id> enter` — also works; a path with slashes + spaces (`/tmp/wf-inject-test/pic.png`) survived intact.
- `herdr pane current` → JSON of the focused pane. `herdr pane read <pane_id> --source visible` → pane content (use **`--source visible`**; `recent`/`recent-unwrapped` returned empty here).
- **All CLI output is JSON** — daemon parses directly, no screen-scraping.

**Constraints / gotchas for the build:**
- **The focused pane can be the caller's own session.** In this run the focused pane was `w14:p2` = this very pi session. In production the browser user drives focus so focused-pane inject is correct — but any server-side automation must not assume "focused" ≠ itself. Target an explicit `pane_id` when not acting for the user.
- Prefer `pane run` over `send-text`+`send-keys enter` for the atomic submit.
- **Untested (no second client yet):** inject while the same pane is mirrored by a ttyd client (ticket 02). Expected fine (Herdr multiplexes clients) but verify during the 02 prototype.
- `herdr agent send <target> <text>` not live-tested (all agents were real user sessions); documented as an alternative that targets the detected agent's pane.
