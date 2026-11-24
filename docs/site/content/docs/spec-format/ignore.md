+++
title = "Ignore"
description = "ignore/: patterns that supported tools leave out of context or indexing."
weight = 100

[extra]
group = "Reference"
+++

# Ignore

`ignore/` lists paths supported tools leave out of context or indexing, such as generated code or large fixtures. An ignore spec writes the same patterns into each tool's own ignore file, such as `.cursorignore`, `.geminiignore`, `.aiignore`, or `.aiderignore`. Claude Code and Codex get none; the [target support table](@/docs/targets/_index.md#capability-matrix) lists supported tools.

An ignore file can cover reads, indexing, or shell commands, depending on the tool. Use [permissions](@/docs/spec-format/settings-reference.md#support-by-target) to block access.

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

With fenced code blocks, only the lines inside them are patterns, and the text around them is prose. Without a fence, the whole body is read as patterns.

{% <details summary="Details: comments, blank lines and trailing spaces"> %}
- Comment and blank lines exclude nothing. `#` starts a comment only at the start of a line.
- Prettier strips trailing spaces inside a block, so write a name ending in a space as `name[ ]`.
{% </details> %}

## Output

Specs are joined into each tool's ignore file under a `#` header naming the source, with a blank line between them. Order and whitespace are kept, and CRLF becomes LF.

- Override the path with `outputs.<target>.ignore-file`.
- Tools without an ignore file report the spec as unsupported.

## Overwrite behaviour

Sync replaces an ignore file that has no agnostic-ai generated-file header only when every existing pattern stays, unchanged and in order. Extra patterns are fine. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with [`AAI-103`](@/docs/errors.md#aai-103-hand-authored-ignore-file-would-lose-patterns) and leave the file untouched, even when the rewrite means the same thing.

To keep your patterns, run `agnostic-ai import <target>`. It copies them into `ignore/<target>.md` as a fenced block.

{% <details summary="Details: import, sharing across tools and turning the check off"> %}
- Import keeps comments, order, and whitespace, and drops a leading UTF-8 byte-order mark.
- The spec sets `target: <target>`. Remove it to share the patterns with every tool, and check the combined order if other specs add negations or reorder patterns.
- `outputs.<target>.provenance-header: false` removes the header and turns this check off.
- A dry run skips the check. `sync --check` still reports unsafe overwrites.
{% </details> %}
