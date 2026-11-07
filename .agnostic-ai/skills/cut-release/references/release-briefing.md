# Release briefing

One post per release at `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md`, in
the release commit with the version bump and the dated changelog section. A
reader learns in one minute what they gain, what they must do, and what changed
in their tools. Aim for about 600 words; a post over 800 is too long.

Follow the `plain-english` rule in `.agnostic-ai/rules/`. Use "you" for what
the reader does. No preamble, project introduction, metaphor, audit count, or
closing line. State each caveat once. Never guess migration time.

## Title and dek

The title is `agnostic-ai vX.Y.Z: <what you can now do>`, at most eight plain
words after the colon. Name the value, not the mechanism: "pick up your chats
where you left them", not "keep files safe when targets are empty". A fix-only
release says what it fixes.

`description` and `dek` are one sentence each, at most 220 characters, naming
the main capabilities. Migration details belong in the actions section.

## Sections, in order

1. `## Highlights {#highlights}`: up to five one-sentence bullets that match
   the changelog `### Highlights`, breaking changes first, each linking its
   reference page where one exists.
2. `## Upgrading from vPREV {#actions}`: only when a reader must act. One
   bullet per required action: who it affects, what to change, where it is
   documented. Close with one line that links the
   [upgrade page](@/docs/cli-reference/upgrades.md#upgrade) for stopping
   watch, installing, and `upgrade --requires`. Do not repeat those steps.
   Skip the section when nothing is required.
3. `## What changed`: at most four `###` subsections, grouped by reader
   outcome, not by module. Each holds a short paragraph or two to three
   bullets: the benefit, the entry point, and a link to the reference. Use at
   most one small example. End with a link to the GitHub release notes for the
   remaining fixes.
4. `## What changed in the targets`: one bullet per target, ordered by
   consequence (breaking, defaults, removals, deprecations, safety, additions).
   Each bullet links its vendor source next to the claim and says what this
   release does about it: shipped mapping, `x-<target>` extension, native-only
   setup, or open gap. End with one sentence that states the verification
   method and date once, such as "Checked on <date> against the linked vendor
   docs and releases; native behavior was not run unless a bullet says so."
   Skip the section when no verified item meets the bar.

Upstream availability is not agnostic-ai support; say which one each claim is.

## Screenshots

Add up to two when the release changes the web interface. Save the file as
`docs/site/static/assets/images/updates/vX.Y.Z-<what>.png` (or `.webp`),
cropped to the relevant panel, at most 1400 px wide and 300 KB. Zola serves
`static/` at the site root, so reference it as:

```markdown
![Conversations page listing recent chats, open ones first](/assets/images/updates/v0.92.0-conversations.png)
```

The alt text says what the picture shows. Take it with any screenshot tool or
the e2e helper `saveScreenshot` in `ui/e2e/helpers.ts`, which writes to
`ui/test-results/screenshots/`. Commit the image with the post; there is no
build step.

## Research

Start from the dated `CHANGELOG.md` section and `git log <prev-tag>..HEAD`.
Verify every command, flag, and limit against the shipped code or reference
page. For target news, open the primary vendor source (docs, release notes,
released code) and keep its date and scope; model output, summaries, and
social posts are not evidence. For a historical post, inspect that release
tag so later features do not appear as shipped in it.

## Front matter

`tests/integration/target_updates_test.go` requires `kind`, `rss_guid`,
`aliases`, `archive_stats`, `targets`, and at least one `[[extra.signals]]`.
Release posts never use `audit_marker`, `report_digest`, `targets_checked`,
`finding_count`, or `clean_count`. `extra.targets` lists the registered IDs
from `docs/site/data/updates.toml` that the post covers substantively; a
target only checked or unaffected does not count, and a general release uses
`[]`. The `.html` alias is the permanent RSS GUID; never change a published
one.

## Skeleton

```markdown
+++
title = "agnostic-ai vX.Y.Z: <what you can now do>"
description = "<one sentence>"
date = YYYY-MM-DDT00:00:00+HH:MM
slug = "YYYY-MM-DD-vX.Y.Z"
aliases = ["updates/YYYY-MM-DD-vX.Y.Z.html"]

[extra]
kind = "release"
version = "vX.Y.Z"
dek = "<one sentence>"
rss_guid = "https://agnostic-ai.org/updates/YYYY-MM-DD-vX.Y.Z.html"
archive_stats = "<theme> · <theme> · <theme>"
targets = ["codex", "pi"]

[[extra.signals]]
status = "shipped"
targets = ["agnostic-ai"]
title = "<highest-consequence change>"
summary = "<what the reader gets>"
+++

## Highlights {#highlights}

- **Breaking:** <change>. See [<page>](@/docs/...).
- <three to five bullets>

## Upgrading from vPREV {#actions}

- **<Who>:** <change this>. See [<page>](@/docs/...).

Then follow the [upgrade page](@/docs/cli-reference/upgrades.md#upgrade).

## What changed

### <Reader outcome>

![<what it shows>](/assets/images/updates/vX.Y.Z-<what>.png)

<Benefit, entry point, link.>

See the [release notes](https://github.com/Chemaclass/agnostic-ai/releases/tag/vX.Y.Z) for the remaining fixes.

## What changed in the targets

- **<Tool> <version>** <changed what> ([source](https://...)). This release <does what>.

Checked on <date> against the linked vendor docs and releases; native behavior was not run.
```

## Before committing

1. Every shipped claim matches the dated changelog and the release tree, with
   syntax a reader can run. No unreleased work.
2. Every target bullet claims no more than its source.
3. Breaking changes, changed defaults, and removals come before additions.
4. `extra.targets` holds every covered ID and nothing else; every link and
   image path resolves.
5. `make site-build site-test` passed on the completed release tree, and the
   built output has the canonical page, the `.html` alias, the archive row,
   and the RSS item.
6. The post is staged with the version bump and changelog; the tag will point
   at that commit.

After pushing, watch the `Release` and `Pages` workflows. Pages runs when the
release commit reaches `main`; `workflow_dispatch` is the manual retry.
