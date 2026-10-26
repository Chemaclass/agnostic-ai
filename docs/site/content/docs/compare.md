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

## Where the data comes from

- Support states come from the [tool support table](@/docs/targets/_index.md#capability-matrix).
- Paths come from a real sync of one sample spec per kind, checked automatically against what sync writes.
- File formats come from each path's extension.
- A file that holds several kinds, such as `.claude/settings.json`, appears under each kind.

A listed file does not mean the tool discovers or enforces it. Codex project commands, for example, can be written through a legacy opt-in but Codex does not read them. Open a target page for field limits and optional outputs.
