+++
title = "Release and AI tooling updates"
description = "Understand release briefings, upstream evidence, and target capability classifications."
weight = 100

[extra]
group = "Workflows"
+++

# Release and AI tooling updates


AI coding tools change their project configuration often. Each agnostic-ai release includes a briefing that explains what the project shipped and which verified upstream CLI or model changes affect project setup, safety, and portability.

Each edition starts as one dated Markdown file under `docs/site/content/updates/`. Zola renders the article, updates the archive, and adds the RSS item. An [RSS feed](https://agnostic-ai.org/updates/feed.xml) announces new editions without turning an issue into a newsletter.

The archive can filter whole editions by target and search their titles, descriptions, editorial summaries, and highlighted signals. Multiple targets use OR, while whitespace-separated search terms use AND. Applied filters stay in the URL so a filtered archive can be bookmarked or shared. Without JavaScript, every edition remains in date order and readable.

## What each release briefing tells you

Every release briefing has two explicit records:

- **Shipped in agnostic-ai.** This section reproduces the release's dated `CHANGELOG.md` section exactly.
- **Upstream CLI and model news.** This section contains selected external changes with primary sources, user impact, and the current agnostic-ai support state.

Breaking behavior, default changes, removals, and deprecations come first. Safety changes and substantial additions follow. Upstream availability never implies that agnostic-ai supports the feature.

Target names do not prove shared behavior. The audit checks each target's project scope, lifecycle, defaults, and native file shape before it calls two features equivalent.

## How observations are classified

| Disposition | Meaning |
|---|---|
| `adapter-gap` | The shared spec can represent the behavior, but one adapter does not emit or import it yet. |
| `spec-candidate` | Several targets share a verified project intent that the current spec cannot preserve. This needs a design decision before implementation. |
| `target-extension` | The capability is useful but vendor-specific. An `x-<target>` extension is the honest fit. |
| `watch` | The change matters, but the evidence or cross-target shape is not strong enough for a product decision. |

A proposed improvement is not shipped support. Design candidates stay out of automated fixes until their schema and scope are approved.

## Know which record to trust

The [release updates](https://agnostic-ai.org/updates/) combine shipped release notes with selected upstream observations. They can describe a vendor feature before agnostic-ai supports it, but label that state explicitly.

[`targets.md`](@/docs/targets.md) records support in the current code, including native paths, capability limits, and opt-in settings.

[`CHANGELOG.md`](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md) records what agnostic-ai released.

Observation, support, release. Keep those three states separate.

Target audit reports and issues are research inputs. They retain the detailed vendor evidence, target comparison, representation tests, and next actions. The `target-audit` skill does not publish articles or open publication PRs.

## Publishing workflow

The `cut-release` skill creates one `YYYY-MM-DD-vX.Y.Z.md` article immediately before the release commit. The article, version bump, and dated changelog section share one commit and tag. Its frontmatter carries the release identity, summary signals, permanent RSS GUID, `.html` compatibility alias, and article-level target IDs. It does not need audit counts, an audit marker, or a report digest.

To prepare a release briefing:

1. Finalize the dated release section in `CHANGELOG.md`.
2. Create `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` from the release briefing contract in `.agnostic-ai/skills/cut-release/references/release-briefing.md`.
3. Copy the dated changelog section exactly into `Shipped in agnostic-ai vX.Y.Z`. Add only verified upstream news to its separate section.
4. Set `extra.targets` to the registered IDs covered substantively in the article. Use `[]` for a general edition. A checked or clean target does not count as coverage.
5. Set both `aliases` and `rss_guid` to the permanent `.html` address, then run `make site-build site-test` with Zola 0.22.0 and Node 22.
6. Review `_site/updates/`, `_site/updates/feed.xml`, and `_site/sitemap.xml`. Do not commit `_site/`.
7. Include the article in the release commit and tag.

Pushing the release commit to `main` automatically runs the Pages workflow. The same workflow supports `workflow_dispatch` on `main` for a manual recovery run. The release process watches both the Release and Pages workflows.

Legacy audit articles keep their original directory URL, `.html` alias, RSS GUID, audit marker, report digest, counts, and capability markers. Do not migrate their metadata to the release schema.

The stable target ID and display-label vocabulary lives in `docs/site/data/updates.toml`. Keep historical IDs there if a target is renamed or removed, so published article metadata keeps its meaning. The landing copy lives in `docs/site/data/landing.toml`. Shared navigation and page structure live in `docs/site/templates/`. CSS and JavaScript live in `docs/site/static/assets/`. A normal release changes only one Markdown article.
