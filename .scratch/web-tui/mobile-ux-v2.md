# Mobile UX v2 — promptbox first-class + topbar

Post-v0.1.0 human-QA on device (herdr.tigor.web.id) surfaced 5 mobile issues.
Design locked with user. Layout model = **A** (split, promptbox prominent).

## Locked design

### Topbar (NEW, fixed top, replaces floating controls)
- Single fixed bar at the top of the viewport, above the terminal.
- Contents left→right: `[font −]  [font +]  …spacer…  [accessory-keys toggle]`.
- Accessory-keys toggle is the RIGHTMOST control.
- Removes App.svelte `.font-controls` (was fixed bottom-right) and KeyBar's
  own `.toggle` button (was fixed bottom-right) — those two overlapped (#3).
- Topbar owns the KeyBar visibility state now; KeyBar becomes a controlled
  child (visible passed in), no self-owned toggle.

### Layout A — vertical stack
Top→bottom: **topbar → terminal (flex:1) → promptbox → accessory keys (when on) → soft keyboard.**
- Terminal stays visible; promptbox is the prominent bottom surface.
- Promptbox is reactive to the soft keyboard: when the keyboard opens the
  whole stack rides above it (interactive-widget=resizes-content already in
  index.html gives layout-viewport shrink; ensure promptbox + accessory bar
  are in normal flow / correctly ordered so neither is hidden — #2).
- Accessory keys must NOT cover the promptbox (#2): they sit between the
  promptbox and the keyboard, not floating over the promptbox.

## Issue → fix map
1. **Pill name too long (ALL views).** Pill.svelte renders full `file.name`
   (max-width 10rem). Show a SHORT name in EVERY view (desktop + mobile,
   not just narrow): aggressive truncate keeping the extension (e.g.
   `Screensh….jpg`), consistent across all breakpoints. Mime badge already
   conveys type.
2. **Promptbox hidden by accessory keys + must react to soft keyboard.**
   Restructure App.svelte layout per Layout A above so promptbox, accessory
   bar, and keyboard stack in order and promptbox is never covered.
3. **Font +/- overlap accessory-keys toggle.** Both were fixed bottom-right
   at the same coords. Move all three into the new topbar.
4. **Pill deletes too easily on touch.** One tap on `×` (and native
   backspace on contenteditable=false) removes the pill instantly. Make it
   two-step: tap pill → selected/armed state (show a clear remove affordance
   / undo), OR long-press to remove. Must stay keyboard-accessible.
5. **Promptbox first-class on mobile.** Layout A with a prominent,
   auto-focusable promptbox is the vehicle. Promptbox UX takes priority
   over terminal chrome on narrow viewports.

## Out of scope
- No mode-toggle / bottom-sheet (rejected in favor of A).
- Desktop layout unchanged except the topbar (harmless there; toggle may
  stay hidden on fine-pointer as today).

## Orchestration
L work, tightly coupled on App.svelte layout → **fleet, 1 DAG**. Foundation
node (topbar + Layout A restructure) first; pill-name, pill-delete,
promptbox-focus polish depend on it (same file, so mostly sequential).
