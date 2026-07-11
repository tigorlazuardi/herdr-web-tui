# Lock the TUI rendering transport

Type: prototype
Status: resolved
Blocked by: —

## Question

Which pty-wrapper renders the Herdr TUI in a browser cleanly, and does it hold up? Leaning **ttyd** (vs gotty / wetty / custom node-pty + xterm.js).

Verify by prototype:
- `herdr` runs inside the wrapper's pty and renders full TUI (colors, layout, resize).
- **Mouse-first** works — xterm.js mouse events forward through to Herdr's clickable UI.
- Terminal resize propagates to the pty (SIGWINCH / TIOCSWINSZ).
- **Responsive / mobile view**: Herdr picks its mobile single-column layout purely from terminal **width in columns** (`[ui] mobile_width_threshold`, default 64 cols). So mobile view must arrive via accurate cols×rows reporting on the resize path — no Herdr-specific API. Verify: xterm.js fit-addon reports cols/rows matching the device viewport (phone portrait → ~30-50 cols → mobile), and orientation/resize changes propagate. Note whether we tune `mobile_width_threshold` or the web font size to hit the right column count on phones.
- ws stream is stable enough to build on.

Lock the tool + note any gotchas (e.g. `HERDR_ENV`, nested-launch block, write-only vs interactive).

## Findings (live smoke test, ttyd 1.7.7 + herdr 0.7.3)

- **Render + LAN**: `ttyd -W -p 7682 herdr` serves the full Herdr TUI at `http://<lan-ip>:7682/` over LAN (bind `0.0.0.0`; ttyd defaults to all interfaces). ttyd runs `herdr` in a fresh pty which attaches to the running server as a normal client — the nested-launch block only applies inside a Herdr pane (`HERDR_ENV`), not here. `-W` = writable/interactive.
- **Mobile view is cols-driven, and raw ttyd fails it on phones.** ttyd's bundled `index.html` has **no `<meta name="viewport" content="width=device-width">`**, so a phone uses a wide layout viewport (~980px), reports many columns, and Herdr stays in desktop layout (>64 cols). Bumping the font (`ttyd -t fontSize=32`) shrinks the column count below `[ui] mobile_width_threshold` (64) and Herdr **flips to the mobile single-column layout** — confirmed on-device. So mobile view works purely through the cols count; the blocker is ttyd's missing viewport meta + default font, not Herdr.
- **Production fix**: serve our **own thin frontend** (xterm.js + fit-addon + a proper `width=device-width` viewport meta) so columns match the real device width at a normal font on any screen. ttyd is then optional — either keep it purely as the pty websocket backend, or run our own pty ws (node-pty). Raw ttyd's page is not shippable for mobile; a proxy that injects the viewport meta is a cheaper stopgap.

### Mouse / touch findings (live)

