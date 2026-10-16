+++
title = "Compare targets"
description = "See side by side what agnostic-ai writes for two or three AI coding tools."
weight = 122

[extra]
group = "Reference"
scripts = ["assets/scripts/compare-targets.js"]
+++


# Compare targets


Pick two AI coding tools to compare their generated files, grouped by spec kind (rules, skills, hooks, and the other source types). Add a third in Target C if needed. The URL keeps your choice so you can share it with teammates.

For a first comparison, choose Claude Code and Codex. A shared rule uses a Claude rule file and a Codex `AGENTS.md` section; a skill uses each tool's skill folder.

Before switching tools in an existing project, run [`agnostic-ai compare`](@/docs/cli-reference/inspect.md#compare) too. It checks the fields in your own specs; this page shows sample output paths.

{{ <compare_targets /> }}

## Where the data comes from

- Support states come from the [tool support table](@/docs/targets/_index.md#capability-matrix).
- Paths come from a real sync of one sample spec per kind. Automated checks fail when these states or paths differ from what sync writes.
- File formats come from each path's extension.
- A file that holds several kinds, such as `.claude/settings.json`, appears under each kind that writes to it.

This page compares output files. It does not confirm native discovery or enforcement. Open a target page for field limits and the setting behind an optional output. Codex project commands, for example, can be written through a legacy opt-in but are not discovered by Codex.

Next, [compare your own specs](@/docs/cli-reference/inspect.md#compare) before changing the selected targets.
