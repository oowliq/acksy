---
name: Acksy
description: Calm Catppuccin surfaces where related alerts settle into one tray instead of flooding the list.
colors:
  bg: "#1e1e2e"
  bg-side: "#181825"
  bg-sunken: "#11111b"
  bg-selected: "#313244"
  bg-hover: "color-mix(in oklab, #313244 55%, transparent)"
  line: "#313244"
  line-strong: "#45475a"
  fg: "#cdd6f4"
  fg-2: "#bac2de"
  fg-muted: "#a6adc8"
  fg-faint: "#9399b2"
  accent: "#b4befe"
  accent-fg: "#11111b"
  accent-soft: "color-mix(in oklab, #b4befe 16%, transparent)"
  prio-high: "#f38ba8"
  prio-mid: "#fab387"
  ok: "#a6e3a1"
  code: "#f5c2e7"
  topic-mauve: "#cba6f7"
  topic-sky: "#89dceb"
  topic-teal: "#94e2d5"
  topic-yellow: "#f9e2af"
  topic-sapphire: "#74c7ec"
  topic-blue: "#89b4fa"
  latte-bg: "#eff1f5"
  latte-bg-side: "#e6e9ef"
  latte-bg-sunken: "#dce0e8"
  latte-bg-selected: "#ccd0da"
  latte-line-strong: "#bcc0cc"
  latte-fg: "#4c4f69"
  latte-fg-2: "#5c5f77"
  latte-fg-muted: "#6c6f85"
  latte-accent: "#1e66f5"
  latte-accent-fg: "#ffffff"
  latte-prio-high: "#d20f39"
  latte-prio-mid: "#fe640b"
  latte-ok: "#40a02b"
  latte-code: "#b0369a"
typography:
  headline:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "22px"
    fontWeight: 600
    lineHeight: 1.25
  title:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "16px"
    fontWeight: 600
    lineHeight: 1.45
  body:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.45
    fontFeature: "tnum"
  reading:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.65
  label:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.45
  caption:
    fontFamily: "system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.45
  mono:
    fontFamily: "ui-monospace, 'SF Mono', 'JetBrains Mono', Menlo, Consolas, monospace"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  control: "6px"
  tray: "10px"
  panel: "14px"
spacing:
  xs: "4px"
  sm: "8px"
  row: "10px"
  md: "12px"
  lg: "24px"
  xl: "40px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-fg}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "32px"
  button-secondary:
    textColor: "{colors.fg}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "32px"
  button-secondary-hover:
    backgroundColor: "{colors.bg-hover}"
  button-ghost:
    textColor: "{colors.fg-2}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "32px"
  button-ghost-hover:
    backgroundColor: "{colors.bg-hover}"
    textColor: "{colors.fg}"
  button-confirm:
    backgroundColor: "{colors.bg-selected}"
    textColor: "{colors.fg}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "32px"
  button-sm:
    padding: "0 10px"
    height: "28px"
  icon-button:
    textColor: "{colors.fg-2}"
    rounded: "{rounded.control}"
    size: "32px"
  input-field:
    backgroundColor: "{colors.bg-sunken}"
    textColor: "{colors.fg}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 12px"
    height: "40px"
  segmented-option-active:
    backgroundColor: "{colors.bg-selected}"
    textColor: "{colors.fg}"
    typography: "{typography.label}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "28px"
  nav-item:
    textColor: "{colors.fg-2}"
    typography: "{typography.body}"
    rounded: "{rounded.control}"
    padding: "0 10px"
    height: "32px"
  nav-item-active:
    backgroundColor: "{colors.bg-selected}"
    textColor: "{colors.fg}"
  notification-row:
    typography: "{typography.body}"
    padding: "10px 12px 10px 6px"
  notification-row-active:
    backgroundColor: "{colors.bg-selected}"
  tray:
    backgroundColor: "{colors.bg-side}"
    rounded: "{rounded.tray}"
  menu:
    backgroundColor: "{colors.bg-side}"
    rounded: "{rounded.tray}"
    padding: "4px"
  dialog:
    backgroundColor: "{colors.bg-side}"
    rounded: "{rounded.panel}"
    padding: "20px"
---

# Design System: Acksy

## Overview

**Creative North Star: "The Bento Tray"**

Acksy is a notification inbox built to be cleared to zero, and its visual world is a quiet lacquered box with compartments. Catppuccin Mocha supplies the material: a mantle-dark sidebar, a base-dark list, crust-dark wells for code and inputs, all divided by single surface0 hairlines. Related alerts that share a group key do not repeat down the list; they settle into one inset compartment, the tray, with dashed partitions and a ×N count. Everything else is a compact, one-size row.

