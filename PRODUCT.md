# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Developers and teams that use more than one AI coding CLI, or expect to change tools without rewriting project instructions and automation.

## Product Purpose

agnostic-ai keeps project setup portable across AI coding tools. It also helps developers keep up with consequential vendor changes in an ecosystem that moves every week. Success means a team can maintain one setup, understand which upstream changes affect it, and adopt useful capabilities without binding the project to one tool.

## Positioning

agnostic-ai combines native multi-target configuration emission with evidence-backed capability tracking. It connects upstream tool changes to the exact support, workaround, gap, or design decision in agnostic-ai.

## Operating Context

Developers author canonical specs in a repository, run `agnostic-ai sync`, review generated native files, and use `sync --check` in CI. Maintainers audit vendor documentation and releases, file confirmed implementation work, and publish one developer briefing with each release on the repository's GitHub Pages site.

## Capabilities and Constraints

- The Go CLI supports 25 registered AI coding targets with different native formats and capability coverage.
- GitHub Pages is a Zola static site built from `docs/site/` and assembled with the browser playground.
- `CHANGELOG.md` records released agnostic-ai behavior. Release briefings reproduce that record and separately cover verified upstream ecosystem changes, including changes agnostic-ai does not support yet.
- Public claims need direct vendor evidence. Proposed shared capabilities are not presented as shipped support.
- Updates are dated Markdown files with a generated archive and RSS feed. The site does not require a CMS or client-side framework.

## Brand Commitments

Use the product name `agnostic-ai`. Keep the direct, technical, plain-language voice already used by the CLI and documentation. Preserve the existing site identity: warm neutral surfaces, rust accent, system sans typography, monospace labels, light and dark themes.

## Evidence on Hand

- Product and capability documentation under `docs/user/`
- Released changes in `CHANGELOG.md`
- Existing GitHub Pages site in `docs/site/`
- Evidence-backed target audit reports under the gitignored `local/target-audit/` working directory
- Confirmed findings and design decisions in labeled GitHub issues

Do not invent customer names, adoption figures, benchmarks, or commercial claims.

## Product Principles

- Keep project configuration portable without hiding native differences.
- Explain the few upstream changes that affect developers now.
- Separate observations, current support, proposed work, and released behavior.
- Link every consequential claim to evidence and an actionable next step.
- Prefer a small dependable release publishing flow over a broad content system.

## Accessibility & Inclusion

The public site must remain usable with keyboard navigation, reduced motion, narrow screens, light and dark system themes, and semantic reading tools.
