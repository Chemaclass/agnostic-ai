+++
title = "Generate output (sync)"
description = "Emit per-target configs, preview changes, and read a failing check."
weight = 20

[extra]
group = "Reference"
+++

# Generate output (sync)

## sync

Emit per-target configs.

```bash
agnostic-ai sync --only claude,cursor
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Emit only these targets. Errors on unknown names. |
| `--except <list>` | Emit all configured targets except these. Errors on unknown names. Mutually exclusive with `--only`. |
| `--all` | Emit every configured target without the [first-sync picker](#first-sync-target-picker). |
| `--dry-run` | Print to stdout instead of writing files. Does not preview the orphan sweep. |
| `--plan` | Print per-target added and changed counts without writing. Exits 0. |
| `--check` | Exit non-zero if disk differs from emitted output. Writes nothing. |
| `--against <index\|HEAD>` | With `--check`, compare what Git holds instead of the working tree: `index` renders the staged specs and compares them with the staged outputs, `HEAD` does the same for the last commit. Only outputs Git tracks are compared; ignored outputs are skipped. A tracked output that no spec produces fails as a leftover to delete: a file with the generated header, or any file the previous state rendered (`HEAD` for `index`, the first parent for `HEAD`) that still holds what it rendered. A file edited since is left alone, and a run narrowed with `-t`, `--only`, or `--except` skips that comparison. See [git hooks](@/docs/git-hooks.md#check-staged-files). Not with `--plan`, `--watch`, or `--global`. |
| `--diff` | With `--check`, print a unified diff per drifted file (on-disk vs what sync would write). |
| `--format <human\|github>` | With `--check`: `human` (default) table or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert`. |
| `--keep-edits` | Keep each output edited since the last sync (with no ledger entry, each Git-tracked output that differs from `HEAD`), write the rest, and name each kept file as `~ kept <path>` (on stderr under `--quiet`). Exits 0. For [git hooks](@/docs/git-hooks.md#regenerate-on-checkout). Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--untrack` | Run `git rm --cached` on generated paths that git tracks and ignores. The working copy stays. Not with `--check`, `--plan`, `--dry-run`, `--watch`, or `--global`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Stay running and re-emit on changes. Incompatible with `--check`. See [watch mode](#watch-mode). |
| `--watch-poll` | With `--watch`, force 200 ms polling, for network mounts or container volumes. |
| `--jobs <n>` | Targets emitted in parallel. `0` (default) is one worker per CPU; `1` is serial. See [parallel emission](#parallel-emission). |
| `--json` | Output as JSON, also with `--plan` or `--dry-run`. Not with `--watch` or `--diff`. See [JSON output](#json-output). |
| `--global` | Install user-level instructions, unconditional rules, hooks, and skills from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into 22 tools' user config, plus native agents for 18 targets. |

`sync --global` details:

- Loads overrides from `local/` in the source root. Reads `targets` from an optional `agnostic-ai.yaml` there. See [global configuration](@/docs/configuration.md#global-configuration) and [global output](@/docs/target-behavior.md#global-output).
- Works outside a project; never loads project config or packs.
- Accepts `--target`, `--only`, `--except`, `--dry-run`, `--check`, `--check --diff`, `--backup`, `--plan`, and `--json`. `--plan` lists each file a sync would create, update, or delete, with key-level settings and MCP edits. `--json` adds a `keys` list per settings or MCP file.
- MCP specs install servers in the user MCP files of Augment, Claude, Codex, Cursor, Copilot, Gemini, OpenHands, and Qoder. Settings specs set `model` and `effort` for Claude, Codex, Copilot, Qoder, and Gemini. See [default model and effort](@/docs/configuration.md#global-default-model-and-effort).
- Rejects `--watch`, `--gitignore`, `--jobs`, and `--plan` with `--check` or `--dry-run`.

Paths listed under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) are skipped and reported as `~ skip (unmanaged) <path>`.

**Summary.** `sync` ends with what it changed. With nothing to do it prints `✓ 25 targets up to date · 49ms`. After an edit it names the changed specs, then the files created (`+`), updated (`~`), and removed (`-`):

```
  ~ rule testing → 9 files in 9 targets
  + .cursor/rules/testing.mdc  .kiro/steering/testing.md  (+1 more)
  ~ AGENTS.md  CLAUDE.md  GEMINI.md  (+2 more)
  ! 1 file to commit: .openhands/setup.sh
✓ synced 25 targets · 3 created · 6 updated · 49ms
```

Each list shows three paths; `-v` lists all. `config` in the spec line means `agnostic-ai.yaml` changed. The `!` line lists changed files git tracks or does not ignore, and is absent outside a git repository.

**Tracked despite ignored.** A generated path git already tracks, typically committed before it moved into the managed `.gitignore` block, prints `! 1 file tracked despite being ignored: git rm --cached .claude/rules/tone.md`. `sync --untrack` runs that command for every such path and reports `~ untracked <path>`. `doctor` shows the same finding and never fails on it alone.

**Orphan sweep.** `sync` records every file it writes in `.agnostic-ai/.sync-state`. A full run deletes files it no longer emits and prunes empty directories, but only what it can prove it wrote (provenance header or recorded hash). A file edited since is kept as `~ kept orphan <path>` and counts as drift until you delete it or list it under `sync.unmanaged`.

Without `.sync-state` (a fresh checkout of a repo that commits generated files), `sync --check` and `doctor` scan git-tracked files instead. A tracked file is a leftover when it sits where a configured target writes and its first line carries the provenance header, or when it still holds exactly what the last commit's specs rendered. The second test covers a headerless JSON output such as `.claude/launch.json` whose spec you deleted but have not committed. Once that deletion is committed, a headerless JSON file is checked against the commit that last changed it: when it still holds exactly what that commit's specs rendered, it is listed for you to delete by hand. Up to eight past commits are rendered per run. A plain full `sync` removes none of them; it records them under target `unledgered` and lists each as `~ kept leftover <path>`. `--only` and `--except` record them without naming them. An empty `.sync-state` still counts as a ledger and turns this scan off. `doctor --fix` removes a leftover in a tool directory or root dotfile. Delete a scope document such as `services/api/AGENTS.md` by hand or add it to `sync.unmanaged`.

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state` yet), if the config still lists every supported target, `sync` asks which to keep. The choice is saved to `agnostic-ai.yaml`.

| Context | Behavior |
|---------|----------|
| TTY | Multi-select prompt, same as `init`. |
| Piped stdin | Selects and saves without a prompt. |
| Non-TTY, nothing piped (CI) | Emits every configured target. |
| `--all`, `-t`, `--only`, or `--except` | Skips the picker for this run. |

`echo "claude,codex" | agnostic-ai sync` keeps two targets.

### Reading a failing `--check` {#reading-a-failing---check}

A drifting `--check` exits non-zero in every format. A file still holding what the last sync wrote is reported as out of date (the specs changed). A file whose bytes changed since is reported as edited locally (the next sync overwrites it). Without a record of the last sync, as in a fresh CI checkout, a changed file reads as out of date. Stderr names the fix, `agnostic-ai sync`, and points at `agnostic-ai doctor`.

- `--diff` prints changed lines. A missing file gets a one-line create summary; a large diff truncates with a count.
- `--format=github` emits `::error file=...,line=...::` annotations on the pull request.

Neither has a config-file key.

### Watch mode

`sync --watch` watches `agnostic-ai.yaml`, `agnostic-ai.local.yaml`, `.agnostic-ai/AGNOSTIC_AI.md`, every `sources.*` directory, `.agnostic-ai/local/`, and `.agnostic-ai/overlays/`, including ones that appear later. It uses fsnotify with a 50 ms debounce, polls every 200 ms where fsnotify fails, and exits on Ctrl+C. A spec change re-syncs only targets that emit that kind. Config and overlay edits, deletes, and renames re-sync everything.

### Parallel emission {#parallel-emission}

`sync` emits each target on its own worker; `--jobs <n>` bounds how many run at once. There is no config-file key. Output never depends on the value. Use `--jobs 1` only to debug or pin ordering.

### Profiling a slow sync

`--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. `--verbose` adds wall time per target, as in `→ claude: 12 created, 3 updated, 0 unchanged in 42ms`. Under `--jobs > 1` these times overlap: read them as per-adapter cost, not a serial breakdown.

### JSON output

`sync --json`, `sync --plan --json`, `sync --dry-run --json`, and `sync --check --json` share one schema:

| Field | Description |
|-------|-------------|
| `version` | Schema version, currently `"1"`. Breaking changes bump it. |
| `command` | `"sync"`, `"sync --plan"`, `"sync --dry-run"`, or `"sync --check"`. |
| `writes` | Files written (`"create"`, `"update"`), orphans removed (`"delete"`), or, for `--check`, files needing attention (`"missing"`, `"stale"`, `"edited"`, `"orphan"`, `"leftover"`). `"stale"`: the specs changed. `"edited"`: the file changed since the last sync. `"leftover"`: no longer generated, removed by the next full sync (target `ledger`, or `unledgered` when no ledger proves sync wrote it). With `--untrack`, a path removed from the index is `"untracked"`, target `agnostic-ai`. |
| `skipped` | Files already matching (`"skip"`), user-owned (`"unmanaged"`), edited orphans kept (`"orphan"`), unledgered leftovers kept (`"leftover"` for `doctor --fix` to remove, `"orphan"` for a scope document), or, with `--keep-edits`, hand edits left in place (`"edited"`). A path git tracks and ignores, without `--untrack`, is `"tracked"`, target `agnostic-ai`. Empty for `--check`. |
| `errors` | Per-target errors with `target` and `message`. |

`writes` and `skipped` entries have `target`, `path`, `action` (strings), and `bytes` (number), for example `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

`--plan --json` and `--dry-run --json` write nothing and exit 0. Each leftover they would remove is `"delete"` in `writes`; kept files are `"orphan"` or `"leftover"` in `skipped`. `--dry-run --json` also lists every unchanged output as `"skip"`. Count `writes` by `target` for the per-target numbers `--plan` prints.

