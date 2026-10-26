+++
title = "Documentation"
description = "Set up your project, share instructions across coding tools, and check the generated files."
sort_by = "weight"
template = "docs/section.html"
page_template = "docs/page.html"
+++

New to agnostic-ai? Follow these three steps in order.

## Your first steps {#choose-your-first-task}

1. [Why agnostic-ai?](@/docs/why-agnostic-ai.md): what it does, and when you do not need it.
2. [Install](@/docs/installation.md): get the `agnostic-ai` command.
3. [Set up your project](@/docs/getting-started.md): write one note, sync it to Claude Code and Codex, and learn the daily loop.

To let your coding agent do steps 2 and 3, [paste the setup prompt](@/docs/agent-setup.md). If the project already has `CLAUDE.md`, `AGENTS.md`, or other tool files, read [Import an existing setup](@/docs/migration.md) before the first sync.

## Find something else

| You want to | Go to |
|---|---|
| Work in a project that is already set up | [Daily use](@/docs/getting-started.md#daily-use) |
| See your configuration and warnings in a browser | [Inspect your configuration](@/docs/inspect-configuration.md) |
| Look up a command, field, or supported tool | [CLI reference](@/docs/cli-reference/_index.md), [Configuration](@/docs/configuration.md), [Supported tools](@/docs/targets/_index.md) |
| Fix a failed command or missing file | [Troubleshooting](@/docs/troubleshooting.md) |

## Terms used in these guides

A **spec** is a source file you edit. A **target** is a coding tool. **Generated files** are what sync writes for each tool, and **drift** means they no longer match their sources.

For coding agents, [agent-setup.txt](https://agnostic-ai.org/agent-setup.txt) holds the setup checklist. [llms.txt](https://agnostic-ai.org/llms.txt) lists the guides, and [llms-full.txt](https://agnostic-ai.org/llms-full.txt) holds the full documentation.

Contributing to agnostic-ai itself? Use the [contributor setup](https://github.com/Chemaclass/agnostic-ai/blob/main/CONTRIBUTING.md).
