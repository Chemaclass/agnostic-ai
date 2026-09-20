+++
title = "Release and AI tooling updates"
description = "How each release announcement records reader impact, upstream evidence, and target capability classifications."
weight = 100

[extra]
group = "Workflows"
+++

# Release and AI tooling updates


AI coding tools change their project configuration often. Each agnostic-ai release ships a concise announcement: what changes a reader's work, and which verified upstream CLI or model changes affect project setup, safety, and portability.

Each edition is one dated Markdown file under `docs/site/content/updates/`. Zola renders the article, updates the archive, and adds the item to the [RSS feed](https://agnostic-ai.org/updates/feed.xml).

The archive filters editions by target and searches their titles, descriptions, editorial summaries, and highlighted signals. Multiple targets use OR, whitespace-separated search terms use AND. Applied filters stay in the URL, so a filtered archive can be bookmarked or shared. Without JavaScript, every edition stays in date order and readable.

## What each release announcement tells you

Each announcement has two records:

- **What changed in agnostic-ai.** Two or three reader consequences. The GitHub release notes remain the complete record.
- **Upstream CLI and model news.** Selected external changes with primary sources, user impact, and the current agnostic-ai support state.

Breaking behavior, default changes, removals, and deprecations come first. Safety changes and large additions follow. Upstream availability never implies that agnostic-ai supports the feature.

Target names do not prove shared behavior. The audit checks each target's project scope, lifecycle, defaults, and native file shape before it calls two features equivalent.

## How observations are classified

| Disposition | Meaning |
|---|---|
| `adapter-gap` | The shared spec can represent the behavior, but one adapter does not emit or import it yet. |
| `spec-candidate` | Several targets share a verified project intent that the current spec cannot preserve. This needs a design decision before implementation. |
| `target-extension` | The capability is useful but vendor-specific. An `x-<target>` extension is the honest fit. |
| `watch` | The change matters, but the evidence or cross-target shape is not strong enough for a product decision. |

A proposed improvement is not shipped support: design candidates stay out of automated fixes until their schema and scope are approved.

## Know which record to trust

The [release updates](https://agnostic-ai.org/updates/) summarize the release and selected upstream observations. They can describe a vendor feature before agnostic-ai supports it, and they label that state.

[`targets.md`](@/docs/targets/_index.md) records support in the current code: native paths, capability limits, and opt-in settings.

[`CHANGELOG.md`](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md) records what agnostic-ai released.

Observation, support, release. Keep those three states separate.

Target audit reports and issues are research inputs: they hold the vendor evidence, target comparison, representation tests, and next actions. The `target-audit` skill does not publish articles or open publication PRs.

## Publishing workflow

The `cut-release` skill creates one `YYYY-MM-DD-vX.Y.Z.md` article immediately before the release commit. The article, version bump, and dated changelog section share one commit and tag. Its frontmatter carries the release identity, summary signals, permanent RSS GUID, `.html` compatibility alias, and article-level target IDs. It needs no audit counts, audit marker, or report digest.

To prepare a release announcement:

1. Finalize the dated release section in `CHANGELOG.md`.
2. Create `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` from the release announcement contract in `.agnostic-ai/skills/cut-release/references/release-briefing.md`.
3. Summarize the two or three consequences a reader needs to know and link the complete GitHub release notes. Add only verified upstream news to its separate section.
4. Set `extra.targets` to the registered IDs covered substantively in the article. Use `[]` for a general edition. A checked or clean target does not count as coverage.
5. Set both `aliases` and `rss_guid` to the permanent `.html` address, then run `make site-build site-test` with Zola 0.22.0 and Node 22.
6. Review `_site/updates/`, `_site/updates/feed.xml`, and `_site/sitemap.xml`. Do not commit `_site/`.
7. Include the article in the release commit and tag.

Pushing the release commit to `main` runs the Pages workflow. The same workflow supports `workflow_dispatch` on `main` for a manual recovery run. The release process watches both the Release and Pages workflows.

Legacy audit articles keep their original directory URL, `.html` alias, RSS GUID, audit marker, report digest, counts, and capability markers. Do not migrate their metadata to the release schema.

Where the rest of the site lives:

- `docs/site/data/updates.toml`: stable target IDs and display labels. Keep historical IDs there when a target is renamed or removed, so published article metadata keeps its meaning.
- `docs/site/data/landing.toml`: landing copy.
- `docs/site/templates/`: shared navigation and page structure.
- `docs/site/static/assets/`: CSS and JavaScript.

A normal release changes only one Markdown article.
