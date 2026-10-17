+++
title = "Follow releases and tool updates"
description = "Find release announcements and tell current support from dated tool news."
weight = 100

[extra]
group = "Reference"
+++

# Release and AI tooling updates

Read [release updates](https://agnostic-ai.org/updates/) for changes to agnostic-ai and selected news from coding tools. Subscribe to the [RSS feed](https://agnostic-ai.org/updates/feed.xml) to follow new editions.

## What each release announcement tells you

Each edition explains the changes a user needs to act on, with breaking changes first. Tool news links to primary sources and says whether agnostic-ai supports the feature yet. Filter the archive by tool, or search it; the URL keeps your chosen view.

## Know which record to trust

| You need | Read |
|---|---|
| What the current code supports and its limits | [Supported tools](@/docs/targets/_index.md) |
| What shipped in an agnostic-ai release | [Changelog](https://github.com/Chemaclass/agnostic-ai/blob/main/CHANGELOG.md) |
| The context and selected tool news at a release date | [Dated updates](https://agnostic-ai.org/updates/) |

A feature announced by a coding tool is not automatically supported by agnostic-ai. Dated announcements remain historical; use the maintained guides for your current setup.

## How observations are classified

Research notes distinguish an adapter gap, a possible shared feature, a tool-specific extension, and a change still being watched. These labels describe proposed work, not shipped support. See the [contributor audit instructions](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/target-audit/SKILL.md).

## Publishing workflow

Maintainers publish the release article with the version and changelog update. The [release instructions](https://github.com/Chemaclass/agnostic-ai/blob/main/.agnostic-ai/skills/cut-release/SKILL.md) cover the article format, verification, and deployment.
