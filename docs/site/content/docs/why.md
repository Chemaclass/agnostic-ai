+++
title = "Trace generated files"
description = "Trace a generated file back to the specs and adapter that produced it."
weight = 90

[extra]
group = "Workflows"
+++

# `agnostic-ai why <file>`


Trace an emitted file back to its source: the adapter that wrote it, the source spec(s), the `outputs.<target>.*` keys used for the path, and the last sync time.

[`agnostic-ai explain <spec>`](@/docs/cli-reference.md#explain) does the inverse, spec to outputs.

## Usage

```sh
agnostic-ai why <file>
agnostic-ai why <file> --format json
```

`<file>` resolves relative to the project root, and symlinks are followed. `--format json` returns the same data with stable keys, for editor extensions and CI scripts.

## Example

```sh
$ agnostic-ai why .cursor/rules/no-console-log.mdc
.cursor/rules/no-console-log.mdc
  adapter: cursor
  output keys: (adapter defaults)
  last sync: 2026-05-15T10:23:00Z
  sources:
    [rule] no-console-log (.agnostic-ai/rules/no-console-log.md): full
```

## Output fields

| Field | Meaning |
|-------|---------|
| `adapter` | Target whose adapter wrote the file. For a shared entry-point file (`AGENTS.md`), the first consuming target in registry order. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path. `(adapter defaults)` when no overrides match. |
| `last sync` | UTC timestamp from `.agnostic-ai/.sync-state`. `unknown` when the state file is missing. |
| `sources` | Every spec that contributes. Mode is `full` (spec owns the file) or `section` (one of many merged into a shared document). |

## Entry-point files

Targets with no native rules directory (codex, gemini, aider, amp, warp, zed, opencode, crush, jules, goose, openhands, factory, kilo) inline rule bodies into their entry-point file (`AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, ...) under a sentinel `## Rules` block. Augment has a native `.augment/rules/` directory but inlines into `AGENTS.md` too. Sync's entry-point distribution writes that file, not an adapter, so `why` traces it specially: it lists every inlined rule spec as a `section` source and credits the file to the first consuming target.

## Errors

- **No sync state**: `.agnostic-ai/.sync-state` is absent. `why` tells you to run `agnostic-ai sync` first.
- **Untracked file**: the path matches no adapter emission. `why` reports "not synced or not tracked" and tells you to re-run `sync` or check the path.