- **Desktop render**: clean, full TUI, readable, mouse focus works.
- **Right-click double-fires**: Herdr's context menu shows, but the **browser's native context menu also pops** (Vivaldi Undo/Copy/Paste/Bitwarden). ttyd does not `preventDefault` on `contextmenu`.
- **Touch**: normal taps are fine (expected). The problem is **long-press**, which triggers the mobile browser's zoom and stays zoomed until refresh. Narrower than first thought — a `user-scalable=no` viewport + `touch-action: none` still wanted to kill the long-press zoom, but taps themselves work.
- **No modifier keys on mobile (gap)**: the phone soft keyboard can't send `Ctrl` / `Esc` / `Tab` / arrows — like Termux, a TUI needs an on-screen **accessory key bar**. Critical here: Herdr's prefix is `ctrl+b` and agents need `ctrl+c` / `esc`. Raw ttyd has no such bar. Our frontend must add a mobile key row (Ctrl, Esc, Tab, arrows, prefix) that injects the raw control sequences (e.g. `ctrl+b` → `\x02`). **The bar must be toggleable** — hidden on desktop or when the device has a hardware keyboard, shown on touch devices without one. Another reason raw ttyd is not shippable.

  Model it on Termux's extra-keys row (what Termux has and ttyd lacks):
  - **Sticky / one-shot modifiers** — the essential trick. Tap `CTRL` → it latches → the next keypress becomes Ctrl+key (you can't chord on a touchscreen). Same for `ALT` / `FN`. Without this there is no Ctrl+C / prefix on mobile.
  - **Default key set**: `ESC`, `CTRL`, `ALT`, `TAB`, `FN`, arrows (← ↑ ↓ →).
  - **Second row / swipe** for extras: `HOME`, `END`, `PGUP`, `PGDN`, `/`, `-`, `|`, `~`, `:`, `F1–F12`.
  - **Long-press → alternate key** popup.
  - Dedicated **prefix button** (`ctrl+b`) is a nice-to-have for Herdr.
  - Termux's **volume-key-as-Ctrl** fallback is **not possible in a browser** (no volume API) — skip.
- **Mobile soft keyboard works (good)**: after closing the keyboard, tapping the TUI re-summons it automatically — xterm's hidden focusable textarea gets focus on tap. Confirmed OK. Our own frontend must preserve this (keep the xterm helper textarea focusable so tap = keyboard).
- **Multi-client sizing = "last view wins"**: with a desktop and a mobile client attached at once, the shared pane resizes to whichever client was **last active** (Herdr sizes the pane to the last-active client's cols/rows), so the two contend and the view glitches on switch. Expected for a shared multiplexer; fine with a single active viewer. Feeds the multi-user fog.
- **Focus is server-wide** (see ticket 04 / skill): a browser client switching workspace moved `herdr pane current` for everyone — good for "inject into focused pane".

### Decision

**Raw ttyd's page is NOT shippable.** All three breakages (mobile view needs a font hack, double context menu, touch-zoom) come from ttyd's uncontrolled bundled HTML. Lock the transport as: **our own thin frontend hosting xterm.js** (+ fit-addon) that we fully control:
- `<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no">` — kills touch zoom; makes cols track device width so Herdr mobile view triggers at a normal font.
- `preventDefault` on `contextmenu` over the terminal — only Herdr's menu shows.
- `touch-action: none` on the terminal element — no pan/zoom gestures eating taps.
- fit-addon + resize → cols/rows to the pty (SIGWINCH), driving Herdr's responsive/mobile layout.
- **PWA manifest + `display: standalone`** (+ `theme-color` / iOS web-app meta) — phone treats it as an app: fullscreen, no browser URL bar stealing vertical space (more rows/cols). Cheap: a static `manifest.json` + a couple meta tags.
- **`overscroll-behavior: none`** on the terminal/page — kills pull-to-refresh so a stray swipe can't reload the page. One CSS line.
- **ws auto-reconnect** — mobile networks drop; Herdr panes survive detach (the server owns them), so reconnect is just re-attach + re-render, no new state. Requirement, not nice-to-have.
- *(optional)* a **font +/− button** using the proven `fontSize` lever, letting the user force the column count (hence mobile vs desktop layout) by hand.

ttyd stays only as an optional pty↔websocket backend (or replace with our own node-pty ws). The prebuilt ttyd UI is rejected because the product needs its own page anyway (artifact promptbox, pill editor, thumbnails).

**Per-session render (multi-user)**: route `(domain)/(path)` → the pty backend spawns `herdr --session <path>` (fallback `default`, name sanitized to `[a-zA-Z0-9-]`). Each named session is an isolated namespace (own socket, focus, sizing), so different users/paths never contend — this replaces raw ttyd's single fixed command and sidesteps the "last active client wins" glitch. (This is why our own pty-ws backend is cleaner than one fixed ttyd instance: the command varies per URL path.)

### Verified by transitivity (demo optional)

Inject-while-mirrored: ttyd broadcasts the pane's pty live (confirmed — the browser shows real-time pane content), and ticket 03 proved `herdr pane run` writes to that pty. So an injected line will appear live in any attached client. High confidence without a separate demo.
