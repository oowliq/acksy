---
version: 1
slug: "web-src-routes-index-tsx"
primary_target: "web/src/routes/index.tsx"
related_targets: ["web/src/routes/login.tsx","web/src/routes/__root.tsx","web/src/routes/_app/index.tsx"]
---

# Inbox (web app shell, login, inbox)

Scope: the authenticated web app shell, the login screen and the inbox route (list, reading pane, filters, bulk, realtime). Mode: **Operate**.

Audience and job: a homelab self-hoster clearing alerts to zero, mostly on a phone after a push at night, sometimes at a desk. Task: scan, read, act (read / archive / snooze / delete), keep zero honest. Content: real alerts from backups, monitoring, CI, home automation.

Constraints (confirmed): desktop = sidebar + list + reading pane, phone = one column with detail as its own screen; compact mail-like rows; keyboard shortcuts (j/k, e, a, s, #, x) plus hover/focus row actions (always visible on touch); Catppuccin Mocha by default, Latte for light; modern, minimal, pastel.

## Direction contract

THESIS: Calm Mocha surfaces where related alerts settle into one partitioned tray instead of flooding the list. Refuses the grey shadcn mail clone and the neon terminal dashboard.

OWN-WORLD: Catppuccin Mocha (Latte in light). Mantle sidebar, base list, crust code blocks; 1px surface0 hairlines, ink never pure black/white. One radius family (6/10/14). Lavender = selection and primary action only; red/peach = priority 4–5 only; green = success only. Read state is a mark in a fixed cell: filled lavender dot unread, empty ring read. Times right-aligned tabular figures.

STORY: The user sees what is unread and how urgent, opens one item, acknowledges it with one key, and watches the count fall to zero on every device.

FIRST VIEWPORT: Left 232px mantle sidebar (brand, Inbox/Snoozed/Archive with counts, topics with swatches and counts, user menu). Center list (~520px): segmented Unread/All, priority filter, "Mark all read"; one-size rows. Groups and dedup stacks render as one inset tray with partition hairlines and a ×N count. Right: reading pane with markdown body and the action bar (primary Mark read).

FORM: Bento (grounded, 6th of my ordered list in re-roll round 1). seed:5dfad8c8a9264671. Raises: live rows restyle in place (split-flap); read state as fixed-cell mark (centre rail); one type size in rows (slide rack); hairline structure, no pure black (plate section); right-aligned tabular times (j-card).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Adaptations (recorded at finish review)

- Sidebar counts: Inbox shows unread (lavender when active); Snoozed shows the snoozed count, always muted (it is not "unread"). Archive has no count by decision: everything there is already handled.
- Touch row actions (confirmed "always visible on touch"): every touch row shows its two one-tap actions, the read-state cell as a 44px read toggle and a trailing archive button (which takes the topic label's place; the topic swatch stays in the reader). Snooze and delete need a choice or a confirmation and live in the reader. Selection on touch is entered with a "Select" button.
- Dedup repeats are shown inline with a repeat glyph and count (not a tray); trays are reserved for group_key groups and carry ×N.

## Unresolved

- Search: done (M7.1). Field above the list toolbar, `/` focuses, searches within the current view; group view via a tray's "Show all".
- Settings screens: see `web-src-routes-settings.md` (account, push, tokens, topics done; webhooks, users, sign-in next).
