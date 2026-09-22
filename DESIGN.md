---
name: agnostic-ai
description: Calm editorial briefings for a fast-moving AI toolchain.
colors:
  rust-signal: "#a8481a"
  warm-paper: "#f7f7f6"
  white-surface: "#ffffff"
  quiet-surface: "#eeeeeb"
  near-black-ink: "#1a1714"
  soft-ink: "#45403a"
  muted-ink: "#6f6960"
  hairline: "#d9d7d1"
  night-paper: "#14120f"
  night-surface: "#1c1917"
  night-ink: "#f2ede5"
  night-signal: "#f0a35e"
typography:
  display:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "clamp(2.8rem, 8vw, 6rem)"
    fontWeight: 780
    lineHeight: 1.08
    letterSpacing: "-0.025em"
  headline:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "clamp(2rem, 5vw, 3.75rem)"
    fontWeight: 700
    lineHeight: 1.08
    letterSpacing: "-0.025em"
  body:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, sans-serif"
    fontSize: "17px"
    fontWeight: 400
    lineHeight: 1.65
  label:
    fontFamily: "ui-monospace, JetBrains Mono, SFMono-Regular, SF Mono, Menlo, Consolas, monospace"
    fontSize: "0.78rem"
    fontWeight: 400
    lineHeight: 1
    letterSpacing: "0.02em"
rounded:
  focus: "3px"
  code: "4px"
  mark: "8px"
  callout: "14px"
  pill: "999px"
spacing:
  xs: "0.4rem"
  sm: "0.8rem"
  md: "1rem"
  lg: "1.5rem"
  xl: "2.5rem"
components:
  signal-tag:
    backgroundColor: "{colors.quiet-surface}"
    textColor: "{colors.muted-ink}"
    rounded: "{rounded.pill}"
    padding: "0 0.5rem"
    height: "1.55rem"
    typography: "{typography.label}"
  evidence-callout:
    backgroundColor: "{colors.white-surface}"
    textColor: "{colors.soft-ink}"
    rounded: "{rounded.callout}"
    padding: "1.35rem 1.4rem"
---

# Design System: agnostic-ai

## Overview

**Creative North Star: "The Toolchain Field Briefing"**

The site feels like a calm technical publication covering an unstable ecosystem. Large, direct headlines establish the editorial point. Monospace metadata, fine rules, and sparse rust signals make evidence and state easy to scan without turning the page into a dashboard.

The system stays visually close to the CLI and documentation: practical, restrained, and explicit. It uses density where evidence needs it, open space around each release thesis, and no decorative material that competes with the content. Release articles use the same shared navigation and editorial components as legacy audit articles, while their content schema omits audit-only counts and markers.

**Key Characteristics:**

- Warm neutral paper in light mode and warm near-black surfaces in dark mode.
- One rust accent for current state, links, focus, and the moving signal rule.
- Large system-sans headlines paired with compact monospace metadata.
- Hairline borders organize content instead of cards and shadows.
- Responsive editorial grids that become one reading column on narrow screens.

## Colors

The palette is warm, low-saturation, and built around one rare rust signal.

### Primary

- **Rust Signal** (`#a8481a`, dark mode `#f0a35e`): active navigation, links, status emphasis, focus rings, selection, and the animated signal rule.

### Neutral

- **Warm Paper** (`#f7f7f6`, dark mode `#14120f`): page ground.
- **White Surface** (`#ffffff`, dark mode `#1c1917`): the latest-edition field and bordered callouts.
- **Near-Black Ink** (`#1a1714`, dark mode `#f2ede5`): headlines and primary text.
- **Soft Ink** (`#45403a`, dark mode `#cec6ba`): body copy.
- **Muted Ink** (`#6f6960`, dark mode `#a3988b`): dates, counts, and secondary navigation.
- **Hairline** (`#d9d7d1`, dark mode `#38312a`): structural borders and dividers.

### Named Rules

**The One Signal Rule.** Rust identifies attention or action. It does not fill large regions or create a second visual hierarchy.

## Typography

**Display Font:** Instrument Serif, self-hosted, one 400 face
**Body Font:** system sans
**Label/Mono Font:** system monospace, preferring JetBrains Mono where installed

**Character:** Page titles carry an editorial serif at weight 400, large and unhurried, so the site states a position rather than shouting a feature. Everything under a title returns to the platform sans, and monospace labels stay operational, keeping paths, dates, and counts aligned.

The display face is the only web font. It is served from `static/assets/fonts/` under the OFL rather than a font CDN, so no page makes a third-party request, and it is scoped to titles alone: body text never waits on a download.

### Hierarchy

