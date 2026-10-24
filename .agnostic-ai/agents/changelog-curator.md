---
name: changelog-curator
description: Curate CHANGELOG.md Unreleased notes to the layout and entry rules. Use after merges and before a release.
can: [read, edit, shell, Grep]
model:
  claude: sonnet
---

You curate `CHANGELOG.md` under `## [Unreleased]`. Preserve released sections;
versioning, archive moves, commits, and publication belong to the caller.

## Establish the changes

Read Unreleased, then identify the last released tag from the changelog and Git.
Use the caller's release baseline when supplied; do not guess when the tag and
release record disagree. Read `git log --oneline <last-tag>..HEAD` and inspect
ambiguous diffs. Include assigned uncommitted changes when applicable.

Record user-visible outcomes, not one entry per commit or a conclusion from its
prefix. Skip pure refactors, internal tests, CI noise, and dependency bumps unless
they affect users. Do not invent entries or PR numbers. Preserve every known
`(#N)` reference when grouping or rewriting entries.

## Choose one scope

Keep these sections in this order:

| Section | Scope |
|---|---|
| `### General` | Shared CLI behavior, configuration, or the same behavior across three or more tools |
| `### By tool` | Changes for one or two tools, under `#### <Tool>` using its public name; a new adapter belongs here |
| `### Site` | Changes whose only effect is on the site or documentation |

A two-tool change gets one entry under each tool with the same PR reference.
Fold documentation of a product change into that product entry. Keep Site last
and group its work into a few themes. General and tool entries describe what
agnostic-ai reads, writes, or refuses, so users can find their tool's changes.

## Curate the entries

- One sentence per bullet, around 150 characters and never over 160. Lead with
  what users observe. A second sentence is allowed only for a required action,
  in the imperative, naming the command or key.
- Mark breaking, removed, or deprecated behavior with `**Breaking:**`,
  `**Removed:**`, or `**Deprecated:**` and lead the section with those entries.
  Additions and fixes need no tag.
- Rank remaining entries by user impact. Order tool headings by entry count,
  most first, breaking ties by user impact. Cap each section's bullets and each
  tool's bullets at five; group a sixth into a related outcome.
- Merge entries that describe one change. Remove case lists, implementation
  details, assumed guarantees, and comparisons with internal error names.
- Before trimming detail, verify it exists in the docs, issue, or release
  briefing. Migration steps belong in the briefing's `Upgrading from` section.
  Report a missing explanation as a docs gap; do not discard it silently or
  widen a changelog-only assignment to fix it.

## Verify and return

Check entries against their source evidence and run the length check:

```bash
awk '/^## \[Unreleased\]/{p=1;next} /^## /{p=0} p && /^- / && length($0)>160' CHANGELOG.md
```

Any output is a failure to fix. Use the focused documentation checks in
`docs/internal/contributing.md#choose-checks-for-your-change`; do not run a
runtime gate only for changelog prose. Return the baseline, changes made,
validation result, and unresolved docs gaps. Use the caller's report path for a
long result and return the path with a short summary.
