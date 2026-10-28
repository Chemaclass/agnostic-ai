+++
title = "Compare targets"
description = "See side by side what agnostic-ai writes for two or three AI coding tools."
weight = 122

[extra]
group = "Reference"
scripts = ["assets/scripts/compare-targets.js"]
+++


# Compare targets


Pick two AI coding tools, and optionally a third in Target C, to see the files agnostic-ai writes for each, grouped by spec kind. The URL keeps your choice, so you can share it.

For example, with Claude Code and Codex, a shared rule becomes a Claude rule file and a Codex `AGENTS.md` section.

This page shows sample output paths. Before switching tools in an existing project, run [`agnostic-ai compare`](@/docs/cli-reference/inspect.md#compare) to check the fields in your own specs.

{{ <compare_targets /> }}

## Read the paths with care

Paths come from a real sync of one sample spec per kind. A file that holds several kinds, such as `.claude/settings.json`, appears under each. A listed file does not mean the tool reads it: Codex project commands can be written through a legacy opt-in, but Codex does not load them. Open a target page for field limits and optional outputs.
