---
name: agnostic-ai local configuration UI
description: A calm report on what your agents receive, written for people who do not read config files.
colors:
  light-canvas: "#eeebe3"
  light-surface: "#f8f6f1"
  light-sunken: "#e6e2d8"
  light-field: "#ffffff"
  light-line: "#d6d2c7"
  light-control: "#6e7672"
  light-ink: "#22282a"
  light-soft: "#3f4846"
  light-muted: "#4c5654"
  light-faint: "#6f7976"
  light-accent: "#256f61"
  light-accent-strong: "#1f5e52"
  light-accent-wash: "#dbe6df"
  light-focus: "#2f5fd0"
  light-permitted: "#2f6b3a"
  light-approval: "#7f560c"
  light-blocked: "#a8382a"
  light-info: "#2d5d86"
  dark-canvas: "#161b1c"
  dark-surface: "#202728"
  dark-sunken: "#121617"
  dark-field: "#0f1314"
  dark-line: "#2e3738"
  dark-control: "#75807d"
  dark-ink: "#ece8df"
  dark-soft: "#cfd3cd"
  dark-muted: "#b3beb9"
  dark-faint: "#8a9692"
  dark-accent: "#7cc4b2"
  dark-accent-strong: "#9ad6c7"
  dark-accent-wash: "#1f3330"
  dark-focus: "#8fb0ff"
  dark-permitted: "#93cf9b"
  dark-approval: "#e0b252"
  dark-blocked: "#f2998c"
  dark-info: "#9cc1e6"
  rail: "#22282a"
  rail-dark: "#101415"
  rail-raised: "#2f3739"
  rail-raised-dark: "#232a2b"
  rail-ink: "#eeebe3"
  rail-dim: "#a9b4b0"
  backdrop: "rgba(16, 20, 21, 0.5)"
typography:
  verdict:
    fontFamily: "Fraunces, Georgia, serif"
    fontSize: "52px"
    fontWeight: 500
    lineHeight: 1.12
    letterSpacing: "-0.02em"
  headline:
    fontFamily: "Fraunces, Georgia, serif"
    fontSize: "22px"
    fontWeight: 600
    lineHeight: 1.3
  title:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 600
    lineHeight: 1.4
  body:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.55
  small:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "12px"
    fontWeight: 400
    lineHeight: 1.3
    letterSpacing: "0.08em"
rounded:
  small: "3px"
  medium: "8px"
  large: "14px"
  pill: "999px"
spacing:
  1: "4px"
  2: "8px"
  3: "12px"
  4: "16px"
  5: "20px"
  6: "24px"
  7: "28px"
  10: "40px"
  14: "56px"
  18: "72px"
---

# Design System: agnostic-ai local UI

## Overview

**Direction: "Mineral"**

The interface reads like a calm report on your setup. Every page opens with a sentence that answers its question in plain words, set large in a soft serif, before any list or control. A dark slate rail holds the navigation; the page itself is bone paper with verdigris for links and progress. Color marks state only: ochre for what needs a look, brick red for what is broken, green for allowed, blue for notes. Nothing decorative carries color.

The Paper file "agnostic-ai UI redesign (#3120)" holds the source frames: page **Directions** has the three explored directions, page **Mineral** has every page and state. When code and Paper disagree, this file wins. The Paper file lives on the maintainer's machine, so each build PR attaches screenshots of the frames it follows.

**Audience.** Someone who installed agnostic-ai for their team and has never opened a settings file. Section names stay (Context, Authority, Tools, Lifecycle, Settings), because docs and links use them, but the rail leads with each section's question: *What is the agent told?*, *What may it do?*, *What can it use?*, *What runs around it?*, *How is it set up?*, with a live summary under it, such as "Authority: 11 allowed, 3 ask, 4 denied". Words and labels follow the vocabulary in #3121: one label per action (Copy prompt, Preview, Apply, Cancel, Docs), "check" for lint and doctor, tool names instead of ids, and technical codes only under Details.

**It drops** the ruled-ledger look: condensed headings, 2px ink rules, monospace section indexes, the dark top-bar slab, and the rust accent. It keeps hairline rows for lists, monospace for paths and keys, and no shadows on surfaces.

## Colors

Bone (`canvas`) is the page; `surface` holds one grouped thing, such as a finding card or an editor; `sunken` sits behind code and chips. The rail is dark slate in both themes. Ink carries names and values, soft ink carries sentences, muted ink carries paths and secondary lines. Faint ink is for disabled controls only.

