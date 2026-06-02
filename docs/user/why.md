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
    [rule] no-console-log (rules/no-console-log.md): full
```

## Output fields

| Field | Meaning |
|-------|---------|
| `adapter` | Target whose adapter wrote the file. |
| `output keys` | Every `outputs.<target>.*` key whose value appears in the path. `(adapter defaults)` when no overrides match. |
| `last sync` | UTC timestamp from `.agnostic-ai/.sync-state`. `unknown` when the state file is missing. |
| `sources` | Every spec that contributes. Mode is `full` (spec owns the file) or `section` (one of many merged into a shared document). |

## Errors

- **No sync state**: `.agnostic-ai/.sync-state` is absent. `why` suggests running `agnostic-ai sync` first.
- **Untracked file**: path matches no adapter emission. `why` reports "not synced or not tracked" and suggests re-running `sync` or verifying the path.