Density is mail-client tight, but colour is rationed. Lavender marks what is selected and the one action to take; red and peach speak only for priority 5 and 4; green says only "live" or "done". Rank inside a row comes from weight and ink, never size. Ink is never pure black or white, and the surfaces are flat until something floats above them. The world refuses both the grey, borderless mail clone and the neon terminal dashboard.

Mocha is the default on every device regardless of OS preference. Latte is an explicit opt-in light theme with the same structure; because Latte's lavender is too light to carry text, blue takes over the accent role there.

**Key Characteristics:**
- Catppuccin Mocha by default, Latte opt-in, both driven by the same semantic tokens.
- Hairline structure (1px surface0) instead of cards or shadows.
- One ink, one meaning: every hue has exactly one job.
- One type size in list rows (14px); rank by weight and colour.
- Read state is a mark in a fixed cell, and that mark is the toggle.
- Trays group related alerts; dedup repeats are counted inline.
- Tabular figures everywhere; times right-aligned.

## Colors

A pastel-on-dark Catppuccin palette in which the neutrals do almost all the work and each saturated hue holds a single, enforced job.

### Primary
- **Mocha Lavender** (accent): selection and the primary action only. The unread dot, the active count in the sidebar, the primary button ("Mark read", "Sign in"), links in markdown, the focus ring, the caret, the logo. Its soft wash (accent-soft) is the live-update flash and the empty-state badge.
- **Latte Blue** (latte-accent): the same role in the light theme, with white text on it.

### Secondary
- **Priority Red** (prio-high): priority 5 only, on the double-chevron glyph and its "P5" label.
- **Priority Peach** (prio-mid): priority 4 only, on the single chevron glyph; the "P4" label stays neutral.

### Tertiary
- **Live Green** (ok): success and connection state only, the filled "Live" dot in the sidebar and mobile header.
- **Code Pink** (code): inline code text in markdown bodies (Latte uses a deeper magenta, latte-code, for contrast).
- **Topic swatches** (topic-mauve, topic-sky, topic-teal, topic-yellow, topic-sapphire, topic-blue): a stable identity colour per topic, hashed from its id, shown only as a small rounded square beside the topic name. Never text, never fills.

### Neutral
- **Base** (bg): the list and reading pane.
- **Mantle** (bg-side): the sidebar, mobile header, trays, menus, dialogs, the login page.
- **Crust** (bg-sunken): wells: input fields, code blocks, the segmented control track, dialog overlay tint.
- **Surface0** (line / bg-selected): every 1px hairline, and the fill of a selected row, active nav item or active segment.
- **Surface0 wash** (bg-hover): hover fill on rows, nav items and ghost controls.
- **Surface1** (line-strong): borders on secondary buttons, inputs, Kbd keys, the hollow read ring, scrollbar thumbs.
- **Text** (fg): titles of unread rows, headings, primary copy.
- **Subtext1** (fg-2): read row titles, markdown body copy, ghost button labels.
- **Subtext0** (fg-muted): previews, times, meta, captions.
- **Overlay2** (fg-faint): placeholders, list markers, priority 1-2. Latte raises this to its subtext0 for contrast.

### Named Rules
**The One Ink, One Meaning Rule.** Lavender (blue in Latte) means selection or primary action; red and peach mean priority 5 and 4; green means live or success. No hue is borrowed for another job, and topic swatches are drawn from the hues no role owns (pink belongs to inline code, so it is not a swatch).

**The No Red Alarm Rule.** There is no danger colour. Destructive confirmation is an outlined neutral button (fg border on surface0); red stays reserved for priority.

**The No Pure Ink Rule.** Text and surfaces never use #000 or #fff in Mocha; every ink comes from the Catppuccin ramp. The only white is Latte's accent-fg on blue.

## Typography

**Display Font:** none; the system has no display face.
**Body Font:** system-ui (with -apple-system, Segoe UI, Roboto, Helvetica Neue, sans-serif)
**Label/Mono Font:** ui-monospace (with SF Mono, JetBrains Mono, Menlo, Consolas, monospace)

**Character:** The platform's own UI face, set small and steady, so the inbox reads like a native tool rather than a website. Mono appears only for code and key caps.

### Hierarchy
- **Headline** (600, 22px, 1.25): the reading-pane title (balanced, max 60ch) and the login heading.
- **Title** (600, 16px): dialog titles, empty-state headings, the wordmark in the sidebar.
- **Body** (400 / 600, 14px, 1.45): the app's base size; every list row, nav item, tray header and toolbar heading.
- **Reading** (400, 15px, 1.65): markdown notification bodies in subtext1, max 72ch; headings inside at 650 weight in fg, scaled 1.3em / 1.15em / 1em.
- **Label** (400 / 600, 13px): buttons, segmented options, menu items, form labels, the reader meta line.
- **Caption** (400, 12px): menu labels, the Topics heading, live status, tags.
- **Mono** (400, 13px code blocks; 11px key caps; 0.86em inline code).

