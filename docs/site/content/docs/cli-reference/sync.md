+++
title = "Generate output (sync)"
description = "Write each tool's config, preview changes, and read a failing check."
weight = 20

[extra]
nav_title = "Generate output"
group = "Reference"
+++

# Generate output (sync)

## sync

Generate files from your specs for each selected AI coding tool. A run that writes takes the [project lock](@/docs/cli-reference/_index.md#concurrent-commands); `--watch` holds it until it exits.

```bash
agnostic-ai sync --only claude,cursor
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config) |
| `--only <list>` | Write only these targets. Errors on unknown names. |
| `--except <list>` | Write all configured targets except these. Errors on unknown names. Not with `--only`. |
| `--all` | Write every configured target without the [first-sync picker](#first-sync-target-picker). |
| `--dry-run` | Print to stdout instead of writing. Does not preview leftover deletions. |
| `--plan` | Print added and changed counts per target without writing. Exits 0, or 1 on drift with `--check`, so CI can gate on the short report. Ignores `--diff` and `--format`. |
| `--check` | Exit non-zero if files on disk differ from what sync would write. Writes nothing. |
| `--against <index\|HEAD>` | With `--check`, compare the staged (`index`) or last committed (`HEAD`) specs with the outputs Git holds there, not the working tree. See [`--against` details](#against-details) and [git hooks](@/docs/git-hooks.md#check-staged-files). Not with `--plan`, `--watch`, or `--global`. |
| `--diff` | With `--check`, print a diff per changed file, on disk vs what sync would write. Fails without `--check` or `--dry-run`. |
| `--format <human\|github>` | With `--check`: `human` (default) table or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing output file to `<path>.bak` before overwriting. Pair with `revert`. |
| `--keep-edits` | Write every output except those edited since the last sync (with no record, tracked outputs that differ from `HEAD`), and print `~ kept <path>` for each (stderr under `--quiet`). Exits 0. For [git hooks](@/docs/git-hooks.md#regenerate-on-checkout). Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--untrack` | Run `git rm --cached` on generated paths that git tracks and ignores, keeping the working copy. Not with `--check`, `--plan`, `--dry-run`, `--watch`, or `--global`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch` | Stay running and [re-write on changes](#watch-mode). Not with `--check`. |
| `--watch-poll` | With `--watch`, poll every 200 ms, for network mounts or container volumes. |
| `--jobs <n>` | Targets written in [parallel](#parallel-emission): `0` (default) is one per CPU, `1` is serial. |
| `--json` | Print [JSON](#json-output). Not with `--watch` or `--diff`. |
| `--global` | Install user-level specs from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into each tool's user config. See [global configuration](@/docs/global-configuration.md). |

<a id="against-details"></a>
<details>
<summary>What --against compares</summary>

- Only outputs Git tracks are compared. An untracked `agnostic-ai.local.yaml` or `.agnostic-ai/local/` is not read.
- Shared skill links that Git checks out as plain files fail with the affected link and repair choices. See [shared skill checkout policy](@/docs/configuration.md#syncshared-skills).
- A tracked output that no spec produces fails as a leftover to delete: a file with the generated header, or one that still holds what the previous state wrote (`HEAD` for `index`, the first parent for `HEAD`). A file edited since is left alone. A run narrowed with `-t`, `--only`, or `--except` skips this comparison.
- A hand-written skill, agent, rule, or command that Git tracks inside a folder the managed block ignores also fails, because only the tool that reads that folder sees it. The failure names the `import` that adopts it. For example, a branch from before the move to `.agnostic-ai/` adds `.cursor/skills/<name>/SKILL.md`: run `agnostic-ai import cursor`, then `git rm --cached` the file, or list it under `sync.unmanaged` to keep it.

</details>

`sync --global` works outside a project and never loads project config or packs. It loads overrides from `local/` in the source root and reads `targets` and `on-unsupported` from an optional `agnostic-ai.yaml` there.

- Accepts `--target`, `--only`, `--except`, `--dry-run`, `--check`, `--check --diff`, `--backup`, `--plan`, and `--json`. Rejects `--watch`, `--gitignore`, `--jobs`, and `--plan` with `--check` or `--dry-run`.
- `--plan` lists each file a sync would create, update, or delete, with key-level settings and MCP edits. `--json` adds a `keys` list per settings or MCP file.
- Files per tool: [global output](@/docs/global-configuration.md#global-output), [MCP servers](@/docs/global-configuration.md#global-mcp-servers), and [model, effort, and permissions](@/docs/global-configuration.md#global-default-model-and-effort).

**Summary.** With nothing to do, `sync` prints `✓ 26 targets up to date · 49ms`. After an edit it names the changed specs, then the files created (`+`), updated (`~`), and removed (`-`):

```
  ~ rule testing → 9 files in 9 targets
  + .cursor/rules/testing.mdc  .kiro/steering/testing.md  (+1 more)
  ~ AGENTS.md  CLAUDE.md  GEMINI.md  (+2 more)
  ! 1 file to commit: .openhands/setup.sh
✓ synced 26 targets · 3 created · 6 updated · 49ms
```

Each list shows three paths (`-vv` lists all). A `config` spec means `agnostic-ai.yaml` changed. The `!` line, absent outside a git repository, lists changed files git tracks or does not ignore.

**Tracked despite ignored.** For a generated path git both tracks and ignores (usually committed before the managed `.gitignore` block), `sync` prints `! 1 file tracked despite being ignored: git rm --cached .claude/rules/tone.md`. `sync --untrack` runs that command for every such path and reports `~ untracked <path>`. `doctor` reports it but never fails on it alone.

**Hand edits.** `sync` overwrites an output edited by hand since the last sync, after saving the edit as `<path>.bak`, and prints `! overwrote a hand edit to <path> (saved as <path>.bak)` (on stderr under `--quiet`). `revert` puts the edit back; move it into `.agnostic-ai/` to keep it. With the [`memory` built-in](@/docs/memory.md), the line for an instructions file such as `GEMINI.md` also suggests saving a fact in `.agnostic-ai/memory/`, since a tool's own memory may have written the edit. `--keep-edits` leaves the edit in place instead.

<details>
<summary>Hand edits: existing backups and merged settings</summary>

- When `<path>.bak` already exists, sync leaves the new edit in place and says so. An earlier backup is never replaced.
- A key you add to a merged settings file, such as `.claude/settings.json`, is kept in place, not backed up.

</details>

**Typos.** `sync` and `sync --check` stop on a hook event that looks like a typo, such as `PreToolUze`, and name the closest; another tool's event passes in any case or snake_case. An agent `skills:` name close to a project skill, such as `pr-swep`, only warns, since it may be a user or plugin skill. Pack specs are left to `validate`.

**Hand-written instructions.** `sync`, `--check`, `--plan`, and `--json --dry-run` stop when an instructions file such as `CLAUDE.md` or `AGENTS.md` holds text agnostic-ai did not write and neither `.agnostic-ai/AGNOSTIC_AI.md` nor `.agnostic-ai/local/` has, and name the `import` that keeps it. `sync --backup` replaces the file and keeps the old one as `<path>.bak`, unless that `.bak` exists. A file an earlier sync wrote, or with the generated header, never stops it, nor does a tracked file under `--keep-edits`.

**Remove leftover files.** `sync` records every file it writes in `.agnostic-ai/.sync-state`. A full run deletes files, and then empty directories, it no longer writes, if it can prove it wrote them (generated-file header or recorded hash). A headerless file edited since sync, or written by a sync that recorded no checksum, is kept as `~ kept orphan <path>` with the reason. Kept orphans stay in the managed `.gitignore` block and count as drift until removed or listed under `sync.unmanaged`. In a terminal, `doctor --fix` offers to remove them, defaulting to no.

<details>
<summary>Find leftover files without .sync-state (fresh checkout)</summary>

With no `.sync-state`, as in a fresh checkout of a repo that commits generated files, `sync --check` and `doctor` scan Git-tracked files. A tracked file is a leftover when:

- It sits where a configured target writes, and its first line carries the generated-file header.
- It still holds exactly what the last commit's specs would write, such as a headerless `.claude/launch.json` whose spec you deleted but have not committed.
- After that deletion is committed, a headerless JSON file still holds exactly what the commit that last changed it would write. Up to eight past commits are checked per run, and you delete these by hand.

A plain full `sync` removes none of these. It records them under target `unledgered` and lists each as `~ kept leftover <path>` (`--only` and `--except` record them without naming them). An empty `.sync-state` still counts as a record and turns this scan off. `doctor --fix` removes a leftover in a tool directory or root dotfile; delete a scope document such as `services/api/AGENTS.md` by hand, or add it to `sync.unmanaged`.

</details>

### Warning guidance {#warning-guidance}

Unsupported global agents get one `fix:` per run: set `targets:` in `$AGNOSTIC_AI_HOME/agnostic-ai.yaml` (or `local/agnostic-ai.yaml`), or `targets-exclude:` on each agent. These field notes add a next step; guidance never edits specs, and repeated project notes stay hidden until they change or you pass `-vv`.

| Field | Next step |
|-------|-----------|
| Skill `argument-hint` | Move it under each keeping tool's override block (today only `x-claude`). Put guidance other tools need in the description or body. |
| Skill `metadata` | Move it under each keeping tool's override block (today `x-claude`, `x-cursor`, `x-opencode`). Shared global skill files omit overrides, so use the description or body. |
| Skill `disable-model-invocation` | Use a native invocation policy, or exclude tools that cannot enforce manual invocation. Excluding a tool does not hide a copy another tool installs in a shared global skills directory it reads. |
| Agent `readonly` | Configure and verify native permissions or session settings, or exclude tools that cannot enforce read-only access. |
| Codex agent `sandbox_mode` | Enforce it in the parent session's `config.toml` (for read-only, `sandbox_mode = "read-only"`), then set `x-codex: {sandbox_mode: null}` to omit the inactive key. Exclude the agent from Codex if the session cannot enforce it. |

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state` yet), if the config still lists every supported target, `sync` asks which to keep and saves the choice to `agnostic-ai.yaml`. `echo "claude,codex" | agnostic-ai sync` keeps two.

