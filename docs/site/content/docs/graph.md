+++
title = "Graph"
description = "See which specs reach which targets, and which files they produce."
weight = 80

[extra]
group = "Workflows"
+++

# graph

Show which specs reach which targets, and which files they produce.

`graph` walks the loaded specs and asks each configured adapter which files each spec produces. Then it prints the result. It is read-only and never invokes Emit on disk. Output is deterministic, sorted by spec name, then target.

## Synopsis

```bash
agnostic-ai graph [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--format <text\|mermaid\|dot\|json>` | Output format. Default `text`. |
| `--target <name>` | Restrict to one target. |
| `--spec <name>` | Restrict to one spec name. |
| `--kind <kind>` | Restrict to one kind (`agent`, `skill`, `rule`, `hook`, `mcp`, `command`). |

## Formats

### text (default)

A matrix with specs as rows and targets as columns. Each cell holds the kind that target emits, or `-` when it emits nothing.

```text
spec           | claude cursor codex
code-reviewer  | agent  agent  agent
no-console-log | rule   rule   rule
other          | rule   rule   rule
```

### mermaid

`graph LR`, for embedding in Markdown docs.

```mermaid
graph LR
  S_no_console_log["no-console-log"] --> T_claude["claude"] --> F_0[".claude/rules/no-console-log.md"]
  S_no_console_log["no-console-log"] --> T_cursor["cursor"] --> F_1[".cursor/rules/no-console-log.mdc"]
```

### dot

Graphviz directed graph. Pipe it to `dot(1)` for SVG or PNG.

```bash
agnostic-ai graph --format dot | dot -Tsvg > graph.svg
```

```dot
digraph agnostic_ai {
  rankdir=LR;
  "no-console-log" -> "claude" -> ".claude/rules/no-console-log.md";
  "no-console-log" -> "cursor" -> ".cursor/rules/no-console-log.mdc";
}
```

### json

One record per edge, for editor extensions and scripts.

```json
[
  {
    "spec": "no-console-log",
    "kind": "rule",
    "target": "claude",
    "path": ".claude/rules/no-console-log.md"
  }
]
```

## Examples

```bash
# Default matrix view
agnostic-ai graph

# Only the claude column
agnostic-ai graph --target claude

# One spec across every target
agnostic-ai graph --spec no-console-log

# Only rule-kind edges, as JSON
agnostic-ai graph --kind rule --format json

# Render an SVG of the whole graph
agnostic-ai graph --format dot | dot -Tsvg > graph.svg
```

## See also

- [`render`](@/docs/cli-reference/inspect.md#render) prints the file content for one spec, per target.
- [`explain`](@/docs/cli-reference/inspect.md#explain) lists every output file and section one spec contributes to.
