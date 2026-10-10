+++
title = "Release and AI tooling updates"
description = "What each release announcement tells you, and how maintainers publish one."
weight = 100

[extra]
group = "Workflows"
+++

# Release and AI tooling updates

AI coding tools change their project configuration often. Each agnostic-ai release has a short announcement. It says what changes for you, and which verified changes in the tools and models affect your setup, safety, and portability.

Each edition is one dated Markdown file under `docs/site/content/updates/`. Zola builds the article, adds it to the archive, and adds it to the [RSS feed](https://agnostic-ai.org/updates/feed.xml).

The archive filters editions by target and searches titles, descriptions, summaries, and highlights. Pick several targets to see editions that match any of them. Search terms separated by spaces must all match. Filters stay in the URL, so you can bookmark or share a view. Without JavaScript, every edition is listed by date.

## What each release announcement tells you

Each announcement has two parts:

- **What changed in agnostic-ai.** Two or three effects on you. The GitHub release notes have the full list.
- **News from the tools and models.** Selected changes outside agnostic-ai, with primary sources, what they mean for you, and whether agnostic-ai supports them yet.

Breaking changes, changed defaults, removals, and deprecations come first. Safety changes and large additions follow. A feature a tool has may not be supported by agnostic-ai yet.

The same name does not mean the same behavior. Before calling two features equivalent, the audit compares which project files each feature affects, when it runs, its defaults, and its file format.

## How observations are classified

| Disposition | Meaning |
|---|---|
| `adapter-gap` | The shared spec can describe the behavior, but one adapter does not write or import it yet. |
| `spec-candidate` | Several tools share a verified project setting that the current spec cannot hold. It needs a design decision first. |
| `target-extension` | Useful, but specific to one tool. It fits an `x-<target>` extension. |
| `watch` | The change matters, but the evidence is too thin for a product decision. |

A proposed improvement is not shipped support. Automated fixes skip design candidates until their schema and scope are approved.

## Know which record to trust

- The [release updates](https://agnostic-ai.org/updates/) summarize the release and selected news from the tools. They can describe a tool feature before agnostic-ai supports it, and they say so.
- [`targets.md`](@/docs/targets/_index.md) records what the current code supports: file paths, limits, and opt-in settings.
- [`CHANGELOG.md`](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md) records what agnostic-ai released.

A change seen in a tool, support in agnostic-ai, and a release are three separate states.

Target audit reports and issues are research notes. They hold the evidence from the tool's docs, the comparison between tools, and next steps. The `target-audit` skill does not publish articles or open pull requests for them.

## Publishing workflow

The `cut-release` skill creates one `YYYY-MM-DD-vX.Y.Z.md` article right before the release commit. The article, version bump, and dated changelog section go in one commit and tag. Its front matter holds the release identity, summary highlights, permanent RSS GUID, `.html` alias, and target IDs. It needs no audit counts, audit marker, or report digest.

To prepare a release announcement:

1. Finalize the dated release section in `CHANGELOG.md`.
2. Create `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` using the release announcement instructions in `.agnostic-ai/skills/cut-release/references/release-briefing.md`.
3. Summarize the two or three effects a reader needs and link the full GitHub release notes. Put only verified news from the tools in its own section.
4. Set `extra.targets` to the registered IDs the article covers in substance. Use `[]` for a general edition. A target that was checked and found nothing to report does not count.
5. Set both `aliases` and `rss_guid` to the permanent `.html` address, then run `make site-build site-test` with Zola 0.23.6 and Node 22.
6. Review `_site/updates/`, `_site/updates/feed.xml`, and `_site/sitemap.xml`. Do not commit `_site/`.
7. Include the article in the release commit and tag.

Pushing the release commit to `main` runs the Pages workflow. To recover a failed run, start it with `workflow_dispatch` on `main`. The release process watches both the Release and Pages workflows.

Older audit articles keep their original URL, `.html` alias, RSS GUID, audit marker, report digest, counts, and capability markers. Do not change their metadata to the release format.

Other files that shape the site:

- `docs/site/data/updates.toml`: target IDs and display labels. Keep old IDs there when a target is renamed or removed, so published articles keep working.
- `docs/site/data/landing.toml`: landing copy.
- `docs/site/templates/`: navigation and page layout.
- `docs/site/static/assets/`: CSS and JavaScript.

A normal release changes only one Markdown article.
