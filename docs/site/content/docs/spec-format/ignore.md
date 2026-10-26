+++
title = "Ignore"
description = "ignore/: patterns that supported tools leave out of context or indexing."
weight = 100

[extra]
group = "Reference"
+++

# Ignore

`ignore/` lists paths supported tools leave out of context or indexing, such as generated code or large fixtures. An ignore spec writes the same patterns into each tool's own ignore file, such as `.cursorignore`, `.geminiignore`, `.aiignore`, or `.aiderignore`. Claude Code and Codex get none; the [target support table](@/docs/targets/_index.md#capability-matrix) lists who does.

Depending on the tool, an ignore file covers reads, indexing, or shell commands. It does not block access on its own; use [permissions](@/docs/spec-format/settings.md#support-by-target) for that.

## Write one

```bash
agnostic-ai new ignore generated-files
```

The template excludes nothing. Put gitignore-syntax patterns in the body, one file per group, using exact paths or narrow patterns.

````markdown
Paths to leave out of context or indexing.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns, and the text around them is prose. A body without a fence is read whole. Prettier strips trailing spaces inside a block, so write a name ending in a space as `name[ ]`. Comment and blank lines exclude nothing; `#` starts a comment only at the start of a line.

## Output

Specs are joined into each tool's ignore file under a `#` header naming the source, with a blank line between them. Order and whitespace are kept, and CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Tools without an ignore file report the spec as unsupported.

## Overwrite behaviour

Sync replaces an ignore file that has no agnostic-ai generated-file header only when every existing pattern stays, unchanged and in order. Extra patterns are fine. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with [`AAI-103`](@/docs/errors.md#aai-103-hand-authored-ignore-file-would-lose-patterns) and leave the file untouched, even when the rewrite means the same thing.

Run `agnostic-ai import <target>` to copy the patterns into `ignore/<target>.md` as a fenced block, with comments, order, and whitespace kept and a leading UTF-8 byte-order mark dropped. The spec sets `target: <target>`; remove it to share the patterns with every tool, and check the combined order if other specs add negations or reorder patterns.

`outputs.<target>.provenance-header: false` removes the header and turns this check off. A dry run skips the check; `sync --check` still reports unsafe overwrites.
