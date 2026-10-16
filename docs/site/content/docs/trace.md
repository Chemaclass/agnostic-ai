+++
title = "Trace generated files"
description = "Find which specs and which tool produced a generated file."
weight = 90
aliases = ["/docs/why/"]

[extra]
group = "Workflows"
+++

# `agnostic-ai why <file>`

Use `why` to find the source of a generated file before changing it. It lists the source specs, the tool that writes the file, and its last sync time.

[`agnostic-ai explain <spec>`](@/docs/cli-reference/inspect.md#explain) does the reverse: it lists the files a spec writes.

## Usage

```sh
agnostic-ai why <file>
agnostic-ai why <file> --format json
```

Use a project-relative path, even for a planned file. Symlinks resolve to the real path. `--format json` exposes the report to scripts.

In [VS Code](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode), use `agnostic-ai: Open canonical source` for the current file.

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

Edit the path listed under `sources`, then run `agnostic-ai sync`. To see which tools receive that source, use [graph](@/docs/graph.md).

<details>
<summary>Interpret report fields and shared instruction files</summary>

## Output fields

| Field | Meaning |
|-------|---------|
| `adapter` | Tool that wrote the file. For a shared file such as `AGENTS.md`, the first tool that reads it. For a path several tools share (`.agents/skills/`), a tool listed in `targets`. If only a tool you did not list writes the path, the line reads `adapter: amp (not configured)`. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path. `(adapter defaults)` when none do. |
| `last sync` | UTC time from `.agnostic-ai/.sync-state`. `unknown` when that file is missing. |
| `configured` | JSON only. `true` when the tool is listed in `targets`. |
| `sources` | Every spec that feeds the file. The mode is `full` (the spec owns the file) or `section` (one of several merged into a shared file). |

## Entry-point files

Each root instruction file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, ...) is a copy of `.agnostic-ai/AGNOSTIC_AI.md`, so `why` lists that file first, as an `instructions` source. It is `full` when nothing else goes into the file.

```sh
$ agnostic-ai why CLAUDE.md
CLAUDE.md
  adapter: claude
  ...
  sources:
    [instructions] AGNOSTIC_AI.md (.agnostic-ai/AGNOSTIC_AI.md): full
```

The ignored `.agnostic-ai/local/AGNOSTIC_AI.md` follows when present. Tools with no rules directory (codex, gemini, aider, amp, warp, zed, opencode, crush, jules, goose, openhands, factory, kilo) also get rule text in a `## Rules` block in the entry-point file. Augment does the same in `AGENTS.md`. Each such rule is a `section` source. The file is credited to the first tool that reads it.

</details>

## Errors

- **No sync state**: `.agnostic-ai/.sync-state` is missing. Run `agnostic-ai sync` first.
- **Source file**: a spec (`.agnostic-ai/rules/x.md`) is a source file, so `why` points you to `agnostic-ai explain <spec>`. `.agnostic-ai/AGNOSTIC_AI.md` gets the same note, naming the entry-point files it feeds.
- **Untracked file**: the path is not something sync writes. `why` reports "not synced or not tracked". Re-run `sync` or check the path.
