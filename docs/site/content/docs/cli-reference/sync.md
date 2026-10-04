+++
title = "Generate output (sync)"
description = "Emit per-target configs, preview changes, and read a failing check."
weight = 20

[extra]
group = "Reference"
+++

# Generate output (sync)

## sync

Emit per-target configs. A run that writes takes the [project lock](@/docs/cli-reference/_index.md#concurrent-commands); `--watch` holds it until it exits.

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
| `--plan` | Print per-target added and changed counts without writing. Exits 0. With `--check`, exits 1 on the same drift `--check` fails on, so CI can gate on the short report. `--diff` and `--format` do not apply. Not with `--global`. |
| `--check` | Exit non-zero if disk differs from emitted output. Writes nothing. |
| `--against <index\|HEAD>` | With `--check`, compare what Git holds instead of the working tree. `index` renders the staged specs and compares them with the staged outputs. `HEAD` does the same for the last commit. Ignored outputs are skipped. See [`--against` details](#against-details) and [git hooks](@/docs/git-hooks.md#check-staged-files). Not with `--plan`, `--watch`, or `--global`. |
| `--diff` | With `--check`, print a unified diff per drifted file (on disk vs what sync would write). Without `--check` or `--dry-run` it fails and writes nothing. |
| `--format <human\|github>` | With `--check`: `human` (default) table or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing target file to `<path>.bak` before overwriting. Pair with `revert`. |
| `--keep-edits` | Keep each output edited since the last sync, write the rest, and name each kept file as `~ kept <path>` (on stderr under `--quiet`). With no ledger entry, it keeps each Git-tracked output that differs from `HEAD`. Exits 0. For [git hooks](@/docs/git-hooks.md#regenerate-on-checkout). Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--untrack` | Run `git rm --cached` on generated paths that git tracks and ignores. The working copy stays. Not with `--check`, `--plan`, `--dry-run`, `--watch`, or `--global`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Stay running and re-emit on changes. Incompatible with `--check`. See [watch mode](#watch-mode). |
| `--watch-poll` | With `--watch`, force 200 ms polling, for network mounts or container volumes. |
| `--jobs <n>` | Targets emitted in parallel. `0` (default) is one worker per CPU. `1` is serial. See [parallel emission](#parallel-emission). |
| `--json` | Output as JSON, also with `--plan` or `--dry-run`. Not with `--watch` or `--diff`. See [JSON output](#json-output). |
| `--global` | Install user-level instructions, unconditional rules, hooks, and skills from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into 22 tools' user config, plus native agents for 18 targets. |

<a id="against-details"></a>
{% <details summary="What --against compares"> %}
- Only outputs Git tracks are compared.
- A tracked output that no spec produces fails as a leftover to delete. That covers a file with the generated header, and any file the previous state rendered that still holds what it rendered. The previous state is `HEAD` for `index` and the first parent for `HEAD`.
- A file edited since is left alone.
- A run narrowed with `-t`, `--only`, or `--except` skips the leftover comparison.
- A hand-written skill, agent, rule, or command that Git tracks inside a folder the managed block ignores also fails, because only the tool reading that folder sees it. The failure names the `import` that adopts it. List the file under `sync.unmanaged` to keep it.
{% </details> %}

`sync --global` details:

- Loads overrides from `local/` in the source root. Reads `targets` and `on-unsupported` from an optional `agnostic-ai.yaml` there. See [global configuration](@/docs/configuration.md#global-configuration) and [global output](@/docs/target-behavior.md#global-output).
- Works outside a project. It never loads project config or packs.
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

Each list shows three paths. `-v` lists all. `config` in the spec line means `agnostic-ai.yaml` changed. The `!` line lists changed files that git tracks or does not ignore. It is absent outside a git repository.

**Tracked despite ignored.** A generated path that git both tracks and ignores was usually committed before it moved into the managed `.gitignore` block. `sync` prints `! 1 file tracked despite being ignored: git rm --cached .claude/rules/tone.md`. `sync --untrack` runs that command for every such path and reports `~ untracked <path>`. `doctor` shows the same finding and never fails on it alone.

**Hand edits.** An output whose bytes changed since the last sync was edited by hand. `sync` still writes the spec's version, so the specs stay the one source. It first saves the edit as `<path>.bak` and prints `! overwrote a hand edit to <path> (saved as <path>.bak)` (on stderr under `--quiet`). `revert` puts the edit back. Move the edit into `.agnostic-ai/` to keep it. `--keep-edits` leaves the edit in place instead.

{% <details summary="Hand edits: backups, merged settings, fresh clones"> %}
- When `<path>.bak` already exists, sync leaves the new edit in place and says so. An earlier backup is never replaced.
- A merged settings file, such as `.claude/settings.json`, holds your own keys by design. A key you add there is kept in place, not backed up.
- Without a record of the last sync, as in a fresh clone, there is no proof of an edit.
{% </details> %}

**Typos.** Before writing, `sync` and `sync --check` stop on a hook event that is a likely typo of a known one, such as `PreToolUze`, and name the closest. Another tool's event passes, in any case or snake_case. An agent `skills:` name that is a likely typo of a project skill, such as `pr-swep`, only prints a warning, as in `validate`, because it may be a user or plugin skill. Specs from packs are left to `validate`.

**Hand-written instructions.** Before writing, `sync` stops when an instructions file such as `CLAUDE.md` or `AGENTS.md` holds text that agnostic-ai did not write and `.agnostic-ai/AGNOSTIC_AI.md` does not have. It names the `import` that keeps the text. `sync --backup` replaces the file instead and keeps it as `<path>.bak`, unless that `.bak` already exists. `--check`, `--plan`, and `--json --dry-run` stop the same way.

{% <details summary="Hand-written instructions: what does not stop sync"> %}
- Text the local layer holds counts as held.
- A file that an earlier sync wrote, or one with the generated header, is written as before.
- `--keep-edits` keeps a file that Git tracks in place.
{% </details> %}

**Orphan sweep.** `sync` records every file it writes in `.agnostic-ai/.sync-state`. A full run deletes files it no longer emits and prunes empty directories. It removes only what it can prove it wrote (provenance header or recorded hash). A headerless file is kept as `~ kept orphan <path>`, with the reason, when it was edited since sync or written by a sync that recorded no checksum. Kept orphans stay in the managed `.gitignore` block, including during partial syncs. They count as drift until you remove them or list them under `sync.unmanaged`. In a terminal, `doctor --fix` offers their removal, defaulting to no.

{% <details summary="Orphan sweep without .sync-state (fresh checkout)"> %}
A fresh checkout of a repo that commits generated files has no `.sync-state`. Then `sync --check` and `doctor` scan git-tracked files instead. A tracked file is a leftover in either of these cases:

- It sits where a configured target writes, and its first line carries the provenance header.
- It still holds exactly what the last commit's specs rendered. This covers a headerless JSON output such as `.claude/launch.json` whose spec you deleted but have not committed.

Once the deletion is committed, a headerless JSON file is checked against the commit that last changed it. When it still holds exactly what that commit's specs rendered, it is listed for you to delete by hand. Up to eight past commits are rendered per run.

A plain full `sync` removes none of these files. It records them under target `unledgered` and lists each as `~ kept leftover <path>`. `--only` and `--except` record them without naming them. An empty `.sync-state` still counts as a ledger and turns this scan off.

`doctor --fix` removes a leftover in a tool directory or root dotfile. Delete a scope document such as `services/api/AGENTS.md` by hand, or add it to `sync.unmanaged`.
{% </details> %}

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state` yet), if the config still lists every supported target, `sync` asks which ones to keep. It saves the choice to `agnostic-ai.yaml`.

| Context | Behavior |
|---------|----------|
| TTY | Multi-select prompt, same as `init`. |
| Piped stdin | Selects and saves without a prompt. |
| Non-TTY, nothing piped (CI) | Emits every configured target. |
| `--all`, `-t`, `--only`, or `--except` | Skips the picker for this run. |

`echo "claude,codex" | agnostic-ai sync` keeps two targets.

### Reading a failing `--check` {#reading-a-failing---check}

A drifting `--check` exits non-zero in every format. Each drifted file gets one of two labels:

| Label | Meaning |
|-------|---------|
| Out of date | The file still holds what the last sync wrote, so the specs changed. |
| Edited locally | The file's bytes changed since the last sync. The next sync saves it as `<path>.bak`, then writes over it. |

Without a record of the last sync, as in a fresh CI checkout, a changed file reads as out of date. Stderr names the fix, `agnostic-ai sync`, and points at `agnostic-ai doctor`.

A file several targets read, such as `.agents/skills/<name>/SKILL.md`, is listed once under the first target, followed by `(shared with <targets>)`. `--diff`, `--format=github`, and the `status` count show it once too. `--json` keeps one record per target.

- `--diff` prints changed lines. A missing file gets a one-line create summary. A large diff truncates with a count.
- `--format=github` emits `::error file=...,line=...::` annotations on the pull request.

Neither has a config-file key.

Bare capabilities in settings `permissions.allow` or `permissions.ask` print a LINT038 note once each sync run. It names the native permissions and a scoped alternative. `on-unsupported: silent` and `--quiet` hide it. With `--json`, the note goes to stderr. See [permission rules](@/docs/spec-format/settings.md#permission-rules).

### Watch mode

`sync --watch` watches these paths, including ones that appear later:

- `agnostic-ai.yaml` and `agnostic-ai.local.yaml`
- `.agnostic-ai/AGNOSTIC_AI.md`
- every `sources.*` directory, including absolute paths outside the project and linked roots
- `.agnostic-ai/local/` and `.agnostic-ai/overlays/`

It uses fsnotify with a 50 ms debounce, polls every 200 ms where fsnotify fails, and exits on Ctrl+C. A spec change re-syncs only the targets that emit that kind. Config and overlay edits, deletes, and renames re-sync everything.

Polling picks up edits made during a re-sync on the next tick. Config changes update the source paths it watches.

Watch mode also polls when a missing external source has no safe parent to watch. It never adds a watch on a parent that contains the project.

### Parallel emission {#parallel-emission}

`sync` emits each target on its own worker. `--jobs <n>` bounds how many run at once. There is no config-file key. Output never depends on the value. Use `--jobs 1` only to debug or pin ordering.

### Profiling a slow sync

`--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. `--verbose` adds wall time per target, as in `→ claude: 12 created, 3 updated, 0 unchanged in 42ms`. Under `--jobs > 1` these times overlap. Read them as per-adapter cost, not a serial breakdown.

### JSON output

`sync --json`, `sync --plan --json`, `sync --dry-run --json`, and `sync --check --json` share one schema:

| Field | Description |
|-------|-------------|
| `version` | Schema version, currently `"1"`. Breaking changes bump it. |
| `command` | `"sync"`, `"sync --plan"`, `"sync --dry-run"`, or `"sync --check"`. |
| `writes` | Files written (`"create"`, `"update"`), orphans removed (`"delete"`), or, for `--check`, files needing attention (`"missing"`, `"stale"`, `"edited"`, `"orphan"`, `"leftover"`). `"stale"`: the specs changed. `"edited"`: the file changed since the last sync. `"leftover"`: no longer generated, removed by the next full sync (target `ledger`, or `unledgered` when no ledger proves sync wrote it). With `--untrack`, a path removed from the index is `"untracked"`, target `agnostic-ai`. A write over a hand edit names the copy it kept in `"backup"`. |
| `skipped` | Files already matching (`"skip"`), user-owned (`"unmanaged"`), orphans kept because ownership could not be proven (`"orphan"`), unledgered leftovers kept (`"leftover"` for `doctor --fix` to remove, `"orphan"` for a scope document), or, with `--keep-edits`, hand edits left in place (`"edited"`). A path git tracks and ignores, without `--untrack`, is `"tracked"`, target `agnostic-ai`. Empty for `--check`. |
| `errors` | Per-target errors with `target` and `message`. `sync --json` goes on with the other targets. A failed target keeps what it wrote before the error, and those files are in `writes` and the ledger. |

`writes` and `skipped` entries have `target`, `path`, `action` (strings), and `bytes` (number), for example `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

`sync --json` adds `warnings` and `notes`: the capability warnings and coverage notes a plain `sync` prints. `--plan`, `--dry-run`, and `--check` carry the same entries. There is one entry per target, and the lists are empty when there are none. Each entry has `target`, `kind`, `count`, and `message`, for example `{"target": "aider", "kind": "mcp", "count": 1, "message": "1 mcp unsupported by aider"}`. A project-wide note has target `agnostic-ai`, an empty `kind`, and `count` 0. The lists are complete on every run, even for a set a plain `sync` would hide as unchanged. A later plain `sync` still prints them. Accepted notes ([`coverage.accept`](@/docs/configuration.md#coverageaccept)) are left out.

`--plan --json` and `--dry-run --json` write nothing and exit 0. Each leftover they would remove is `"delete"` in `writes`. Kept files are `"orphan"` or `"leftover"` in `skipped`. `--dry-run --json` also lists every unchanged output as `"skip"`. Count `writes` by `target` for the per-target numbers `--plan` prints.