Verdigris (`accent`) marks links, the current page, switches that are on, and progress. Keyboard focus has its own blue `focus` token, so a focused link or current row never hides its ring in the accent color. `accent-wash` fills the selected rail row's dot area and hover states. State colors: `permitted` (green) for allowed, `approval` (ochre) for warnings and ask-first, `blocked` (brick) for errors and always-blocked, `info` (blue) for notes. Each state also has a text label or glyph; color never stands alone.

Contrast: every text token passes 4.5:1 on `canvas`, `surface` and `sunken` in its theme, except `faint`, which is never used for text a reader needs. On the rail, text uses only `rail-ink` and `rail-dim`, and state colors use their dark-theme values in both themes; all of those pass 4.5:1 on `rail` and `rail-raised`. Field borders and other controls use `control`, which passes 3:1 on `canvas`, `surface` and `field`. The tokens slice adds a unit test that reads these frontmatter tokens and checks every pair.

## Typography

- **Fraunces** (SIL OFL) sets the verdict sentence (52px, 34px at 760px and below), and section headings (22px, weight 600). The verdict may set its key phrase in the accent color, such as "**One rule** needs a closer look".
- **Atkinson Hyperlegible Next** (SIL OFL, already bundled) sets everything else: body 15px, controls 14px, small text 13px, in 400 and 600.
- **System monospace** sets paths, keys, rule values, counts and the small uppercase labels (12px, 0.08em tracking).

Fraunces ships as two subset woff2 files, weights 500 and 600, roman only, built from the variable font at optical size 72 from a pinned google/fonts commit with the same rebuild steps as the other faces (`src/assets/fonts/README.md`). Archivo Narrow is removed. Expect the binary to grow 30 to 50 KB; the fonts slice raises `scripts/binary-size-budget` with a measurement.

## Layout

