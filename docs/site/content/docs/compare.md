+++
title = "Compare targets"
description = "See side by side what agnostic-ai writes for two or three AI coding tools."
weight = 122

[extra]
group = "Reference"
scripts = ["assets/scripts/compare-targets.js"]
+++


# Compare targets


Pick two coding tools, or add a third in Target C, to compare generated files by spec kind. Share the URL to keep your selection.

For example, with Claude Code and Codex, a shared rule becomes a Claude rule file and a Codex `AGENTS.md` section.

This page shows sample output paths. Before switching tools in an existing project, run [`agnostic-ai compare`](@/docs/cli-reference/inspect.md#compare) to check the fields in your own specs.

{{ <compare_targets /> }}

## Read the paths with care

Paths come from syncing one sample spec per kind. Files that hold several kinds, such as `.claude/settings.json`, appear under each. Check whether the tool loads each optional output. Codex, for example, can receive legacy project command files but does not load them. Check each target page for field limits and optional outputs.
