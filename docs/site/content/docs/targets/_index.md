+++
title = "Targets"
description = "Compare what agnostic-ai writes for each supported AI tool."
weight = 120
sort_by = "weight"
template = "docs/targets.html"
page_template = "docs/page.html"

[extra]
group = "Reference"
nav_after = "spec-format"
scripts = ["assets/scripts/capability-matrix.js"]
+++


# Targets


See what `agnostic-ai sync` writes for each supported tool. Filter the matrix, then open a target name for exact paths and configuration.

## Capability matrix

{{ <capability_matrix /> }}

## Related reference

- [Select project targets](@/docs/configuration.md#targets)
- [Understand cross-target behavior](@/docs/target-behavior.md)
- [Use directory-specific instructions](@/docs/scoped-context.md)
- [Define portable spec kinds](@/docs/spec-format.md)
- [Add a new adapter](https://github.com/Chemaclass/agnostic-ai/blob/main/docs/internal/adding-adapters.md)
