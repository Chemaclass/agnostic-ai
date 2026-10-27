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


A target writes configuration for one coding tool. Filter the table, then open a tool for its paths, import coverage, and native limits. New here? [Create one rule for Claude Code and Codex](@/docs/getting-started.md#first-rule).

## Tool support {#capability-matrix}

{{ <capability_matrix /> }}

Some tools are left out of the default target list and must be named in `targets`; see [select project targets](@/docs/configuration.md#targets).

## Tools without a target

Many tools read the root `AGENTS.md` and `.agents/skills/`. Enable a target that writes both, such as [codex](@/docs/targets/codex.md) with its default output paths. Those tools then get your project-wide rules and skills. Codex includes project-wide rules in `AGENTS.md`; see [cross-target behavior](@/docs/target-behavior.md#entry-point-files).

Check the tool's own discovery rules before relying on those shared paths. MCP servers, commands, and permissions need its native configuration; enabling a compatible target does not translate them for an unknown tool.

To ask for full support, [open an issue](https://github.com/Chemaclass/agnostic-ai/issues).

## Related reference

- [Compare two or three targets](@/docs/compare.md)
- [Release updates](https://agnostic-ai.org/updates/): dated tool news. A feature a tool announces is supported only once its target page says so.
- [Select project targets](@/docs/configuration.md#targets)
- [Understand cross-target behavior](@/docs/target-behavior.md)
- [Use directory-specific instructions](@/docs/scoped-context.md)
- [Define specs shared across tools](@/docs/spec-format/_index.md)
- [Add a new adapter](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md)
