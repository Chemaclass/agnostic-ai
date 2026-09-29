+++
title = "Ignore"
description = "ignore/: paths an agent must not read or index, written into each tool's ignore file."
weight = 100

[extra]
group = "Reference"
+++

# Ignore

`ignore/` lists what agents must not read or index: secrets, generated code, vendored dependencies, large fixtures. Most tools read their own ignore file, such as `.cursorignore`, `.geminiignore`, `.aiignore`, or `.aiderignore`. An ignore spec writes the same patterns into each.

- **Secrets stay out of context.** An `.env` file or a key folder is not sent to the model.
- **Smaller, sharper context.** Build output and vendored code stop crowding search and indexing.
- **One list.** Add a pattern once instead of in every tool's file.

Tools apply an ignore file to their own reads and indexing. Whether it also stops a shell command differs by tool, so pair it with a [settings](@/docs/spec-format/settings.md) `deny` rule for anything that must never be read.

## Write one

Markdown with optional YAML frontmatter, one file per group. The body holds gitignore-syntax patterns.

````markdown
Secrets and build artifacts the agent should never read.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns and the surrounding text is prose, which formatters like Prettier can rewrite safely. A body without a fence is read whole. Prettier strips trailing spaces inside a block: write a name ending in a space as `name[ ]`.

## Output

Specs concatenate into each target's native ignore file under a `#` provenance header, with a blank line between them. Order and whitespace are kept; CRLF becomes LF. Override the path with `outputs.<target>.ignore-file`. Targets without an ignore file report the spec as unsupported.

## Overwrite behaviour

Sync replaces an ignore file with no agnostic-ai provenance header only when every existing exclusion survives, unchanged and in order. Extra patterns are allowed. Missing or reordered patterns, added negations (`!pattern`), and changed whitespace fail with `AAI-103` and leave the file untouched. The check is conservative, so an equivalent rewrite can still fail.

Run `agnostic-ai import <target>` to copy the patterns into `ignore/<target>.md` as a fenced block, with comments, order, and whitespace intact and a leading UTF-8 byte-order mark dropped. The spec sets `target: <target>`; remove the line to share the patterns with every ignore-capable target. Review the combined order if other specs add negations or reorder patterns.

Comment and blank lines exclude nothing. `#` starts a comment only at the start of a line. `outputs.<target>.provenance-header: false` removes the marker and disables this check. Dry-run skips the check; `sync --check` still reports unsafe overwrites.

