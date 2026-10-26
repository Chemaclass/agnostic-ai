+++
title = "Follow releases and tool updates"
description = "Find release announcements and tell current support from dated tool news."
weight = 100

[extra]
nav_title = "Releases and updates"
group = "Reference"
+++

# Release and AI tooling updates

Read [release updates](https://agnostic-ai.org/updates/) for agnostic-ai changes and selected coding tool news, or follow the [RSS feed](https://agnostic-ai.org/updates/feed.xml).

## What each release announcement tells you

Each edition lists the changes you need to act on, breaking changes first. Tool news links to primary sources and says whether agnostic-ai supports the feature yet. You can filter the archive by tool or search it; the URL keeps your view.

## Know which record to trust

| You need | Read |
|---|---|
| What the current code supports and its limits | [Supported tools](@/docs/targets/_index.md) |
| What shipped in an agnostic-ai release | [Changelog](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md) |
| The context and selected tool news at a release date | [Dated updates](https://agnostic-ai.org/updates/) |

A feature a coding tool announces is not automatically supported by agnostic-ai. Dated announcements stay as written; use the maintained guides for your current setup.

## How observations are classified

Research notes label each observation as an adapter gap, a possible shared feature, a tool-specific extension, or a change still being watched. The labels describe proposed work, not shipped support. See the [contributor audit instructions](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/target-audit/SKILL.md).

## Publishing workflow

Maintainers publish the release article with the version and changelog. The [release instructions](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/cut-release/SKILL.md) cover the article format, verification, and deployment.
