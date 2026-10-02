# Release briefing

Create one release briefing immediately before the release commit. The file,
version bump, and final dated changelog section must land in the same commit
and receive the same signed tag.

## Title and dek

Start the title with `agnostic-ai vX.Y.Z:` and name the main user-visible
outcome. Use concrete nouns, commands, or workflows. A maintenance release
should say what it fixes; do not inflate it into a new feature.

The dek is one or two sentences and at most 220 characters. Name the main
new capabilities and their value. Keep migration details in the upgrade steps.

| Layer | Purpose |
|---|---|
| Title and dek | Why the reader should look at this release |
| `Upgrading from vX.Y.Z` | Installation, migrations, and checks after upgrading |
| `What agnostic-ai changed` | Shipped features, before/after behavior, and examples |
| `What changed in the targets` | Verified vendor changes and their support here |

Make the main value visible in the headings and first sentence of each item.
Do not repeat a claim in several sections. Avoid announcement
preambles, metaphors, rhetorical questions, and a project introduction.
Use "you" for what the reader does and plain words for what the software does.

## Curation

A release briefing answers three questions: how do I move from the previous
version, what can I do now, and what changed in the tools I use? Lead with
upgrade blockers, then group substantial features into a few reader outcomes.
Name the important capabilities without explaining their full contracts.
Keep smaller fixes and secondary details in the linked release notes.

- Mention site work only when it changes how a reader uses the site. Omit
  internal design and visual polish. If most work was on the site, say so.
- Each feature gets a short paragraph or bullet: the benefit, entry point,
  and a limit only when it changes the reader's decision. Link the reference
  for settings, edge cases, and the full contract.
- Never cap a release briefing at a fixed number of bullets or characters.
  Group related fixes into a reader outcome without deleting substantial work.
- Keep examples for the main features readers can adopt. Use the smallest
  working command or config; link optional examples instead of adding another
  block. Avoid turning every fix or vendor setting into a subsection.
- State the win concretely. "Rules land under the configured dir" is a win a
  reader can check; "improved path resolution" is not.
- No audit counts or closing pleasantry. Do not claim that nothing else needs
  action unless all upgrade paths in the release were checked.
- Name who is affected and the actual migration unit, such as one field per
  rule or one hook per project. Never guess elapsed migration time.

## Research inputs

Start with the exact `CHANGELOG.md` section being released and the commits
since the previous tag. Verify feature syntax and limits against the shipped
implementation and reference pages. When revising a historical post, inspect
that release tag so later features do not appear as shipped in it.

Then inspect target audit reports and issues created since the previous
release. Treat them as research leads. Reopen every cited primary vendor
source and verify its date, availability, affected CLI or model, and current
wording. Use vendor documentation, release notes, model-provider announcements,
or released vendor code. Do not use model output, summaries, social posts, or
another vendor's behavior as evidence.

Select target news that changes project setup, safety, portability, model
availability, or a workflow users depend on. Include important target-specific
additions even when no adapter change was needed. Order by consequence:

1. breaking behavior;
2. changed defaults;
3. removals;
4. deprecations;
5. safety or permission changes;
6. substantial additions;
7. routine improvements.

If no verified upstream item meets that bar, say so. Do not invent filler.
Do not label a documentation discovery as a feature launched that day, or
move a later announcement into an earlier release's news.

## Article contract

Use these three sections, with feature or target subsections when useful:

1. `## Upgrading from vX.Y.Z {#actions}` names the immediately previous
   release. Explain installation, required migrations, and regeneration.
   Separate required migrations from optional adoption. Include any
   `requires` or schema pin update, and put required rule/config migrations
   and watcher shutdown before package install hooks can run sync. Preserve a
   deliberate range or minimum requirement; do not tell every project to use
   an exact pin.
2. `## What agnostic-ai changed` makes the main shipped value easy to scan.
   Use brief feature groups and a few adoption examples. End with a link to
   the GitHub release notes. Do not copy the changelog or group changes by
   internal modules.
3. `## What changed in the targets` selects important external changes and
   their practical consequences. Use short bullets or paragraphs. Name the
   affected product and version when
   known, the announcement date or explicitly the documentation discovery
   date, and what this agnostic-ai release can express.

For target news, explain support in plain language: shipped mapping, existing
`x-<target>` extension, native-only setup, unresolved adapter gap, or
experimental feature. A status label alone does not tell a reader what to do.
Use the smallest working example when it helps adoption, and link the primary
source next to its claim. Group tools by theme when they share a consequence.

Never imply that upstream availability means agnostic-ai support. If shipped
work responds to a target change, connect the two without repeating its whole
feature explanation. An announcement date, availability gate, or native-only
limit must be explicit when it affects whether a reader can use the feature.

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
archive_stats = "<main upgrade and feature themes>"
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

1. Check every shipped claim against the exact dated changelog, release tag,
   and implementation. Cover every substantial feature and migration, with
   syntax a reader can use. Do not include unreleased work.
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
