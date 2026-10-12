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

Keep one editable set of instructions in `.agnostic-ai/`. Run `agnostic-ai sync` to write the files your coding tools read.

## Choose your first task

| You want to | Start here |
|---|---|
| Ask a coding agent to set up the project | [Agent setup](@/docs/agent-setup.md) |
| Set up the project yourself | [Getting started](@/docs/getting-started.md) |
| Keep an existing tool setup | [Migration](@/docs/migration.md) |
| Work in a project that already uses agnostic-ai | Run `agnostic-ai sync` from its root, then follow [Daily use](@/docs/getting-started.md#daily-use) |
| Find a command, field, or tool's output path | [CLI reference](@/docs/cli-reference/_index.md), [Configuration](@/docs/configuration.md), or [Targets](@/docs/targets/_index.md) |
| Fix a failed command or missing file | [Troubleshooting](@/docs/troubleshooting.md) |
| Contribute to agnostic-ai itself | [Contributor setup](https://github.com/Chemaclass/agnostic-ai/blob/main/CONTRIBUTING.md) |

## Terms used in these guides

A **spec** is a source file that describes instructions, a task, or a setting. A **target** is a coding tool selected in `agnostic-ai.yaml`. **Generated files** are the files sync writes for those tools. **Drift** means those files differ from what the current sources would generate.

Edit the sources, preview with `agnostic-ai sync --plan`, then sync. Check the result with `agnostic-ai sync --check`.

For coding agents, [agent-setup.txt](https://agnostic-ai.org/agent-setup.txt) contains the setup checklist. [llms.txt](https://agnostic-ai.org/llms.txt) links to guides by task; [llms-full.txt](https://agnostic-ai.org/llms-full.txt) provides the full user documentation in one fetch.