### Named Rules
**The One Size Row Rule.** Inside a list row every element is 14px. An unread title is 600 in fg; a read title is 400 in fg-2; the preview, time and topic are 400 in fg-muted. Rank by weight and colour, never by size.

**The Tabular Figures Rule.** The body sets `font-variant-numeric: tabular-nums` globally; counts, ×N, times and snooze times all align. List times are right-aligned in their own column.

## Layout

Desktop (md, 768px and up) is three panes: a 232px mantle sidebar, a list column of minmax(340px, 520px), and the reading pane filling the rest, divided by 1px hairlines rather than gaps. Below md the app is one column: a 48px mantle header with a drawer trigger, the view name, the unread count in accent and the live status; opening an item replaces the list with the reader, which gains a back button.

Toolbars above the list and reader are at least 48px tall with 12px horizontal padding (24px in the reader on desktop). Rows run 10px vertical padding on a fixed grid: a 28px selection column, a 14px read-mark cell, the flexible text column, and an auto trailing column for time and topic. Trays inset 8px from the list edges. The reading body is padded 16px on phones and 40px on desktop, with 24px above the title and 64px below.

Touch is detected with `(hover: none)`, not width. On touch the selection column collapses until "Select" is pressed, the read-mark cell grows to a 44px target, the trailing column swaps the topic label for a 36px archive button, the topic swatch moves in front of the preview, Kbd hints disappear, and hover-only row actions are suppressed.

## Elevation & Depth

Flat by default. Depth comes from tonal layering of the Catppuccin ramp (crust below base below mantle surfaces) and 1px hairlines. A single shadow exists, and only for things that float over the layout: dropdown menus, the shortcuts dialog and the mobile navigation drawer. Overlays dim the page with crust at 70%.

### Shadow Vocabulary
- **Pop** (`box-shadow: 0 8px 24px -6px rgb(0 0 0 / 0.55), 0 2px 6px -2px rgb(0 0 0 / 0.4)`; Latte: `0 8px 24px -8px rgb(76 79 105 / 0.28), 0 2px 6px -2px rgb(76 79 105 / 0.18)`): menus, dialogs, drawer.

### Named Rules
**The Float-Only Shadow Rule.** Only surfaces that sit above the layout (menus, dialogs, drawers) cast a shadow. Rows, trays, panes and buttons stay flat; they separate by tone and hairline.

## Shapes

One radius family: 6px for controls (buttons, inputs, nav items, segments, focus outline), 10px for trays, menus and code wells, 14px for dialogs. Read marks and avatars are full circles. Topic swatches are small near-squares (8-10px with 2-3px corners) so identity colour never reads as a status dot. Borders are always 1px; tray partitions are dashed to read as compartments of one box rather than separate rows.

### Named Rules
**The Three Radii Rule.** Corners come from control (6px), tray (10px) or panel (14px). A new component picks the family member of its tier, not a new value.

## Components

### Buttons
Quiet, compact, label-sized controls; only one per toolbar is filled.
- **Shape:** gently rounded (6px), 32px tall (28px small), 13px label, icon gap 8px (6px small).
- **Primary:** accent fill with accent-fg text at 600 weight; hover brightens 10%, active dims 5%. Used for the single primary action (Mark read, Sign in).
- **Secondary (default):** 1px line-strong outline, fg text, hover fills with the surface0 wash, active with surface0.
- **Ghost:** no border, fg-2 text, hover wash and fg text. Used for toolbar actions (Mark all read, Mark group read, Delete at rest).
- **Confirm:** a neutral inverse for a destructive second press: 1px fg border, surface0 fill, 600 weight. Delete becomes "Press again to delete" for 4 seconds.
- **Icon button:** 32px (28px small) square, fg-2, same hover wash; always carries an aria-label and title.
- **Transitions:** background, colour and filter over 150ms ease-out; disabled drops to 45% opacity.
- **Kbd hints:** 11px mono key caps with a 1px line-strong border sit inside buttons; on the primary button they tint toward accent-fg. Hidden on touch.

### Segmented control
- **Style:** crust track with 2px inset; options are 28px, 13px, fg-muted.
- **State:** the checked option takes the surface0 fill and fg text. Used for Unread / All.

### Cards / Containers
There are no cards. The containers are trays, menus and dialogs.
- **Corner Style:** tray and menu 10px, dialog 14px.
- **Background:** mantle (bg-side).
- **Shadow Strategy:** trays flat; menus and dialogs carry Pop.
- **Border:** 1px line.
- **Internal Padding:** menus 4px with 32px items; dialogs 20px.

