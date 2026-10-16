+++
title = "Documentation"
description = "Set up your project, share instructions across coding tools, and check the generated files."
sort_by = "weight"
template = "docs/section.html"
page_template = "docs/page.html"

[extra.demo]
title = "Watch it run."
muted_title = "The talk demo."
body = "Twenty minutes from one spec to files for every tool."
video_id = "uEG6ITlqyHU"
video_title = "One spec, every AI CLI"
play_label = "Play the demo video, 20 minutes"
upload_date = "2026-09-17"
duration = "PT19M55S"
+++

New to coding agents? Start with a small setup: project notes that explain how to work here, and one convention the tool should follow.

## Choose your first task

| You want to | Start here |
|---|---|
| Understand instructions, rules, skills, and the other pieces | [Start here: understand the setup](@/docs/why-agnostic-ai.md) |
| Build one shared Claude Code and Codex setup | [Getting started](@/docs/getting-started.md) |
| Keep the tool files you already use | [Import existing configuration](@/docs/migration.md) |
| Ask a coding agent to do the setup | [Assisted setup](@/docs/agent-setup.md) |
| Work in an already configured project | [Daily use](@/docs/getting-started.md#daily-use) |
| Look up a command, field, or supported tool | [CLI reference](@/docs/cli-reference/_index.md), [Configuration](@/docs/configuration.md), [Supported tools](@/docs/targets/_index.md) |
| Fix a failed command or missing file | [Troubleshooting](@/docs/troubleshooting.md) |

## Terms used in these guides

A **spec** is an editable source file under `.agnostic-ai/`. A **target** is a selected coding tool. **Generated files** are the native files `sync` writes for those tools. **Drift** means those files no longer match what the sources produce.

For agents, [agent-setup.txt](https://agnostic-ai.org/agent-setup.txt) contains the setup checklist. [llms.txt](https://agnostic-ai.org/llms.txt) lists guides; [llms-full.txt](https://agnostic-ai.org/llms-full.txt) includes the full documentation, including the detailed sections.

Contributing to agnostic-ai itself? Use the [contributor setup](https://github.com/Chemaclass/agnostic-ai/blob/main/CONTRIBUTING.md).