- **Display** (400, `clamp(2.8rem, 8vw, 6rem)`, 1.04, Instrument Serif): one first-viewport statement, capped near 15 characters per line.
- **Headline** (400, `clamp(2rem, 5vw, 3.75rem)`, 1.1, Instrument Serif): section, guide, and article titles. A trailing muted span carries the second half of a two-part title.
- **Subhead** (500, `1.3rem` to `1.75rem`, sans): headings inside a guide, where the serif would fight the prose.
- **Title** (700, `1.08rem` to `1.2rem`, 1.08): change summaries and archive entries.
- **Body** (400, `17px`, 1.65): editorial explanation, with article reading width capped at 46rem.
- **Label** (400 or 700, `0.69rem` to `0.78rem`, `0.02em` tracking): dates, edition state, target tags, and audit counts.

### Named Rules

**The Consequence First Rule.** A headline names the developer consequence. Process names and audit mechanics stay in metadata or lower sections.

## Layout

The archive uses a 72rem page container with 1rem minimum side gutters. The first viewport separates a large editorial thesis from a short explanation, then places the latest article beside its critical changes on screens at least 760px wide. Search and target controls sit below the All editions heading, never in the editorial hero. The article uses a 46rem reading column. Below 700px, the five primary links wrap into a compact second header row, grids and filter controls become one column, and audit stats stack.

Documentation uses the same publication shell. Its landing page groups guides from page metadata, so adding a page does not require a second directory list. Guide pages use a sticky task navigation beside the 46rem reading column, add a page outline on wide screens, and collapse navigation into a native disclosure on narrow screens.

Vertical rhythm expands between editorial sections and tightens inside evidence rows. Content order does not change across breakpoints.

## Elevation & Depth

The system has no shadows. Depth comes from tonal surfaces, whitespace, and one-pixel rules. Hover states change color or shift the arrow slightly without lifting containers.

### Named Rules

**The Ruled Surface Rule.** Use borders and background tone to group content. Do not add elevation where a hairline communicates the boundary.

## Shapes

Most containers remain square and merge into the page grid. Pills are reserved for compact target and status labels. The brand mark uses an 8px radius, inline code uses 4px, and evidence callouts use 14px as the only soft container shape.

## Components

### Navigation

- **Style:** compact system-sans links beside a monospace wordmark; the current page uses Rust Signal and 700 weight.
- **Mobile:** keep Home, Updates, Playground, Docs, and GitHub visible in a compact wrapped row.
- **Focus:** a 2px Rust Signal outline with 4px offset.

### Documentation navigation

- **Landing:** flat ruled rows group Start, Workflows, and Reference pages from Zola metadata.
- **Guide pages:** the current guide is visible in a sticky left navigation, with an On this page outline on wide screens.
- **Mobile:** a native disclosure keeps every guide reachable before the article without requiring JavaScript.
- **Source:** each guide links to its canonical Markdown file for quick edits and review.

### Agent handoff

- **Discovery:** the landing first viewport links to agent setup, while the docs index pairs a ready-to-paste prompt with the plain-text instructions.
- **Content:** the prompt and safety contract live in the canonical agent setup guide. Generated text endpoints never become another source to maintain.
- **Presentation:** use one flat bordered prompt field, shared copy behavior, and direct links. Keep the full operational workflow in the guide rather than expanding the landing page.

### Signal tags

- **Style:** 999px pill, 1px Hairline border, Muted Ink monospace text, and 0.5rem inline padding.
- **Status variant:** Rust Signal text on a quiet rust tint with no visible border.

### Update rows

- **Style:** flat rows separated by Hairline borders. Headline color changes to Rust Signal on hover. Descriptions and static target tags make each edition scannable without turning it into a card.
- **Layout:** date, title with description and tags, and finding count form three columns at 760px and one column below it.

### Archive filters

- **Style:** one bordered search field, a native Targets disclosure, and compact square action buttons use the existing paper, hairline, rust, and monospace control language.
- **Behavior:** controls appear only after JavaScript initializes. Target choices use OR, search terms use AND, and explicit Apply keeps the archive calm while state is edited. Result count, applied summary, unknown-target notice, and empty recovery share one polite status region.
- **Fallback:** all rows stay in the HTML and remain readable without JavaScript. Print hides inputs and buttons while keeping the applied summary and visible results.

### Evidence callouts

- **Style:** White Surface, 1px Hairline border, 14px radius, and `1.35rem 1.4rem` padding.
- **Use:** explain state boundaries, provenance, or interpretation. Do not use for ordinary body copy.

### Disclosure rows

- **Style:** square, border-only rows with a Rust Signal plus or minus marker aligned right.
- **Use:** full findings and research limits after the editorial explanation.

## Do's and Don'ts

### Do:

- **Do** lead every edition with one developer consequence and a dated label.
- **Do** use hairlines, whitespace, and monospace metadata to organize dense evidence.
- **Do** preserve light, dark, reduced-motion, keyboard, print, and narrow-screen behavior.
- **Do** keep the visible difference between observation, current support, proposed work, and released behavior.

### Don't:

- **Don't** turn the updates page into an issue dashboard, metric wall, or vendor changelog dump.
- **Don't** introduce extra accent colors, shadows, gradients, or decorative illustration without a new product need.
- **Don't** use pills for structural containers or ordinary navigation.
- **Don't** replace the large editorial first viewport with cards or controls.
