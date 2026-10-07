+++
title = "Trace generated files"
description = "Find which specs and which tool produced a generated file."
weight = 90
aliases = ["/docs/why/"]

[extra]
group = "Workflows"
+++

# `agnostic-ai why <file>`

Find where a generated file came from: the tool that wrote it, the source specs, the `outputs.<target>.*` keys behind its path, and the last sync time.

[`agnostic-ai explain <spec>`](@/docs/cli-reference/inspect.md#explain) does the reverse: it lists the files a spec writes.

## Usage

```sh
agnostic-ai why <file>
agnostic-ai why <file> --format json
```

`<file>` is relative to the project root. The file does not have to exist yet. Symlinks are followed, so a project opened through a link gives the same answer as its real path. `--format json` returns the same data with fixed keys, for editor extensions and CI scripts.

The [VS Code extension](https://github.com/Chemaclass/agnostic-ai/tree/main/editors/vscode) uses this command. `agnostic-ai: Open canonical source` opens the source spec of the current file. If several specs feed the file, it lets you pick one.

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
| `adapter` | Tool that wrote the file. For a shared file such as `AGENTS.md`, the first tool that reads it. For a path several tools share (`.agents/skills/`), a tool listed in `targets`. If only a tool you did not list writes the path, the line reads `adapter: amp (not configured)`. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path. `(adapter defaults)` when none do. |
| `last sync` | UTC time from `.agnostic-ai/.sync-state`. `unknown` when that file is missing. |
| `configured` | JSON only. `true` when the tool is listed in `targets`. |
| `sources` | Every spec that feeds the file. The mode is `full` (the spec owns the file) or `section` (one of several merged into a shared file). |

## Entry-point files

Each entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, ...) is a copy of `.agnostic-ai/AGNOSTIC_AI.md`, so `why` lists that file first, as an `instructions` source. It is `full` when nothing else goes into the file.

```sh
$ agnostic-ai why CLAUDE.md
CLAUDE.md
  adapter: claude
  ...
  sources:
    [instructions] AGNOSTIC_AI.md (.agnostic-ai/AGNOSTIC_AI.md): full
```

The ignored `.agnostic-ai/local/AGNOSTIC_AI.md` follows when present. Tools with no rules directory (codex, gemini, aider, amp, warp, zed, opencode, crush, jules, goose, openhands, factory, kilo) also get rule text in a `## Rules` block in the entry-point file. Augment does the same in `AGENTS.md`. Each such rule is a `section` source. The file is credited to the first tool that reads it.

## Errors

- **No sync state**: `.agnostic-ai/.sync-state` is missing. Run `agnostic-ai sync` first.
- **Source file**: a spec (`.agnostic-ai/rules/x.md`) is an input, so `why` points you to `agnostic-ai explain <spec>`. `.agnostic-ai/AGNOSTIC_AI.md` gets the same note, naming the entry-point files it feeds.
- **Untracked file**: the path is not something sync writes. `why` reports "not synced or not tracked". Re-run `sync` or check the path.