| Context | Behavior |
|---------|----------|
| Interactive terminal (TTY) | Multi-select prompt, same as `init`. |
| Piped stdin | Selects and saves without a prompt. |
| No interactive terminal, nothing piped (CI) | Writes every configured target. |
| `--all`, `-t`, `--only`, or `--except` | Skips the picker for this run. |

### Reading a failing `--check` {#reading-a-failing---check}

A failing `--check` exits non-zero in every format. Stderr names the fix, `agnostic-ai sync`, and points at `agnostic-ai doctor`. Each changed file gets a label:

| Label | Meaning |
|-------|---------|
| Out of date | The file still holds what the last sync wrote, so the specs changed. Without a record of the last sync, as in fresh CI, every changed file reads this way. |
| Edited locally | The file changed since the last sync. The next sync saves it as `<path>.bak`, then writes over it. |

A file several targets read, such as `.agents/skills/<name>/SKILL.md`, is listed once, under the first target with `(shared with <targets>)`, and counted once in `--diff`, `--format=github`, and `status`. `--json` keeps one record per target.

`--diff` prints changed lines, a one-line summary for a missing file, and truncates a large diff with a count. `--format=github` prints `::error file=...,line=...::` annotations on the pull request. Neither has a config-file key.

The once-per-run [LINT038 permission note](@/docs/spec-format/settings.md#permission-rules) goes to stderr with `--json`.

### Credential previews

Dry-run, render, import, and migration previews hide literal values in known credential fields, environment maps, headers, and credential flags, assignments, and URLs in recognized commands. Plain environment references stay visible; ones with defaults or mixed literal text are hidden. A hidden-value change still reports drift.

- JSON, JSON with comments, YAML, and TOML previews keep valid syntax; a recognized document or sensitive command that cannot be shown safely is withheld.
- API-key helper paths stay visible, but helper commands with literal arguments or shell operations are hidden, since they can contain the key.
- Written files, captured content, and tracking checksums keep the real values. Secrets in freeform prompts, prose, unknown scripts, or binary assets are not detected.

### Watch mode

`sync --watch` watches `agnostic-ai.yaml`, `agnostic-ai.local.yaml`, `.agnostic-ai/AGNOSTIC_AI.md`, `.agnostic-ai/local/`, `.agnostic-ai/overlays/`, and every `sources.*` directory (absolute paths outside the project and linked roots included), even ones that appear later. It groups changes within 50 ms and exits on Ctrl+C. A spec change re-syncs only the targets that write that kind; config and overlay edits, deletes, and renames re-sync everything and update the watched paths. It polls every 200 ms where notifications fail or a missing external source has no safe parent to watch (never a parent holding the project), catching edits made during a re-sync on the next tick.

### Parallel emission {#parallel-emission}

Each target gets its own worker; `--jobs <n>` bounds how many run at once (no config-file key). The output never depends on it. Use `--jobs 1` only to debug or pin ordering.

### Profiling a slow sync

`sync -v` shows counts and elapsed time for targets that wrote files, and summarizes unchanged targets. `sync -vv` shows every target, path, and repeated coverage note, including accepted notes; it runs sync again. `--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. Under `--jobs > 1` target times overlap.

### JSON output

`sync --json`, also with `--plan`, `--dry-run`, or `--check`, writes one object to stdout, without summaries or verbose diagnostics. The first-sync picker still saves your choice, prompting on stderr.

| Field | Description |
|-------|-------------|
| `version` | JSON format version, currently `"1"`. A breaking change bumps it. |
| `command` | `"sync"`, `"sync --plan"`, `"sync --dry-run"`, or `"sync --check"`. |
| `writes` | Files written (`"create"`, `"update"`), orphans removed (`"delete"`), shared skill folders linked or retargeted (`"link"`), or, for `--check`, files needing attention (`"missing"`, `"stale"`: the specs changed, `"edited"`: changed since the last sync, `"orphan"`, or `"leftover"`: no longer generated, removed by the next full sync, target `ledger`, or `unledgered` when no record proves sync wrote it). With `--untrack`, a path removed from the index is `"untracked"`, target `agnostic-ai`. A write over a hand edit names its copy in `"backup"`. |
| `skipped` | Files already matching or links unchanged (`"skip"`), user-owned (`"unmanaged"`), orphans sync cannot prove it wrote (`"orphan"`), unledgered leftovers kept (`"leftover"` for `doctor --fix` to remove, `"orphan"` for a scope document), or, with `--keep-edits`, hand edits left in place (`"edited"`). A path git tracks and ignores, without `--untrack`, is `"tracked"`, target `agnostic-ai`. Empty for `--check`. |
| `warnings`, `notes` | The capability warnings and coverage notes a plain `sync` prints, one entry per target, complete on every run (also notes a plain `sync` hides as unchanged), minus [accepted notes](@/docs/configuration.md#coverageaccept). Each has `target`, `kind`, `count`, `message`, and an optional `suggestion` (the plain `fix:`). A project-wide note has target `agnostic-ai`, empty `kind`, and `count` 0. |
| `errors` | Per-target errors with `target` and `message`. `sync --json` goes on with the other targets. A failed target keeps what it wrote before the error, listed in `writes` and `.sync-state`. |

`writes` and `skipped` entries have `target`, `path`, `action`, and `bytes`, for example `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

`--plan --json` and `--dry-run --json` write nothing and exit 0. A leftover they would remove is `"delete"`; kept files are `"orphan"` or `"leftover"`. `--dry-run --json` also lists unchanged outputs as `"skip"`. Count `writes` by `target` for the `--plan` numbers.

`sync --global --json` and its previews carry `warnings` and `notes` too, or send them to stderr when a run fails before printing JSON.
