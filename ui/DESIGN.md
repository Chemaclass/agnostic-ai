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

System sans serves headings, controls, and body text (15px, 1.5 line height). Page titles use 30px, weight 780, and tight tracking. System monospace distinguishes keys, paths, code, and JSON controls. Descriptions use 13px; source paths use 12px and wrap within their row.

## Layout

The viewport shell keeps a header and a 244px sidebar beside the scrolling content. Pages have a 1320px maximum width. The sidebar includes section descriptions and configured tools with their generated-file status. At 900px and below, section links become one horizontally scrolling navigation row and keep the current page in view. Descriptions hide; a Configured tools button opens a bounded, scrolling disclosure beside the links. At 760px and below, content uses 16px side gutters and fields, values, metadata, and support groups stack.

## Elevation & Depth

One-pixel borders, warm surface tones, and whitespace define groups. The interface uses no shadows.

## Shapes

Small rounded corners soften controls and metadata. Larger corners group content; pills serve switches and compact badges. Source rows stay aligned within their containing list.

## Components

- **Source rows:** show name, description, then muted monospace path. Layer, scope, edit action, and switches stay alongside the row on wide screens and below its text on narrow screens.
- **Settings:** configured fields place label and description beside values and their source path. Lists use chips, booleans use switches, and other values use text. Native values group by tool. Unconfigured fields live in a disclosure.
- **Lifecycle:** an ordered list forms a numbered vertical timeline. Each stage has a native disclosure that starts closed, with its title, hook count, and description visible. Stages with hooks receive rust markers. Expand all and Collapse all sit beside Hooks. Search opens matching stages; linked hooks and findings open their stage. Tool icons identify stage support inside each disclosure.
- **Generated files:** the sidebar's Configured tools area sums up which tools match the source, need sync, or were not checked, with the check time and Refresh. Each tool logo has its name, state badge, and a disclosure for differing files. A failed refresh or a snapshot never reads as a fresh check.
- **Search:** each page has a labeled search input and live result count. Search matches names, paths, descriptions, and values; matching unconfigured fields open automatically. Items with findings remain visible. Clearing search restores focus to the input; changing pages resets search.
- **Findings:** flat rows use fine separators, quiet metadata, and plain source paths. Warning icons align vertically with their severity labels. Long messages preview three lines and expand on demand; copy actions always include the complete message. The summary scrolls only the main content and focuses the findings heading, keeping mobile navigation in place. Section errors appear before configuration controls. Individual findings retain severity, source, copy action, and a Details disclosure.
- **Accessible states:** current navigation uses `aria-current`; controls have visible focus outlines. Loading announces busy state, failed edits use alerts, and pending switches prevent repeat actions. Narrow-screen navigation, edit controls, and switches have 44px minimum touch targets. Reduced-motion styles remove animations and transitions.

## Do's and Don'ts

Preserve source paths, findings, native disclosures, and keyboard focus when changing layout. Keep status text alongside color. Do not hide findings to produce an empty search result or replace the lifecycle sequence with unrelated cards.
