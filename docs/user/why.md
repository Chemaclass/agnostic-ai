# `agnostic-ai why <file>`

Trace an emitted file back to its provenance: the adapter that produced
it, the source spec(s) that contributed, the `outputs.<target>.*` config
keys used to derive the path, and the last sync timestamp.

Use it when you find a file under a target directory and want to know
why it exists. Pairs with `agnostic-ai explain <spec>`, which goes the
opposite direction (spec to outputs).

## Usage

```sh
agnostic-ai why <file>
agnostic-ai why <file> --format json
```

`<file>` is resolved relative to the project root. Symlinks are
followed.

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

`--format json` returns the same data with stable keys for editor
extensions and CI scripts.

## Output fields

- **adapter**: the target whose adapter wrote the file.
- **output keys**: every `outputs.<target>.*` config key whose value
  appears in the resolved path. Reports `(adapter defaults)` when no
  per-target overrides match.
- **last sync**: UTC timestamp from `.agnostic-ai/.sync-state`. Reports
  `unknown` when the state file is missing.
- **sources**: every loaded spec that contributes to the file, with the
  contribution mode (`full` when the spec owns the file, `section` when
  the spec is one of many merged into a shared document).

## Errors

- **No sync state**: when `.agnostic-ai/.sync-state` is absent, `why`
  suggests running `agnostic-ai sync` first.
- **Untracked file**: when the path does not match any adapter's
  emission, `why` reports "not synced or not tracked" and suggests
  re-running `sync` or verifying the path.
