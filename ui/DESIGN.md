---
name: agnostic-ai local configuration UI
description: Warm, compact source configuration views with visible findings and controls.
colors:
  light-paper: "#f7f7f6"
  light-surface: "#ffffff"
  light-ink: "#1a1714"
  light-muted: "#6f6960"
  light-line: "#d9d7d1"
  light-rust: "#a8481a"
  dark-paper: "#14120f"
  dark-surface: "#1c1917"
  dark-ink: "#f2ede5"
  dark-muted: "#b3a89b"
  dark-line: "#38312a"
  dark-rust: "#f0a35e"
  backdrop: "rgba(0, 0, 0, 0.45)"
rounded:
  small: "6px"
  medium: "10px"
  large: "12px"
  pill: "999px"
---

# Design System: agnostic-ai local UI

## Overview

Context, Authority, Tools, Lifecycle, and Settings share one compact workspace. The warm neutrals, rust accent, and system fonts follow the [product identity](../DESIGN.md). This document describes the local interface in `src/styles.css`, `src/components/Shell.tsx`, and `src/components/Section.tsx`.

## Colors

Paper forms the canvas; surfaces hold navigation and grouped content. Ink carries names and values; muted ink carries descriptions and source paths. Rust marks active navigation, focus, and configured lifecycle markers. Separate green, blue, amber, and red tokens communicate permission, information, approval, and blocked states. Severity also has a visible text label.

## Typography

System sans serves headings, controls, and body text (15px, 1.5 line height). Page titles use 28px, weight 700, and tight tracking. Controls use 13px. System monospace distinguishes keys, paths, code, and JSON controls. Descriptions use 13px; source paths use 12px and wrap within their row.

## Layout

The page scrolls as one document, so a wheel over the sidebar moves the content too; the 64px header and a 228px sidebar stick in place beside it. A sidebar taller than the window scrolls on its own. Pages have a 1240px maximum width. The sidebar lists the five sections, each with an icon and a short description. At 900px and below, all five links share one equal-width navigation row. Descriptions hide. At 760px and below, icons sit above navigation labels, and content uses 16px side gutters. Fields, values, metadata, and support groups stack. Built-in switches stay beside their names; rows with reset buttons place their controls below the description.

## Elevation & Depth

One-pixel borders, warm surface tones, and whitespace define groups. The interface uses no shadows.

## Shapes

Small rounded corners soften controls and metadata. Larger corners group content; pills serve switches and compact badges. Source rows stay aligned within their containing list.

## Components

- **Change previews:** keep Apply and Cancel in a sticky heading below the application navigation. Tools, changed files, and review notes use native disclosures that start closed. Notes group by tool in a bounded scrolling region; Copy notes preserves every message. Pending source changes remain visible above the disclosures. Opening a proposal scrolls its summary into view.
- **Source rows:** show name, description, then muted monospace path. Layer, scope, edit action, and switches stay alongside the row on wide screens and below its text on narrow screens.
- **Section tabs:** Context, Tools, and Settings show sibling source groups in an underlined tab row, with counts and finding indicators. Default to the first populated group. Tabs use roving keyboard focus, Left/Right, Home, and End keys, and linked tab panels. Inactive panels stay mounted and hidden to preserve drafts and previews. Search selects a matching group, and deep links select their owning group; manual tab changes keep the query and clear the linked entry. The URL preserves the selected tab. Mobile tab rows scroll horizontally. Authority and Lifecycle retain continuous task views. Support and page findings stay outside panels.
- **Settings:** Shared and Personal select both the displayed values and the save destination. Shared shows shared built-in states and settings; Personal shows overrides with shared values inherited where no override exists. The selector explains that personal overrides take priority. Different values appear together as Shared and Personal; the personal value uses the accent color. Personal resets appear in Personal. A shared value blocked by an override stays visible, disabled, with a route to Personal. Built-in counts follow the selected view. The config snapshot and tool resolution explicitly describe the combined configuration. Configured fields place label and description beside values and their source path. Lists use chips, booleans use switches, and other values use text. Native values group by tool. Unconfigured fields live in a disclosure.
- **Permissions:** Authority keeps the three editable lists visible even when unset. Each opens a labeled multiline editor and previews a bounded file diff before Save rules. Scope and destination stay beside the preview. Personal replacements offer a shared/personal comparison and reset; inherited lists start with shared rules. A saved notice shows the sync command. Drafts survive a conflicting refresh, while switching scope or file starts a new draft.
- **Lifecycle:** an ordered list forms a numbered vertical timeline. Each stage has a native disclosure that starts closed, with its title, hook count, and description visible. Stages with hooks receive rust markers. Expand all and Collapse all sit beside Hooks. Search opens matching stages; linked hooks and findings open their stage. Tool icons identify stage support inside each disclosure. With no hooks at all, the stages render as a plain numbered list with their descriptions and tool icons, without disclosures, counts, or Expand all and Collapse all.
- **Search:** each page has a labeled search input with an icon and live result count. Search matches names, paths, descriptions, and values across source tabs; matching unconfigured fields open automatically. Tab badges show matching counts while searching, with separate finding counts. Items with findings remain available in their group. Clearing search restores focus to the input and keeps the active tab; changing pages resets search and tab selection.
- **Findings:** flat rows use fine separators, quiet metadata, and plain source paths. Warning icons align vertically with their severity labels. Long messages preview three lines and expand on demand; copy actions always include the complete message. The summary scrolls the page to the findings heading, below the sticky header and navigation, and focuses it. Section errors appear before configuration controls. Individual findings retain severity, source, copy action, and a Details disclosure.
- **Accessible states:** current navigation uses `aria-current`; controls have visible focus outlines. Loading announces busy state, failed edits use alerts, and pending switches prevent repeat actions. Narrow-screen navigation, edit controls, and switches have 44px minimum touch targets. Reduced-motion styles remove animations and transitions.

## Do's and Don'ts

Preserve source paths, findings, native disclosures, and keyboard focus when changing layout. Keep status text alongside color. Do not hide findings to produce an empty search result or replace the lifecycle sequence with unrelated cards.
