+++
title = "Compare targets"
description = "See side by side what agnostic-ai writes for two or three AI coding tools."
weight = 122

[extra]
group = "Reference"
scripts = ["assets/scripts/compare-targets.js"]
+++


# Compare targets


Pick two tools to see what `agnostic-ai sync` writes for each spec kind, and where. Add an optional third in Target C. The URL keeps your choice, so you can share it.

{{ <compare_targets /> }}

## Where the data comes from

- Support states come from the [tool support table](@/docs/targets/_index.md#capability-matrix).
- Paths come from a real sync of one sample spec per kind. Automated checks fail when these states or paths differ from what sync writes.
- File formats come from each path's extension.
- A file that holds several kinds, such as `.claude/settings.json`, appears under each kind that writes to it.

This page compares output files, not how each tool behaves once it reads them. Each target page lists limits for individual fields and the config keys needed to enable an optional kind.
