+++
title = "Ignore"
description = "ignore/: paths an agent must not read or index, written into each tool's ignore file."
weight = 100

[extra]
group = "Reference"
+++

# Ignore

`ignore/` lists what agents must not read or index: secrets, generated code, vendored dependencies, large fixtures. Most tools read their own ignore file, such as `.cursorignore`, `.geminiignore`, `.aiignore`, or `.aiderignore`. An ignore spec writes the same patterns into each one.

- **Secrets stay out of context.** An `.env` file or a key folder is not sent to the model.
- **Smaller context.** Build output and vendored code stop crowding search and indexing.
- **One list.** Add a pattern once instead of in every tool's file.

Tools apply an ignore file to their own reads and indexing. Whether it also stops a shell command depends on the tool, so pair it with a [settings](@/docs/spec-format/settings.md) `deny` rule for anything that must never be read.

## Write one

Markdown with optional YAML settings between `---` lines at the top, one file per group. The body holds gitignore-syntax patterns.

````markdown
Secrets and build artifacts the agent should never read.

```gitignore
*.env
secrets/
dist/
```
````

With fenced code blocks, only the lines inside them are patterns. The text around them is prose, which formatters like Prettier can safely rewrite. A body without a fence is read whole. Prettier strips trailing spaces inside a block, so write a name ending in a space as `name[ ]`.

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

