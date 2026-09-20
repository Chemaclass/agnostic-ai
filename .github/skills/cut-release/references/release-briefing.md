# Release briefing

Create one release briefing immediately before the release commit. The file,
version bump, and final dated changelog section must land in the same commit
and receive the same signed tag.

## Title and dek

The title carries a `agnostic-ai vX.Y.Z:` prefix, because this is an installed
CLI and "which version has the fix" is a real question. The prefix is overhead,
so the words after the colon have to work harder.

- Name a thing that now exists or now behaves differently. Never name a concept
  the release invented. "The memory boundary" fails: the dek has to define it
  before it means anything, and a reader scanning the index cannot act on it.
- One outcome after the colon, or a list of concrete nouns. Never two
  abstractions joined by "and".
- Keep the website out of the title. The site section states that it changes
  nothing in the tool, so a headline spent on it is a headline wasted.
- When the release is maintenance, title it as maintenance and lead with the
  change that will surprise someone. A four-fix release titled as four fixes
  keeps its credibility; one inflated into a design thesis loses it.

The dek is one or two sentences and at most 220 characters. Name the change,
then its consequence. It is not an inventory. A reader who needs every detail
can open the GitHub release notes.

Each fact appears in exactly one layer:

| Layer | Owns |
|---|---|
| `dek` | the single highest-consequence change, and what it means |
| `## What to do` | every imperative and command in the article |
| `## What changed` | the two or three changes most readers need to know |
| `## Upstream` | what changed outside this project |

A dek that restates `## What to do`, or a change bullet that repeats the dek,
is the same fact told twice. That repetition, not sentence length, makes an
announcement feel like generated prose.

Banned openers, no exceptions:

- The release as the subject of a claim: "This release draws the line", "This
  release closes the loop", "This release is all about", "This release focuses
  on". The release may be the subject only when the predicate is an inventory,
  as in "This release includes numerous bug fixes".
- Announcement throat-clearing: "We are excited to announce", "We are thrilled",
  "We've been busy", "Without further ado".
- A rhetorical question as the first sentence.
- A "what is this project" paragraph.
- Metaphor verbs for state changes: draws the line, closes the loop, unlocks,
  levels up, supercharges.

Use "you" for what the reader does. Use "we" only for a decision a person made,
never to describe what the software does.

## Curation

A release announcement is curated, never transcribed. Before writing, decide
which two or three product changes alter a reader's work. Lead with those. The
GitHub release notes remain the exhaustive record.

Two rules follow from that:

1. Mention site work only when it changes how a reader uses the site. One bullet
   is enough. Omit internal design and visual polish.
2. Each product bullet states an observable effect, not the mechanism. Name the
   flag, path, or file a user will see. The issue and PR hold the detail.

If a release is mostly site work, say so plainly in the dek rather than padding
the product sections to hide it.

Six more rules keep a briefing scannable:

1. Open the article with `## What to do {#actions}`: a numbered list of what a
   reader upgrading must actually do, each item one bounded action, ordered by
   who is affected most. Close it with a line stating that nothing else needs
   action. Omit the whole section when there is genuinely nothing to do, and
   never pad it to look thorough.
2. Rank every list by consequence and cap the whole article at three change
   bullets. More than three means the release has not been curated.
3. Use one sentence per bullet, about 120 characters. A blog post tells readers
   whether to upgrade. The GitHub release notes tell them everything that moved.
4. State the win concretely. "Rules land under the configured dir" is a win a
   reader can check; "improved path resolution" is not.
5. No preamble and no closing pleasantry. The dek is the summary; each section
   starts with substance.
6. Give every `## What to do` item an effort estimate in concrete units, in
   parentheses at the end: `(one line in your config)`, `(a rename per spec)`,
   `(about 10 minutes on a large repo)`. "A small change" and "some work" read
   the same to a reader deciding whether to upgrade now or after lunch, so they
   do not count. Estimate the work the reader does, not the work the release
   did. Omit it only when the action is literally "run `agnostic-ai sync`", and
   never guess a number for a migration nobody has run.

## Research inputs

Start with the exact `CHANGELOG.md` section being released. Then inspect target
audit reports and issues created since the previous release. Treat them as
research leads. Reopen every cited primary vendor source and verify its date,
availability, affected CLI or model, and current wording before publication.
Use vendor documentation, release notes, or model-provider announcements. Do
not use model output, summaries, social posts, or another vendor's behavior as
evidence.

