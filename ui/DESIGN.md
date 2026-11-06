---
name: agnostic-ai local configuration UI
description: A ruled ledger of what your agents receive, with findings and controls in plain view.
colors:
  light-canvas: "#f3f1ec"
  light-sidebar: "#eceae4"
  light-surface: "#fbfaf8"
  light-raised: "#e9e6df"
  light-sunken: "#f0ede7"
  light-field: "#ffffff"
  light-slab: "#16130f"
  light-line: "#d8d4ca"
  light-control: "#7f776c"
  light-rule: "#1a1714"
  light-ink: "#1a1714"
  light-soft: "#45403a"
  light-muted: "#675f56"
  light-faint: "#857d72"
  light-rust: "#a8481a"
  light-rust-strong: "#8c3912"
  light-permitted: "#2f6a2a"
  light-approval: "#875800"
  light-blocked: "#ad2f1f"
  light-info: "#2c5c93"
  dark-canvas: "#121110"
  dark-sidebar: "#171513"
  dark-surface: "#1b1917"
  dark-raised: "#25221e"
  dark-sunken: "#0f0e0d"
  dark-field: "#0e0d0c"
  dark-slab: "#0c0b0a"
  dark-line: "#34302a"
  dark-control: "#766d60"
  dark-rule: "#cfc7bb"
  dark-ink: "#f2ede5"
  dark-soft: "#cfc7bb"
  dark-muted: "#a59a8b"
  dark-faint: "#7f766a"
  dark-rust: "#f0a35e"
  dark-rust-strong: "#f7b97f"
  dark-permitted: "#9fcf8a"
  dark-approval: "#e6bb67"
  dark-blocked: "#f39689"
  dark-info: "#9dbde4"
  slab-ink: "#f0eae0"
  slab-dim: "#a99f91"
  slab-accent: "#f0a35e"
  backdrop: "rgba(0, 0, 0, 0.45)"
typography:
  display:
    fontFamily: "Archivo Narrow, Arial Narrow, sans-serif"
    fontSize: "34px"
    fontWeight: 700
    lineHeight: 1.05
    letterSpacing: "-0.01em"
  headline:
    fontFamily: "Archivo Narrow, Arial Narrow, sans-serif"
    fontSize: "20px"
    fontWeight: 700
    lineHeight: 1.15
  stat:
    fontFamily: "Archivo Narrow, Arial Narrow, sans-serif"
    fontSize: "40px"
    fontWeight: 700
    lineHeight: 1
  title:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 600
    lineHeight: 1.35
  body:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
  small:
    fontFamily: "Atkinson Hyperlegible Next, ui-sans-serif, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.45
  label:
    fontFamily: "ui-monospace, SFMono-Regular, JetBrains Mono, Menlo, Consolas, monospace"
    fontSize: "11px"
    fontWeight: 400
    lineHeight: 1.3
rounded:
  small: "2px"
  medium: "4px"
  large: "6px"
  pill: "999px"
spacing:
  0_5: "2px"
  1: "4px"
  1_5: "6px"
  2: "8px"
  3: "12px"
  4: "16px"
  5: "20px"
  6: "24px"
  8: "32px"
  10: "40px"
  14: "56px"
---

# Design System: agnostic-ai local UI

## Overview

**Direction: "Field Ledger"**

The interface reads like a ruled ledger of what your agents receive. Each page is a numbered section of a manual: a condensed heading over a 2px ink rule, then full-width rows separated by hairlines. A row that needs attention carries a 3px rail on its left edge: rust when it is linked, amber when it has findings, red when it fails. Color appears only where it means something.

