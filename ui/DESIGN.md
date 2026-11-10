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
    fontSize: "40px"
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

# Design system: agnostic-ai local UI

## Overview

The **Mineral** design starts each page with a plain-language answer to its question. A large serif sentence comes before lists and controls. Navigation uses a dark slate rail; pages use bone backgrounds and verdigris links. Color identifies actions and states.

Design for someone who installed agnostic-ai for a team but has never opened a settings file. Keep the section names used in docs and links: Context, Authority, Tools, Lifecycle, Settings, and Conversations. In the rail, pair each question with a live summary, such as "Authority: 4 blocked, 3 ask, 11 allowed". Use one label per action: Copy prompt, Preview, Apply, Cancel, Docs. Call lint and doctor results "checks", show tool names rather than ids, and keep technical codes under Details.

The Paper file "agnostic-ai UI redesign (#3120)" contains the design frames. **Directions** records the three explored designs; **Mineral** contains the pages and states. This document takes precedence over Paper and code. The Paper file lives on the maintainer's machine, so build PRs attach screenshots of the frames they use.

Use hairline list separators and monospace paths and keys. Surfaces have no shadows; overlays do.

## Colors

`canvas` is the page background, `surface` groups a finding or editor, and `sunken` sits behind code and chips. The rail stays dark slate in both themes. Use `ink` for names and values, `soft` for descriptions, `muted` for paths and secondary text, and `faint` only for disabled controls.

`accent` marks links, the current page, enabled switches, and progress. `accent-wash` fills selected-dot and hover areas. The separate blue `focus` token keeps keyboard focus visible against the accent.

State colors always have a text label or glyph:

- `permitted` (green): allowed.
- `approval` (ochre): warnings and ask-first.
- `blocked` (brick): errors and always-blocked.
- `info` (blue): notes.

Text tokens meet 4.5:1 contrast on `canvas`, `surface`, and `sunken` in each theme, except `faint`, which must not carry required text. Rail text uses `rail-ink` and `rail-dim`; rail states use dark-theme colors in both themes. These meet 4.5:1 on `rail` and `rail-raised`. The `control` token meets 3:1 on `canvas`, `surface`, and `field`. `src/styles/tokens.test.ts` checks these pairs against the frontmatter values.

## Typography

- **Fraunces** (SIL OFL): verdicts at 40px, or 30px at widths of 760px and below; headings at 22px, weight 600. A verdict can highlight its key phrase with `accent`.
- **Atkinson Hyperlegible Next** (SIL OFL): body text at 15px, controls at 14px, small text at 13px; weights 400 and 600.
- **System monospace**: paths, keys, rule values, counts, and uppercase labels at 12px with 0.08em tracking.

Fraunces ships as two roman WOFF2 subsets, weights 500 and 600, at optical size 72. See [font sources and rebuild steps](src/assets/fonts/README.md) for the pinned google/fonts revision and all bundled faces. Record measured binary growth in `scripts/binary-size-budget` when changing fonts.

## Layout

### Rail

The rail is 280px wide, sticky, and full height. It contains the wordmark, project name and path, Setup health, and section links. Setup health shows a colored dot, "Needs attention" or "All clear", and a summary such as "1 to fix, 12 you could add".

Section links show their question above a live summary in small dim text. Conversations asks *Where did you leave off?* and shows the open chat count. The configuration sections ask *What is the agent told?*, *What may it do?*, *What can it use?*, *What runs around it?*, and *How is it set up?* Summaries use one line with an ellipsis; link names and tooltips retain the full text. Align row text and place the health dot in the left padding. The selected row uses `rail-raised` and an 8px radius.

Show the project path below its name in muted monospace. Truncate from the left and expose the full path on hover. Put the project GitHub icon at the end of the name row, labelled "owner/repo on GitHub".

The rail foot contains Search and its shortcut, Open global settings or Open project, the theme button beside the agnostic-ai GitHub icon, then separate rows for the version and required release, Release notes, and Overview JSON.

While the overview and server details load, preserve the rail layout with placeholder bars for the project, summaries, configuration switch, and version. Bars use `rail-dim` at 28% opacity, a 10px height, and pill corners. They pulse unless reduced motion is enabled.

### Page

Wide pages have no top bar. Use 72px side padding, 56px top padding, and a maximum content width of 1080px. Each page starts with a visually hidden `h1`, a verdict paragraph, and one line of supporting facts. Show one mock or snapshot `Notice` above the verdict. Setup health ends its facts with "Checked just now" after a server fetch, or "From a saved snapshot" for snapshots and mock data.

### Narrow screens

At 900px and below, replace the rail with a 60px top bar on `canvas`. Put the wordmark and project on the left; finding counts ("1E 3W"), Search, and Menu on the right. Search and Menu have 44px targets. Below it, equal-width cells show every page link without horizontal scrolling, down to 320px. Menu opens a sheet from the left with the rail questions, summaries, configuration switch, theme, version, and Overview JSON.

At 760px and below, page links put icons above their short section names. Side padding becomes 16px and verdicts become 30px. Finding-card actions stack below the text as full-width pill buttons.

## Components

