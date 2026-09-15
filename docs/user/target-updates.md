# AI tooling updates

[All docs](../README.md) · [Read the updates](https://chemaclass.github.io/agnostic-ai/updates/) · [Targets](targets.md) · [Changelog](../../CHANGELOG.md)

AI coding tools change their project configuration every week. agnostic-ai publishes a short briefing for developers who need to understand which changes affect project setup, safety, and portability.

Each edition lives on the repository's GitHub Pages site. An [RSS feed](https://chemaclass.github.io/agnostic-ai/updates/feed.xml) announces new editions without turning an issue into a newsletter.

## What each edition tells you

Every update answers four questions:

- **What changed?** The documented vendor behavior and affected targets.
- **Why does it matter?** The workflow, safety, or compatibility impact.
- **Where is agnostic-ai now?** Shipped support, a target extension, a lossy workaround, or no representation yet.
- **What happens next?** The smallest adapter fix, design decision, target extension, or research check.

The lead section contains only consequential changes. Source evidence, target comparisons, clean targets, and research limits remain available lower in the article.

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

The [weekly updates](https://chemaclass.github.io/agnostic-ai/updates/) record vendor observations and proposed work. They can describe a feature before agnostic-ai supports it.

[`targets.md`](targets.md) records support in the current code, including native paths, capability limits, and opt-in settings.

[`CHANGELOG.md`](../../CHANGELOG.md) records what agnostic-ai released.

Observation, support, release. Keep those three states separate.

## Publishing workflow

The `target-audit` skill writes a dated static article, updates the archive and RSS feed, and opens a documentation PR. Merging that PR publishes the edition through the existing Pages workflow. Confirmed drift and design decisions keep their own GitHub issues because they are actionable work, not editorial delivery.
