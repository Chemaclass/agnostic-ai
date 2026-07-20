# `agnostic-ai why <file>`

Trace an emitted file back to its source: the adapter that wrote it, the source spec(s), the `outputs.<target>.*` keys used for the path, and the last sync time.

Use it to find out why a file under a target directory exists. The inverse of [`agnostic-ai explain <spec>`](cli-reference.md#explain) (spec to outputs).

## Usage

```sh
agnostic-ai why <file>
agnostic-ai why <file> --format json
```

`<file>` resolves relative to the project root. Symlinks are followed. `--format json` returns the same data with stable keys for editor extensions and CI scripts.

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

Targets with no native rules directory (codex, gemini, aider, amp, warp, zed, opencode, crush) inline rule bodies into their entry-point file (`AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, ...) under a sentinel `## Rules` block. That file is written by sync's entry-point distribution, not by an adapter, so `why` traces it specially: it lists every inlined rule spec as a `section` source and attributes the file to the first consuming target.

## Errors

- **No sync state**: `.agnostic-ai/.sync-state` is absent. `why` suggests running `agnostic-ai sync` first.
- **Untracked file**: path matches no adapter emission. `why` reports "not synced or not tracked" and suggests re-running `sync` or verifying the path.