- **Verdict block:** an answer and up to three facts that affect a decision. **Tool support** opens the support table as an overlay; **Docs** sits at the right. Avoid "No findings", which the rail already communicates. Setup health adds one 14x22px bar with a 3px radius per supported improvement (`health.improvements`): accent-filled when used, outlined otherwise. A line states how many are in use. Hide the track when improvement data is absent, as in the global view.
- **Finding card:** `surface`, 14px radius, and a 4px state-colored left border. Severity and section use 13px, weight 600, in the state color. The plain title uses Fraunces 22px; the source path uses body text. Keep the check message and suggested fix under Details unless two findings in one file share a title. Actions are **Copy prompt**, **Details**, and **Recheck** when a server is running.
- **Finding details:** show the check code (for example `LINT038`) as a chip, the explanation, and the exact prompt. Limit long messages to three lines with **Show full message**. **Copy all fixes** sits beside **Fix first**. Recheck results appear below the card as a `Notice`: fixed, still there, or not confirmed.
- **List rows:** hairline `line` separators without boxes. Names occupy a 180px lane; descriptions use `soft` and fill the available space; the primary action is an accent link in a fixed lane. Paths sit below in muted monospace. Secondary actions appear on hover or keyboard focus, with a `surface` background (`RowActions` in `ui.tsx`). Touch screens and widths of 760px or less use a 44px ⋯ menu.
- **Buttons:** primary buttons use `ink`, `canvas` text, a 999px radius, and a 40px height (44px on touch). Secondary buttons are accent text links at weight 600. Destructive confirmations use `blocked`. Avoid bordered box buttons.
- **Chips and badges:** `sunken`, 3px radius, 12px monospace. Rail and tab counts use the same chip.
- **Tabs:** plain text, count chips, and a 2px accent underline on the selected tab.
- **Disclosures:** native `details`; chevrons turn a quarter in 160ms. Touch summaries are at least 44px tall.
- **Forms:** `field` background, 1px `control` border, 8px radius, 40px height. Focus uses an accent border and focus ring. Enabled switches fill with accent. Selects use the same field style with a chevron.
- **Tables:** coding tool support and context-size tables have no outer border, hairline rows, and muted uppercase monospace headings at 12px. Right-align numbers in monospace. Share bars are 6px high with accent fill and a 3px radius.
- **Lifecycle:** a vertical timeline with 10px dots on a 2px `line` rail, without stage numbers. Stages with hooks have filled accent dots; others are hollow. Show stages with hooks first and offer **Show all stages**. Group tool-specific events separately. **Expand all** and **Collapse all** sit beside Hooks. Each hook is a `surface` block with description, origin, **Change in Settings** for built-ins, and a **Command** disclosure. **Add hook** is the toolbar's primary button. Show counts only above zero.
- **Overlays:** search, JSON inspector, change previews, and the rail sheet use `surface`, a 14px radius, a 1px `line` border, one soft shadow, and `backdrop` behind them.
- **Empty states:** left-aligned Fraunces 22px sentence and one next-step line, without a box.
- **Page frame:** verdict, section findings, toolbar, tabs, content. Errors appear as cards before the tabs. Warnings appear in the facts line with **Review**, which scrolls to and focuses the collapsed warnings heading. The toolbar contains search, a match count only while filtering, and Project | Local when the page supports editing.
- **Scope selector:** Project | Local or Global | Local, once in the toolbar without a visible label. Explain the chosen scope in its tooltip and screen-reader status. The selected segment uses `ink` with `canvas` text.
- **Pick tray:** Setup health findings and suggestions have 20px accent checkboxes labelled "Add to prompt: <title>". A selection shows a sticky footer bar with `surface`, a 14px radius, the overlay border and shadow, a monospace live count, **Copy prompt**, and **Clear**. Keep it in page flow so it cannot cover the last row. Individual Copy prompt actions remain available.
- **Change previews:** a `surface` overlay with monospace diffs on `sunken`. Added lines use a `permitted` left border and `+`; removed lines use `blocked` and `-`.
- **Saved and error states:** `Notice` (`Banner.tsx`) uses a 4px state-colored left border on `surface`, with a title, body, actions, and **Dismiss**. Tones are info, success, warning, and error. Errors retain the raw message under **Details** with **Copy details**.
- **Add hook, field resolution, and global setup:** forms and disclosures inside `surface` cards.
- **Tool logos:** 18px in a fixed 24px lane, always beside the tool name.
- **Focus ring:** 2px `focus` outline with a 2px offset on every focusable element (4.8:1 on canvas). Use `rail-ink` on the rail.
- **Forced colors:** add borders to cards, chips, pills, and feature bars. Use `Highlight` for the current rail row and `CanvasText` for state borders, preserving their width.
- **Loading:** a 2px accent line sweeps below the verdict label. Reduced motion uses a steady line.

## Motion

Disclosure chevrons and switches use 160ms ease-out; the rail sheet slides in 200ms; overlays fade in 120ms. Only the loading line animates on page load. `prefers-reduced-motion` removes transitions.

## Do's and Don'ts

- Start each page with an answer in plain words.
- Keep source paths, finding ids, native disclosures, and keyboard focus.
- Pair status colors with words.
- Use cards for individual findings or editors, and separators for lists.
- Use accent color for actions and state, not decoration.
- Keep "lint", "spec", "serve", and other technical terms in paths and Details.
