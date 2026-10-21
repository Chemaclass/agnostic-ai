+++
title = "Ignore"
description = "ignore/: patterns that supported tools leave out of context or indexing."
weight = 100

[extra]
group = "Reference"
+++

# Ignore

`ignore/` lists paths supported tools leave out of context or indexing, such as generated code, copied dependencies, and large fixtures. Most tools read their own ignore file, such as `.cursorignore`, `.geminiignore`, `.aiignore`, or `.aiderignore`. An ignore spec writes the same patterns into each one.

Claude Code and Codex get no ignore output. In other tools, an ignore file covers reads, indexing, or shell commands, depending on the tool. It is not an access restriction on its own: to block access, use [permissions](@/docs/spec-format/settings.md#support-by-target). The [target support table](@/docs/targets/_index.md#capability-matrix) lists which tools get an ignore file. Use exact paths or narrow patterns.

## Write one

```bash
agnostic-ai new ignore generated-files
```

The template excludes nothing. Replace its description and the comment inside the `gitignore` fence with your patterns. Write one file per group, with optional YAML settings between `---` lines at the top and gitignore-syntax patterns in the body.

````markdown
Paths to leave out of context or indexing.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns. The text around them is prose that formatters such as Prettier can rewrite. A body without a fence is read whole. Prettier strips trailing spaces inside a block, so write a name ending in a space as `name[ ]`.

## Output

Specs are joined into each tool's ignore file under a `#` header naming the source, with a blank line between them. Order and whitespace are kept, and CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Tools without an ignore file report the spec as unsupported.

## Overwrite behaviour

Sync replaces an ignore file that has no agnostic-ai generated-file header only when every existing pattern stays, unchanged and in order.

- Extra patterns are allowed.
- Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with `AAI-103` and leave the file untouched.
- The check is strict, so a rewrite that means the same thing can still fail.

Run `agnostic-ai import <target>` to copy the patterns into `ignore/<target>.md` as a fenced block. Comments, order, and whitespace stay as they were, and a leading UTF-8 byte-order mark is dropped. The spec sets `target: <target>`. Remove that line to share the patterns with every tool that has an ignore file. Check the combined order if other specs add negations or reorder patterns.

Comment and blank lines exclude nothing. `#` starts a comment only at the start of a line.

`outputs.<target>.provenance-header: false` removes the marker and turns this check off. A dry run skips the check. `sync --check` still reports unsafe overwrites.
