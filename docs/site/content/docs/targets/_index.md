+++
title = "Targets"
description = "Compare what agnostic-ai writes for each supported AI tool."
weight = 120
sort_by = "weight"
template = "docs/targets.html"
page_template = "docs/page.html"

[extra]
group = "Reference"
nav_second = "Reference"
scripts = ["assets/scripts/capability-matrix.js"]
+++


# Targets


See what `agnostic-ai sync` writes for each supported tool. Filter the matrix, then open a target name for exact paths and configuration.

To weigh two or three tools against each other, open [Compare targets](@/docs/compare.md). It puts them side by side with the paths each one gets.

## Capability matrix

{{ <capability_matrix /> }}

## Tools without a target

Many tools read the root `AGENTS.md` and `.agents/skills/`. Enable a target that writes both, such as [codex](@/docs/targets/codex.md) with its default output paths, and such a tool gets your unscoped rules and skills. Codex inlines unscoped rules into `AGENTS.md`; see [cross-target behavior](@/docs/target-behavior.md#entry-point-files).

Copy MCP servers and commands into the tool's own config by hand. For [pi](https://pi.dev), that is `.pi/mcp.json` and `.pi/prompts/<name>.md`. Pi loads them only after you trust the project.

To ask for full support, [open an issue](https://github.com/Chemaclass/agnostic-ai/issues).

## Related reference

- [Compare two or three targets](@/docs/compare.md)
- [Select project targets](@/docs/configuration.md#targets)
- [Understand cross-target behavior](@/docs/target-behavior.md)
- [Use directory-specific instructions](@/docs/scoped-context.md)
- [Define portable spec kinds](@/docs/spec-format/_index.md)
- [Add a new adapter](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md)