The direction was worked out in code, not in a design tool. The frontmatter above holds the target tokens. The interface lives in `src/styles.css`, `src/components/Shell.tsx` and `src/components/Section.tsx`, and moves to them one slice at a time (#3026), so some sections below still describe the current interface.

**From the docs site** (the [Toolchain Field Briefing](../DESIGN.md)) it takes the rust signal, warm ink on warm paper, hairline rules, monospace metadata for paths, counts and versions, the dark slab block, and no shadows. The top bar becomes a slab in both themes, with a thin rust signal rule on its bottom edge that moves only while a request is in flight. Sidebar sections carry a monospace index, 01 Context to 05 Settings, as Lifecycle already numbers its stages.

**It drops** rounded card boxes, pill badges, full-row tints, gradients, glass, emoji and centered empty states. Corners stay between 0 and 4px; dialogs and the drawer use 6px, and the switch track is the only pill.

**Type:** Archivo Narrow 700 sets page and section headings; Atkinson Hyperlegible Next (400 and 600) sets body text and controls at 14px. Both ship inside the binary as small subsets under the SIL Open Font License; the interface loads no network font. System monospace keeps paths, keys and counts aligned.

## Colors

Paper forms the canvas; the sidebar sits one step darker, and surfaces hold grouped content. Ink carries names and values; muted ink carries descriptions and source paths. Faint ink is for disabled controls and decoration only, never for text a reader needs. Rust marks active navigation, focus, and configured lifecycle markers. Separate green, blue, amber, and red tokens communicate permission, information, approval, and blocked states, each with a solid wash for its background. Severity also has a visible text label. The slab tokens hold the dark top bar, which keeps the surface color until the chrome moves to it.

## Typography

Atkinson Hyperlegible Next serves body text and controls (14px, 1.5 line height) in two weights, 400 and 600; the browser never fakes a bold, and heavier weights resolve to 600. Archivo Narrow 700 sets page and section headings. Page titles use 28px. Controls use 13px. System monospace distinguishes keys, paths, code, and JSON controls. Descriptions use 13px; source paths use 12px and wrap within their row. Both faces load from the binary with a local Arial fallback scaled to their widths, so the swap does not reflow the page; `src/assets/fonts/README.md` shows how to rebuild them.

## Layout

The page scrolls as one document, so a wheel over the sidebar moves the content too; the 64px header and a 228px sidebar stick in place beside it. A sidebar taller than the window scrolls on its own. Pages have a 1240px maximum width. The sidebar lists the five sections, each with an icon and a short description, and ends with the configuration switch (**Open global settings**, or **Open project** on a global page), the release badge, and a quiet **Overview JSON** link that opens the JSON inspector. The sidebar ends its list with **Setup health**. The top bar holds the brand, the project name with its path on the same line in muted monospace (truncated from the left), then the health badge, search, and the theme button. At 900px and below, all five section links share one equal-width navigation row, Setup health leaves it because the badge opens it, and the footer becomes a short row under it that keeps the switch and the link. Descriptions hide. At 760px and below, icons sit above navigation labels, the health badge drops its words and shows each count with a letter, such as 1E 3W, and content uses 16px side gutters. Fields, values, metadata, and support groups stack. Built-in switches stay beside their names; rows with reset buttons place their controls below the description.

## Elevation & Depth

One-pixel borders, warm surface tones, and whitespace define groups. The interface uses no shadows.

## Shapes

Corners stay small: 2px on metadata, 4px on controls and grouped content, and 6px on dialogs and the drawer. Pills serve switches and compact badges. Source rows stay aligned within their containing list.

## Components

- **Change previews:** keep Apply and Cancel in a sticky heading below the application navigation. Tools, changed files, and review notes use native disclosures that start closed. Notes group by tool in a bounded scrolling region; Copy notes preserves every message. Pending source changes remain visible above the disclosures. Opening a proposal scrolls its summary into view.
- **Source rows:** show name, description, then muted monospace path. Layer, scope, edit action, and switches stay alongside the row on wide screens and below its text on narrow screens.
- **Section tabs:** Context, Tools, and Settings show sibling source groups in an underlined tab row, with counts and finding indicators. Default to the first populated group. Tabs use roving keyboard focus, Left/Right, Home, and End keys, and linked tab panels. Inactive panels stay mounted and hidden to preserve drafts and previews. Search selects a matching group, and deep links select their owning group; manual tab changes keep the query and clear the linked entry. The URL preserves the selected tab. Mobile tab rows scroll horizontally. Authority and Lifecycle retain continuous task views. Support and page findings stay outside panels.
- **Settings:** Project and Local (Global and Local in the global view) select both the displayed values and the save destination. Project shows the level's built-in states and settings; Local shows overrides with project values inherited where no override exists. The selector explains that local values win. Different values appear together as Project and Local; the local value uses the accent color. Local resets appear in Local. A project value blocked by an override stays visible, disabled, with a route to Local. Built-in counts follow the selected view. The config snapshot and tool resolution explicitly describe the combined configuration. Configured fields place label and description beside values and their source path. Lists use chips, booleans use switches, and other values use text. Native values group by tool. Unconfigured fields live in a disclosure.
- **Permissions:** Authority keeps the three editable lists visible even when unset. Each opens a labeled multiline editor and previews a bounded file diff before Save rules. Scope and destination stay beside the preview. Local replacements offer a project/local comparison and reset; inherited lists start with the project rules. A saved notice shows the sync command. Drafts survive a conflicting refresh, while switching scope or file starts a new draft.
- **Lifecycle:** an ordered list forms a numbered vertical timeline. Each stage has a native disclosure that starts closed, with its title, hook count, and description visible. Stages with hooks receive rust markers. Expand all and Collapse all sit beside Hooks. Search opens matching stages; linked hooks and findings open their stage. Tool icons identify stage support inside each disclosure. With no hooks at all, the stages render as a plain numbered list with their descriptions and tool icons, without disclosures, counts, or Expand all and Collapse all.
- **Search:** each page has a labeled search input with an icon and live result count. Search matches names, paths, descriptions, and values across source tabs; matching unconfigured fields open automatically. Tab badges show matching counts while searching, with separate finding counts. Items with findings remain available in their group. Clearing search restores focus to the input and keeps the active tab; changing pages resets search and tab selection.
- **Setup health:** a compact badge in the top bar counts findings by severity, red for errors and amber for warnings, and reads a muted **No findings** when clean. Its page groups every finding by severity, then section, then item, with the item name linking to its row, and reuses the section finding rows. **Copy all fixes** sits beside the total.
- **Findings:** flat rows use fine separators, quiet metadata, and plain source paths. Warning icons align vertically with their severity labels. Long messages preview three lines and expand on demand; copy actions always include the complete message. The summary scrolls the page to the findings heading, below the sticky header and navigation, and focuses it. Section errors appear before configuration controls. Individual findings retain severity, source, copy action, and a Details disclosure.
- **Release badge:** the foot of the sidebar shows the serving version in monospace beside the logo mark. A release links its notes in a new tab; on hover the border and arrow turn rust and the mark turns a quarter, landing on its own shape. A checkout build uses a dashed border and no link. When the config sets `requires`, a monospace line under the version names it: muted when the version meets it, in the warning color when it does not, with a title that says what to run. It renders only when `/api/health` names a version and hides with the descriptions at 900px and below.
- **Accessible states:** current navigation uses `aria-current`; controls have visible focus outlines. Loading announces busy state, failed edits use alerts, and pending switches prevent repeat actions. Narrow-screen navigation, edit controls, and switches have 44px minimum touch targets. Reduced-motion styles remove animations and transitions.

## Do's and Don'ts

Preserve source paths, findings, native disclosures, and keyboard focus when changing layout. Keep status text alongside color. Do not hide findings to produce an empty search result or replace the lifecycle sequence with unrelated cards.