Select upstream news that changes project setup, safety, portability, model
availability, or a workflow users depend on. Order the article by consequence:

1. breaking behavior;
2. changed defaults;
3. removals;
4. deprecations;
5. safety or permission changes;
6. substantial additions;
7. routine improvements.

If no verified upstream item meets that bar, say so in the upstream section.
Do not invent filler.

## Article contract

Use these sections only when they contain useful information:

1. `What to do` for upgrade actions.
2. `What changed` for the two or three reader consequences.
3. `Upstream CLI and model news` for verified external changes.

End `What changed` with a short link to the GitHub release notes. Do not repeat
the changelog, group changes by internal category, or publish a Site section
unless site work changes a reader's workflow. The upstream section describes
external changes only, as a list of one item per line, at most three lines. Each
line carries the affected product and version,
the source date, the user consequence, the state here, and the source links:

```
- **<product> <version>** (<date>): <what changed and what it means for a
  reader>. <One sentence on what this project does about it, when relevant.>
  State: <supported | target extension | workaround | adapter gap | design
  candidate | watch>. [<source>](<url>)
```

Group by theme when there are more than five, for instance several CLIs that
all shipped security fixes in the same week. Vendor-side specifics that a reader
would only act on through the vendor's own page stay on that page, behind the
link. Use `supported`, `target extension`,
`workaround`, `adapter gap`, `design candidate`, or `watch` as appropriate.

Never imply that upstream availability means agnostic-ai support. If a shipped
changelog entry responds to the upstream change, link the two in prose without
duplicating it as shipped work.

## File and metadata contract

Use `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md`:

```toml
+++
title = "agnostic-ai vX.Y.Z: <reader consequence>"
description = "<plain summary of shipped work and selected upstream news>"
date = YYYY-MM-DDT00:00:00+HH:MM
slug = "YYYY-MM-DD-vX.Y.Z"
aliases = ["updates/YYYY-MM-DD-vX.Y.Z.html"]

[extra]
kind = "release"
version = "vX.Y.Z"
dek = "<one-paragraph editorial summary>"
rss_guid = "https://agnostic-ai.org/updates/YYYY-MM-DD-vX.Y.Z.html"
archive_stats = "<N product changes · N site updates · N upstream notes>"
targets = ["claude", "codex"]

[[extra.signals]]
status = "shipped"
targets = ["agnostic-ai"]
title = "<highest-consequence shipped change>"
summary = "<specific reader outcome>"
+++
```

Add one or two signals that summarize the highest-consequence items. Signals
can point to shipped work or upstream news, but their status and summary must
make the distinction explicit. Release posts do not use `audit_marker`,
`report_digest`, `targets_checked`, `finding_count`, or `clean_count`.

Set article-level `targets` to the exact registered target IDs covered
substantively by the briefing. A target mentioned only as checked, clean, or
unaffected does not count as coverage. Use an empty list when the edition is
general to agnostic-ai. Do not derive these IDs from the display labels in
signals. Before publishing, confirm every ID exists in
`docs/site/data/updates.toml`, with no duplicates.

The canonical URL is `/updates/YYYY-MM-DD-vX.Y.Z/`. The `.html` alias is the
permanent RSS GUID. Never reuse or change a published GUID. Zola generates the
article, archive row, RSS item, alias, and sitemap route from this one Markdown
file.

Legacy audit articles have a different contract. Never change their paths,
aliases, GUIDs, audit markers, report digests, counts, or capability markers.

## Pre-commit checks

Before committing:

1. Check every blog claim against the new dated changelog section. The blog
   should summarize it, not copy it.
2. Open every upstream source and confirm that the article makes no broader
   claim than the source.
3. Confirm high-impact changes appear before additions in both the changelog
   and article narrative.
4. Confirm `extra.targets` names every substantively covered target and no
   target mentioned only as checked or clean.
5. Run `make site-build site-test`.
6. Check that the new canonical article, `.html` alias, archive entry, and RSS
   GUID exist in the built output.
7. Stage the briefing with the version and changelog. Confirm the release tag
   will point at that exact commit.

After pushing, watch both the `Release` and `Pages` workflows. Pages runs
automatically when the release commit reaches `main`. The workflow also keeps
`workflow_dispatch` for a manual recovery run on `main` if the automatic run
is absent or needs a safe retry.
