+++
title = "Compare targets"
description = "See side by side what agnostic-ai writes for two or three AI coding tools."
weight = 122

[extra]
group = "Reference"
scripts = ["assets/scripts/compare-targets.js"]
+++


# Compare targets


Pick two tools (or a third in Target C) to compare generated files by spec kind. The URL keeps your selection. With Claude Code and Codex, a shared rule becomes a Claude rule file and a Codex `AGENTS.md` section.

These are sample paths. Before switching tools, run [`agnostic-ai compare`](@/docs/cli-reference/inspect.md#compare) on your own specs.

{{ <compare_targets /> }}

## Read the paths with care

- Paths come from syncing one sample spec per kind. A file that holds several kinds, such as `.claude/settings.json`, appears under each.
- Check that the tool loads each optional output. Codex can receive legacy project command files but does not load them.
- Target pages list field limits and optional outputs.