### Inputs / Fields
- **Style:** 40px tall, crust well, 1px line-strong border, 6px corners, 12px horizontal padding, fg-faint placeholder.
- **Focus:** the border turns accent; no glow.
- **Error:** a plain fg message with a muted alert glyph; no red.
- **Select (priority filter):** 28px, 13px, base fill, line-strong border.

### Navigation
- **Style:** 32px nav items, 14px fg-2 text, 16px icon, 10px gap, 6px corners.
- **States:** hover takes the surface0 wash and fg; active takes surface0 fill and fg, and its count turns accent. Counts are tabular and right-aligned.
- **Topics:** listed under a 12px fg-muted "Topics" heading, each led by its 10px swatch.
- **Footer:** live status (green dot when live, pulsing faint dot while reconnecting, hollow ring offline) and the user menu (theme switch, shortcuts, sign out).
- **Mobile:** the same content in a 280px drawer sliding from the left over a crust scrim.

### Notification Row (signature)
- One-size 14px row on a fixed grid, 1px line hairline below; hover takes the surface0 wash, the open item takes surface0 fill. A changed row restyles in place with a single accent-soft flash that fades over 1.6s (`cubic-bezier(0.16, 1, 0.3, 1)`); disabled under reduced motion.
- Line one: title, then the priority mark, then the repeat count. Line two: preview in fg-muted. Trailing: right-aligned time over the topic swatch and name; on pointer hover or focus these hide and the row's Mark read / Archive icon buttons appear in their place.

### Read Mark (signature)
- A fixed cell holds the read state: an 8px filled accent dot for unread, a hollow 1.5px line-strong ring (scaled 90%) for read, animating over 200ms. The cell is the toggle itself; on touch the dot grows to 10px and the hit target to 44px.

### Tray (signature)
- Notifications sharing a group key settle into one inset compartment: mantle fill, 1px line border, 10px corners, 8px inset from the list. The header shows the group key in fg-2 at 500 weight, the topic, a ×N count chip on surface0, and a ghost "Mark group read" while anything is unread. Rows inside are divided by dashed hairlines and omit their topic.

### Priority Mark and Repeat Count
- Priority is glyph plus text, never colour alone. Priority 5: double chevron and "P5" in prio-high at 600. Priority 4: single peach chevron with a neutral "P4". Priority 3 renders nothing. Priority 1-2: a down chevron in fg-faint.
- A deduplicated alert that fired more than once shows a repeat glyph and the count in fg-muted, inline after the title. This is distinct from a tray's ×N.

### Reader
- A toolbar of Mark read (primary while unread, secondary once read), Archive, Snooze (opens a mantle menu of presets with tabular times) and Delete (ghost, then confirm). Below: the 22px headline, a 13px meta line with the topic swatch, the markdown body, link buttons as secondary-style outlined links with an external glyph, and tags as 12px outlined chips.

## Do's and Don'ts

### Do:
- **Do** route every colour through the semantic tokens (bg, bg-side, line, fg, accent...), never raw Catppuccin values, so Mocha and Latte stay in lockstep.
- **Do** keep Mocha as the default regardless of OS preference, and use blue as the accent in Latte.
- **Do** keep every element in a list row at 14px and rank with 600 vs 400 weight and fg vs fg-2 vs fg-muted.
- **Do** show read state as the fixed-cell mark (filled accent dot / hollow ring) and make that mark the read toggle, 44px on touch.
- **Do** reserve trays for group_key groups with a ×N count, and show dedup repeats inline with the repeat glyph and count.
- **Do** pair every priority colour with a glyph and a "P5" / "P4" label, and render nothing for priority 3.
- **Do** keep row actions visible on touch: the read toggle and a trailing archive button, with the topic swatch in front of the preview.
- **Do** use tabular figures for every count and time, right-aligned in lists.
- **Do** pick corners from 6px / 10px / 14px and separate regions with 1px surface0 hairlines.

### Don't:
- **Don't** use red, or any new danger colour, for destructive actions; confirm with the outlined neutral confirm button.
- **Don't** use lavender (or Latte blue) for anything but selection and the primary action, or green for anything but live and success.
- **Don't** colour a topic with lavender, red, peach, green, flamingo or rosewater.
- **Don't** use pure black or pure white ink in Mocha.
- **Don't** put a shadow on rows, trays, panes or buttons; only floating menus, dialogs and the drawer get the Pop shadow.
- **Don't** change type size inside a list row to create hierarchy.
- **Don't** load a web display face; the system UI stack is the type.