- **Rail** (280px, dark slate, sticky, full height): wordmark and project name, then Setup health as a status row (dot in the state color, "Needs attention" or "All clear", and "1 to fix, 12 you could add"), then the five sections (plus Conversations once #3109 adds it), each as its question over its live summary in small dim text (Conversations: *Where did you leave off?*, with the open chat count). The current row fills `rail-raised` with an 8px radius. The foot holds Search with its shortcut, Open global settings or Open project, the version badge, and Overview JSON.
- **No top bar on wide screens.** What the top bar held moves into the rail or the page:
  - Finding counts by severity: the rail's Setup health row shows them as words ("1 error, 3 warnings", or "No findings" in `rail-dim`). On phones the top bar shows them as a chip ("1E 3W").
  - Project path: under the project name in the rail, monospace in `rail-dim`, truncated from the left, with the full path on hover.
  - Project GitHub link and agnostic-ai on GitHub: icon links in the rail foot.
  - Search, theme button and the configuration switch: the rail foot.
  - Mock and snapshot notice: one `Notice` strip at the top of the page content, above the verdict, once per page.
- **Page**: 72px side padding, 56px top, content width up to 1080px. Each page starts with the verdict block: the section name as the page `h1`, styled as the small monospace label; the verdict sentence as a paragraph; and one line of supporting facts. On Setup health the facts line ends with "Checked just now" after a live server fetch, or "From a saved snapshot" for snapshots and mock data, once per page.
- **At 900px and below** the rail collapses to a 60px top bar on canvas: wordmark and project on the left; the finding chip ("1E 3W"), Search and Menu as 44px buttons on the right. Under it, one row of equal-width cells holds every page link, with the icon over a short label (the section name) at 760px and below, and never scrolls, so every page link stays fully visible down to 320px as today. Menu opens a sheet from the left with the questions and summaries, the configuration switch, theme, version and Overview JSON.
- **At 760px and below** side padding is 16px, verdicts drop to 34px, finding cards stack their actions under the text as one full-width pill button.

## Components

- **Verdict block:** label, sentence, facts line. Setup health adds the feature track: one small bar (14x22px, 3px radius) per improvement the configured tools support (`health.improvements`), filled accent for those in use, outlined for the rest, with a line such as "2 of 14 features your tools support are in use". With no improvements data (the global view), the track hides.
- **Finding card:** `surface`, 14px radius, 4px left border in the state color. Severity and section in 13px 600 state color ("Worth a look · What agents may do", "Needs a fix · ..."), the message as a Fraunces 22px sentence, the fix and the source path in body text, then actions: **Copy prompt** as a dark ink pill and **Details** as a disclosure link (#3121 labels). Recheck sits beside them when a server is running.
- **List rows:** hairline (`line`) between rows, no boxes. Name (title, 180px lane), sentence (soft, grows), trailing action as an accent text link in a fixed lane. Paths go on a second line in muted monospace.
- **Buttons:** primary is an ink pill (`ink` fill, `canvas` text, 999px radius, 40px tall, 44px on touch). Secondary is a text link in accent, 600. Destructive confirms use `blocked`. No bordered box buttons.
- **Chips and badges:** `sunken` fill, 3px radius, monospace 12px. Counts in the rail and tabs use the same chip.
- **Tabs:** plain text tabs with a 2px accent underline on the current one; counts as chips.
- **Disclosures:** native `details`, chevron turns a quarter in 160ms. Summaries are at least 44px tall on touch.
- **Forms:** fields use `field`, 1px `control` border, 8px radius, 40px tall; focus turns the border accent and shows the focus ring. Switches are pills that fill accent when on.
- **Tables** (coding tool support, context size): no outer border, hairline rows, monospace head in muted ink at 12px uppercase. Numbers right-aligned in monospace. Share bars are 6px accent bars with 3px radius.
- **Lifecycle:** a vertical timeline. Stage numbers in monospace, a 10px dot on a 2px `line` rail; stages with hooks get a filled accent dot, the rest hollow. Expand all and Collapse all are accent text links beside the Hooks heading. Counts show only above zero (#3121).
- **Overlays** (search, JSON inspector, change previews, rail sheet): `surface`, 14px radius, 1px `line` ring, one soft shadow, `backdrop` behind.
- **Empty states:** a sentence in Fraunces 22px plus one line of what to do, left-aligned, no box.
- **Page frame:** verdict block; then the section's findings slot: errors as finding cards before the tabs, warnings counted in the facts line with a **Review** link that scrolls to the folded warnings list and focuses its heading; then a compact filter row (the section search: one field with an icon, a match count only while filtering) and the Docs link at its right, then the tabs, then the content. Coding tool support folds below the content as a closed disclosure with a one-line summary ("3 of 3 tools support rules").
- **Segmented control** (Project | Local, Global | Local): once per page beside the filter row; the selected segment fills `ink` with `canvas` text.
- **Selects:** same as fields, with a chevron.
- **Finding details:** the finding code (`LINT038`) is a chip inside Details, with the explanation and the exact prompt. Long messages show three lines and Show full message. Copy all fixes is an ink pill beside the Fix first heading. Recheck shows its result as a `Notice` under the card (fixed, still there, not confirmed).
- **Change previews and diffs:** a `surface` overlay panel; file diffs in monospace on `sunken`, added lines with a `permitted` left rule, removed lines with a `blocked` left rule, each line also prefixed + or -.
- **Saved and error states:** one `Notice` (from #3124): state rail, a plain title, the raw text under Details.
- **Add hook form, field resolution, global setup panel:** forms and disclosures as above inside a `surface` card.
- **Tool logos:** 18px, in a fixed 24px lane, with the tool name beside them, never logos alone.
- **Focus ring:** 2px `focus` outline with 2px offset on every focusable element (4.8:1 on canvas); on the rail it uses `rail-ink`.
- **Forced colors:** borders on cards, chips, pills and the feature track bars; the current rail row uses `Highlight`; state rails keep their width with `CanvasText`.
- **Loading:** a 2px accent line sweeps under the verdict label; reduced motion shows a steady line.

## Motion

Only state changes move: disclosure chevrons and switches in 160ms ease-out, the rail sheet slides in 200ms, overlays fade in 120ms. No motion on page load beyond the loading line. `prefers-reduced-motion` removes all transitions.

## Do's and Don'ts

- Do lead every page with its verdict sentence; it is the page's answer.
- Do keep source paths, finding ids, native disclosures and keyboard focus.
- Do keep status words beside color.
- Don't add boxes around lists; cards are for one finding or one editor.
- Don't use the accent for decoration. One color moment per screen.
- Don't use jargon in verdicts or row sentences ("lint", "spec", "serve"); keep it in paths and Details.
