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

The direction was worked out in code, not in a design tool. The frontmatter above holds the target tokens. The interface lives in `src/styles.css`, `src/components/Shell.tsx` and `src/components/SectionFrame.tsx` (with the files it composes), and moves to them one slice at a time (#3026), so some sections below still describe the current interface.

**From the docs site** (the [Toolchain Field Briefing](../DESIGN.md)) it takes the rust signal, warm ink on warm paper, hairline rules, monospace metadata for paths, counts and versions, the dark slab block, and no shadows. The top bar becomes a slab in both themes, with a thin rust signal rule on its bottom edge that moves only while a request is in flight. Sidebar sections carry a monospace index, 01 Context to 05 Settings, as Lifecycle already numbers its stages.

**It drops** rounded card boxes, pill badges, full-row tints, gradients, glass, emoji and centered empty states. Corners stay between 0 and 4px; dialogs and the drawer use 6px, and the switch track is the only pill.

**Type:** Archivo Narrow 700 sets page and section headings; Atkinson Hyperlegible Next (400 and 600) sets body text at 14px and controls at 13px. Both ship inside the binary as small subsets under the SIL Open Font License; the interface loads no network font. System monospace keeps paths, keys and counts aligned.

## Colors

Paper forms the canvas; the sidebar sits one step darker, and surfaces hold grouped content. Ink carries names and values; muted ink carries descriptions and source paths. Faint ink is for disabled controls and decoration only, never for text a reader needs. Rust marks active navigation, focus, and configured lifecycle markers. Separate green, blue, amber, and red tokens communicate permission, information, approval, and blocked states, each with a solid wash for its background. Severity also has a visible text label. The top bar is a dark slab in both themes: inside it the slab tokens stand in for ink, muted ink, lines and surfaces, and state colors use their dark-theme values. A 2px rust signal rule runs along its bottom edge; while the overview loads, a lighter segment sweeps along it (a steady light line with reduced motion).

## Typography

Atkinson Hyperlegible Next serves body text and controls (14px, 1.5 line height) in two weights, 400 and 600; the browser never fakes a bold, and heavier weights resolve to 600. Archivo Narrow 700 sets page and section headings. Page titles use 34px (28px at 760px and below) over a 2px ink rule, under a monospace index such as `02 / Authority` that screen readers skip; section headings use 20px, with a monospace Docs link. Controls use 13px. System monospace distinguishes keys, paths, code, and JSON controls. Descriptions use 13px; source paths use 12px and wrap within their row. Both faces load from the binary with a local Arial fallback scaled to their widths, so the swap does not reflow the page; `src/assets/fonts/README.md` shows how to rebuild them.

## Layout

The page scrolls as one document, so a wheel over the sidebar moves the content too; the 64px header and a 228px sidebar stick in place beside it. A sidebar taller than the window scrolls on its own. Pages have a 1240px maximum width. The sidebar, one step darker than the canvas, starts with **Conversations**, the page `/` opens with a running server or mock data (a static snapshot opens on Context), above a hairline and with its icon, then lists the five sections, each with a monospace index (01 to 05) and a short description; the current section has a 3px rust rail on its left edge and the surface fill. Setup health sits under a hairline and keeps its icon. At 760px and below the indexes hide and the icons return. The sidebar ends with the configuration switch (**Open global settings**, or **Open project** on a global page), the release badge, and a quiet **Overview JSON** link that opens the JSON inspector. The sidebar ends its list with **Setup health**. The top bar holds the brand, the project name with its path on the same line in muted monospace (truncated from the left), then the health badge, search, and the theme button. At 900px and below, Conversations and the five section links share one navigation row of equal columns, where a long label such as Conversations takes the room it needs from the others; Setup health leaves it because the badge opens it, and the footer becomes a short row under it that keeps the switch and the link. Descriptions hide. At 760px and below, icons sit above navigation labels, the health badge drops its words and shows each count with a letter, such as 1E 3W, and content uses 16px side gutters. Fields, values, metadata, and support groups stack. Built-in switches stay beside their names; rows with reset buttons place their controls below the description.

## Elevation & Depth

One-pixel borders, warm surface tones, and whitespace define groups. The interface uses no shadows on surfaces. Overlays (the JSON inspector, the search dialog and the floating global setup panel) carry a 1px ring and a soft drop shadow.

## Shapes

Corners stay small: 2px on metadata, 4px on controls and grouped content, and 6px on dialogs and the drawer. Only the switch track is a pill; badges and chips use 2px corners. Source rows stay aligned within their containing list.

## Components

- **Change previews:** keep Apply and Cancel in a sticky heading below the application navigation. Tools, changed files, and review notes use native disclosures that start closed. Notes group by tool in a bounded scrolling region; Copy notes preserves every message. Pending source changes remain visible above the disclosures. Opening a proposal scrolls its summary into view.
- **Source rows:** full-width ruled rows, not boxed cards: a control-colored rule above the list and hairlines between rows. Each shows name, description in soft text, then muted monospace path; the layer tag is monospace on the sunken tone. A 3px rail on the left edge marks state: amber for findings, red when one is an error, rust for a linked row. Rows carry no full-row tint. Layer, scope, edit action, and switches stay alongside the row on wide screens and below its text on narrow screens.
- **Badges and chips:** 2px corners and a 1px border; badges use monospace at 11.5px in the state color on its wash, and keep their glyph.
- **Notices:** banners, callouts and the missing-entry note use a 3px rail in the state color on its wash, with no full border. `Notice` in `Banner.tsx` takes a tone (info, success, warning, error), a title, a body, actions and **Dismiss**; an error gets a plain title and keeps the raw message under **Details** with **Copy details**. Empty states sit left-aligned in a dashed box.
- **Disclosures:** stay native and draw their own chevron, which turns a quarter in 160ms when open: Show content, finding Details, folded warnings, Lifecycle stages, unconfigured fields, tool resolution, change previews and their notes, changed files, and the local comparison. On narrow screens their summaries are at least 44px tall.
- **Tables:** a 1px border with 4px corners; the head has no fill, uses monospace at 11px in muted ink, and sits on a 2px ink rule. Numbers align right in monospace. Coding tool support marks supported tools with a filled green square and unsupported ones with an outlined square, beside the same words.
- **Context size:** the startup total uses the 40px condensed figure, with bytes in monospace. Each source's share is a 6px bar in the meter color with 2px corners, under its percentage. A startup check over its word budget carries a 3px amber rail, the color of the lint warning it raises.
- **Section tabs:** Context, Tools, and Settings show sibling source groups in an underlined tab row, with counts and finding indicators. Tab labels use 13px at 600 and change color on hover; counts use monospace chips with 2px corners, and finding counts are amber with a small square mark. Default to the first populated group. Tabs use roving keyboard focus, Left/Right, Home, and End keys, and linked tab panels. Inactive panels stay mounted and hidden to preserve drafts and previews. Search selects a matching group, and deep links select their owning group; manual tab changes keep the query and clear the linked entry. The URL preserves the selected tab. Mobile tab rows scroll horizontally. Authority and Lifecycle retain continuous task views. Support and page findings stay outside panels.
- **Coding tool support:** a native disclosure between the page search and the tab row, titled Coding tool support, with its Docs link at the right while it is open. It starts open above 760px and folded at 760px and below, so the tab row stays on a phone's first screen. Opening or folding it is remembered per section in `localStorage` and wins on every width. Each kind name links to its heading, selecting the tab that holds it; each tab's hover text lists its kinds.
- **Forms:** inputs, selects and textareas use the field color, a 1px control border and 4px corners at 36px tall; focus turns the border rust beside the focus outline. Textareas use monospace. The selected segment of a segmented control fills with ink and paper text (the system highlight in forced colors). Switches fill rust when on, moving in 160ms. On wide screens a dotted leader runs from a set field's name toward its value.
- **Settings:** Project and Local (Global and Local in the global view) select both the displayed values and the save destination. Project shows the level's built-in states and settings; Local shows overrides with project values inherited where no override exists. The selector explains that local values win. Different values appear together as Project and Local; the local value uses the accent color. Local resets appear in Local. A project value blocked by an override stays visible, disabled, with a route to Local. Built-in counts follow the selected view. The config snapshot and tool resolution explicitly describe the combined configuration. Configured fields place label and description beside values and their source path. Lists use chips, booleans use switches, and other values use text. Native values group by tool. Unconfigured fields live in a disclosure.
- **Permissions:** Authority keeps the three editable lists visible even when unset. Each opens a labeled multiline editor and previews a bounded file diff before Save rules. Scope and destination stay beside the preview. Local replacements offer a project/local comparison and reset; inherited lists start with the project rules. A saved notice shows the sync command. Drafts survive a conflicting refresh, while switching scope or file starts a new draft. The default permission mode and protected paths also stay visible when unset: the mode is a select with Save in the global view and read-only with Open global settings in a project, and protected paths use the same editor, preview, and saved notice as the lists, with one path per line, a decision, and a reason.
- **Lifecycle:** an ordered list forms a numbered vertical timeline: a two-digit monospace number, then an 8px square marker on a control-colored rail. Each stage has a native disclosure that starts closed, with its title, monospace hook count, and description visible. Stages with hooks get a filled rust marker; the others stay hollow. Expand all and Collapse all sit beside Hooks. Search opens matching stages; linked hooks and findings open their stage. Tool icons identify stage support inside each disclosure. With no hooks at all, the stages render as a plain numbered list with their descriptions and tool icons, without disclosures, counts, or Expand all and Collapse all.
- **Search:** each page has a labeled search input with an icon and live result count. Search matches names, paths, descriptions, and values across source tabs; matching unconfigured fields open automatically. Tab badges show matching counts while searching, with separate finding counts. Items with findings remain available in their group. Clearing search restores focus to the input and keeps the active tab; changing pages resets search and tab selection.
- **Conversations:** the page header lists each tool's store as a chip with its logo, label, and the conversations loaded so far of every one the tool holds. A found store's chip is the tool filter: a toggle button with `aria-pressed` that shows only that tool, pressed again to show every tool, with the accent border, wash, and text when pressed, and 44px tall at 760px and below. A store that was not found is a dashed, unpressable chip with **Not found** and the server's note under it. A labeled search, its status line, and a muted note, **Search covers the weeks loaded so far.**, sit above the rows. Live rows come first under **Open now**, then the rest under Today, Yesterday, This week, and Earlier, each heading with a count; empty groups hide. A row shows the tool logo, the title (**Untitled** when empty), a muted meta line (app, a live badge, folder name with the full path on hover, branch, relative time), up to three muted opening lines, and the last prompt clamped to two lines; a withheld field reads **Hidden: may hold a credential** under its label. The live badge says **Working** in green, **Waiting for you** in amber because it needs the person, **Idle** in neutral, or **Open now** when the tool does not say, with the full sentence, such as "Open now: the agent is working", as its title and screen-reader text. The primary button carries the server's route label and is `aria-disabled` while its request runs, so focus stays on it; **Copy command** sits beside it when the route has one. A status line under the row shows the reply in green when opened, amber when not, and red when the request failed. At 760px and below the actions move under the text and reach 44px. The list loads one week per page: a sentinel after the last group loads the week before about 400px ahead of the end, and **Load the week before** under it does the same from a click or the keyboard. While a week loads the button reads **Loading the week before…** with `aria-busy` and stays focusable; a failed week shows its error in red above **Try again**; the last week ends the list with a muted **That's every conversation.**, which takes focus when the button that loaded it goes away.
- **Setup health:** a compact badge in the top bar counts findings by severity, red for errors and amber for warnings, and reads a muted **No findings** when clean. Its page groups every finding by severity, then section, then item, with the item name linking to its row, and reuses the section finding rows. **Copy all fixes** sits beside the total.
- **Findings:** flat rows use fine separators, quiet metadata, and plain source paths. Warning icons align vertically with their severity labels. Long messages preview three lines and expand on demand; copy actions always include the complete message. The summary scrolls the page to the findings heading, below the sticky header and navigation, and focuses it. Section errors appear before configuration controls. Individual findings retain severity, source, copy action, and a Details disclosure.
- **Release badge:** the foot of the sidebar shows the serving version in monospace beside the logo mark. A release links its notes in a new tab; on hover the border and arrow turn rust and the mark turns a quarter, landing on its own shape. A checkout build uses a dashed border and no link. When the config sets `requires`, a monospace line under the version names it: muted when the version meets it, in the warning color when it does not, with a title that says what to run. It renders only when `/api/health` names a version and hides with the descriptions at 900px and below.
- **Accessible states:** current navigation uses `aria-current`; controls have visible focus outlines. Loading announces busy state, failed edits use alerts, and pending switches prevent repeat actions. Narrow-screen navigation, edit controls, and switches have 44px minimum touch targets. Reduced-motion styles remove animations and transitions.

## Do's and Don'ts

Preserve source paths, findings, native disclosures, and keyboard focus when changing layout. Keep status text alongside color. Do not hide findings to produce an empty search result or replace the lifecycle sequence with unrelated cards.
