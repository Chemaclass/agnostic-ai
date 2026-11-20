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

Write each selected tool's files from your specs. A run that writes takes the [project lock](@/docs/cli-reference/_index.md#concurrent-commands); `--watch` holds it until it exits.

```bash
agnostic-ai sync --only claude,cursor
```

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config). |
| `--only <list>` | Write only these targets. Unknown names error. |
| `--except <list>` | Write all configured targets but these. Unknown names error. Not with `--only`. |
| `--all` | Write every configured target, skipping the [first-sync picker](#first-sync-target-picker). |
| `--dry-run` | Print to stdout instead of writing. Files whose spec is gone show only with `--diff`. |
| `--plan` | Print added and changed counts per target. Writes nothing. Exits 0, or 1 on drift with `--check`. Ignores `--format`. |
| `--check` | Exit non-zero if files on disk differ from what sync would write. Writes nothing. |
| `--diff` | With `--check`, `--plan`, or `--dry-run`, print a diff per changed file. Errors alone, and with `--watch` or `--keep-edits`. |
| `--watch` | Stay running and [re-write on changes](#watch-mode). Not with `--check`. |
| `--json` | Print [JSON](#json-output). Not with `--watch` or `--diff`. |

{% <details summary="More flags: --against, --format, --backup, --keep-edits, --untrack, --jobs and others"> %}

| Flag | Description |
|------|-------------|
| `--against <index\|HEAD>` | With `--check`, compare the staged (`index`) or last committed (`HEAD`) specs with the outputs Git holds there. See [what `--against` compares](#against-details) and [git hooks](@/docs/git-hooks.md#check-staged-files). Not with `--plan`, `--watch`, or `--global`. |
| `--format <human\|github>` | With `--check`: a `human` table (default) or `github` Actions annotations. `--json` wins. |
| `--backup` | Copy each existing output to `<path>.bak` before overwriting, with the file's permissions. Pair with `revert`. |
| `--keep-edits` | Write every output except those edited since the last sync, printing `~ kept <path>` for each (stderr under `--quiet`). With no sync record, "edited" means a tracked output that differs from `HEAD`. Exits 0. For [git hooks](@/docs/git-hooks.md#regenerate-on-checkout). Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--allow-empty-targets` | With an empty `targets` list, remove every file sync generated. Not with `--check`, `--plan`, `--watch`, or `--global`. |
| `--untrack` | Run `git rm --cached` on generated paths git tracks and ignores. The working copy stays. Not with `--check`, `--plan`, `--dry-run`, `--watch`, or `--global`. |
| `--gitignore <on\|off>` | Override `gitignore.enabled` for this run. |
| `--watch-poll` | With `--watch`, poll every 200 ms. For network mounts and container volumes. |
| `--jobs <n>` | Write targets in [parallel](#parallel-emission): `0` (default) is one per CPU, `1` is serial. |
| `--global` | Install user-level specs from `$AGNOSTIC_AI_HOME` (default `~/.agnostic-ai/`) into each tool's user config. See [global configuration](@/docs/global-configuration.md). |

{% </details> %}

<details>
<summary>Flag details: --plan, --diff, --backup, --allow-empty-targets</summary>

- `--plan --diff` prints a diff per changed file after the plan and names each file it would remove.
- With the managed `.gitignore` block on, `--plan` counts `.gitignore` and `.worktreeinclude` under `[agnostic-ai]` when sync would create or change them. They never fail `--plan --check`.
- `--dry-run --diff` also names each generated file a full sync removes because its spec is gone.
- A shared settings file that keeps your own keys shows as a diff, not a removal. `--check`, `--plan`, and plain `--dry-run` report it as changed.
- `--dry-run --diff` stops where sync would stop on a hand edit, and exits 0 on drift.
- `--backup` adds each ignored output's `.bak` to the managed `.gitignore` block, when that block is on. Any other backup sync made, such as one for an output no longer generated, goes in `.git/info/exclude`.
- `--allow-empty-targets` with `--dry-run` (plain, `--diff`, or `--json`) names each file the real run removes and changes nothing.

</details>

<a id="against-details"></a>
<details>
<summary>What --against compares</summary>

- Only outputs Git tracks are compared. An untracked `agnostic-ai.local.yaml` or `.agnostic-ai/local/` is not read.
- Files are read with Git's stored line endings, so `core.autocrlf` and `core.eol` cause no drift. An `eol` rule in `.gitattributes` still applies.
- A shared skill link that Git checks out as a plain file fails with the link and repair choices. See [shared skill checkout policy](@/docs/sync-settings.md#syncshared-skills).
- Any other link checked out as a plain file fails with a message that names `core.symlinks`.
- A tracked output no spec produces fails as a leftover: it has the generated header, or still holds what the previous state wrote (`HEAD` for `index`, the first parent for `HEAD`). A file edited since is left alone. `-t`, `--only`, and `--except` skip this check.
- A tracked hand-written skill, agent, rule, or command in a folder the managed block ignores also fails, since only that tool sees it. The failure names the `import` that adopts it. For a branch that adds `.cursor/skills/<name>/SKILL.md`: run `agnostic-ai import cursor`, then `git rm --cached` the file, or list it under `sync.unmanaged`.

</details>

<details>
<summary>Global sync: accepted flags and output</summary>

- `sync --global` works outside a project. It never loads project config or packs.
- It loads overrides from `local/` in the source root, and `targets` and `on-unsupported` from an optional `agnostic-ai.yaml` there.
- Accepted: `--target`, `--only`, `--except`, `--dry-run`, `--check`, `--backup`, `--plan`, `--json`, and `--diff` with `--dry-run`, `--check`, or `--plan`.
- Rejected: `--watch`, `--gitignore`, `--jobs`, and `--plan` with `--dry-run`.
- `--plan` lists each file a sync would create, update, or delete, with key-level settings and MCP edits. `--json` adds a `keys` list per settings or MCP file.
- Files per tool: [global output](@/docs/global-configuration-reference.md#global-output), [MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers), [model, effort, and permissions](@/docs/global-configuration-reference.md#global-default-model-and-effort).

</details>

### What sync prints

With nothing to do, `sync` prints `✓ 26 targets up to date · 49ms`. After an edit it names the changed specs, then the files created (`+`), updated (`~`), and removed (`-`):

```
  ~ rule testing → 9 files in 9 targets
  + .cursor/rules/testing.mdc  .kiro/steering/testing.md  (+1 more)
  ~ AGENTS.md  CLAUDE.md  GEMINI.md  (+2 more)
  ! 1 file to commit: .openhands/setup.sh
✓ synced 26 targets · 3 created · 6 updated · 49ms
```

- Each list shows three paths (`-vv` lists all). A `config` spec means `agnostic-ai.yaml` changed.
- The `!` line (absent outside a git repository) lists changed files git tracks or does not ignore.

### Hand edits

`sync` saves an output you edited by hand as `<path>.bak` before it overwrites or removes the file. For an overwrite it prints `! overwrote a hand edit to <path> (saved as <path>.bak)` (stderr under `--quiet`).

`agnostic-ai revert` puts the edit back. To keep it, move it into `.agnostic-ai/`, or run `sync --keep-edits`.

<details>
<summary>Hand edit details</summary>

- With the [`memory` built-in](@/docs/memory.md), the line for an instructions file such as `GEMINI.md` also suggests saving a fact in `.agnostic-ai/memory/`, since a tool's own memory may have written the edit.
- If `<path>.bak` exists, sync keeps the new edit in place and says so. It never replaces an earlier backup.
- Exception: sync writes the file when that `.bak` is its own, unchanged, and holds the same bytes as the edited file.
- Sync never writes an output onto a `.bak` it saved earlier, or onto another output's `<path>.bak`. It stops with [AAI-102](@/docs/errors.md#aai-102-targets-emit-to-the-same-output-path) first.
- A key you add to a merged settings file such as `.claude/settings.json` stays in place, without a backup.

</details>

### Tracked files that match ignore rules

A generated path that git tracks and ignores was usually committed before the managed `.gitignore` block. Sync prints `! 1 file tracked despite being ignored: git rm --cached .claude/rules/tone.md`.

- `sync --untrack` runs it for every such path and reports `~ untracked <path>`.
- `doctor` reports it but never fails on it alone.

### Stops, leftovers, and kept files

<details>
<summary>Typos and hand-written instructions</summary>

- A hook event that looks like a typo, such as `PreToolUze`, stops `sync` and `sync --check`, which name the closest event. Another tool's event passes in any case or snake_case.
- An agent `skills:` name close to a project skill, such as `pr-swep`, only warns, since it may be a user or plugin skill.
- `validate` checks pack specs.
- `sync`, `--check`, `--plan`, and `--json --dry-run` stop when an instructions file such as `CLAUDE.md` or `AGENTS.md` holds text agnostic-ai did not write and `.agnostic-ai/AGNOSTIC_AI.md` and `.agnostic-ai/local/` lack it. The message names the `import` that keeps it.
- `sync --backup` replaces the file and keeps the old one as `<path>.bak`, unless that `.bak` exists.
- A file an earlier sync wrote, or with the generated header, never stops sync. Nor does a tracked file under `--keep-edits`.

</details>

<details>
<summary>Removing leftover files</summary>

- `sync` records output paths and checksums in `.agnostic-ai/.sync-state`. A `.sync-state` from a newer agnostic-ai stops every command that writes, such as `sync` and `doctor --fix`, until you upgrade. `status`, `why`, and `doctor` warn.
- A full run deletes files it no longer writes, then empty directories, when a generated-file header or a recorded hash proves ownership.
- A generated file edited since sync is backed up as `<path>.bak` before removal.
- A headerless file edited since sync, or one with no recorded checksum, stays. Sync prints `~ kept orphan <path>` and a reason.
- Kept orphans stay in the managed `.gitignore` block and count as drift. Remove them or add them to `sync.unmanaged`. In a terminal, `doctor --fix` offers removal, defaulting to no.

</details>

<details>
<summary>Empty targets, missing adapters, and case-only renames</summary>

- While `targets` is empty, `sync`, `status`, and `doctor --fix` warn and keep every file. `sync -t` ignores its filter then too.
- `sync --check` fails with AAI-303 while generated files remain.
- If an external adapter is missing from PATH, sync keeps that target's files and shared skill links. It prints `~ skipped removing stale files: <target> did not load; a sync that loads every target removes them`.
- Changing only the capitalization of a rule, skill, or agent name keeps the newly written outputs on case-insensitive disks. Hand-edit backups and `--keep-edits` still apply.
- On case-sensitive disks, sync removes the old spelling's files.

</details>

<details>
<summary>Leftover files without .sync-state (fresh checkout)</summary>

With no `.sync-state`, as in a fresh checkout of a repo that commits generated files, `sync --check` and `doctor` scan Git-tracked files. A tracked file is a leftover when:

- It sits where a configured target writes, and its first line has the generated-file header.
- It still holds exactly what the last commit's specs would write, such as a headerless `.claude/launch.json` whose spec you deleted but have not committed.
- After that deletion is committed, a headerless JSON file still holds exactly what the commit that last changed it would write. Up to eight past commits are checked per run. You delete these by hand.

- A plain full `sync` removes none of them. It records them under target `unledgered` and lists each as `~ kept leftover <path>`. `--only` and `--except` record them without naming them.
- An empty `.sync-state` still counts as a record and turns this scan off.
- `doctor --fix` removes a leftover in a tool directory or root dotfile.
- Delete a scope document such as `services/api/AGENTS.md` by hand, or add it to `sync.unmanaged`.

</details>

### Warning guidance {#warning-guidance}

Each warning comes with one `fix:` line. Guidance never edits specs.

<details>
<summary>Next step by field, and when notes show</summary>

- Unsupported global agents get one `fix:` per run: set `targets:` in `$AGNOSTIC_AI_HOME/agnostic-ai.yaml` (or `local/agnostic-ai.yaml`), or `targets-exclude:` on each agent.
- Repeated project notes stay hidden until they change or you pass `-vv`.

| Field | Next step |
|-------|-----------|
| Skill `argument-hint` | Move it under each keeping tool's override block (today only `x-claude`). Put guidance other tools need in the description or body. |
| Skill `metadata` | Move it under each keeping tool's override block (today `x-claude`, `x-cursor`, `x-opencode`). Shared global skill files have no overrides: use the description or body. |
| Skill `disable-model-invocation` | Use a native invocation policy, or exclude tools that cannot enforce manual invocation. A copy another tool installs in a shared global skills directory still shows. |
| Agent `readonly` | Configure and verify native permissions or session settings, or exclude tools that cannot enforce read-only access. |
| Codex agent `sandbox_mode` | Enforce it in the parent session's `config.toml` (`sandbox_mode = "read-only"` for read-only), then set `x-codex: {sandbox_mode: null}` to omit the inactive key. If the session cannot enforce it, exclude the agent from Codex. |

</details>

### First-sync target picker

On the first `sync` (no `.agnostic-ai/.sync-state`), if the config lists every supported target, `sync` asks which to keep and saves the choice to `agnostic-ai.yaml`. `echo "claude,codex" | agnostic-ai sync` keeps two.

| Context | Behavior |
|---------|----------|
| Interactive terminal | Multi-select prompt, as in `init`. |
| Piped stdin | Selects and saves without a prompt. |
| No terminal, nothing piped (CI) | Writes every configured target. |
| `--all`, `-t`, `--only`, or `--except` | Skips the picker for this run. |

### Reading a failing `--check` {#reading-a-failing---check}

A failing `--check` exits non-zero in every format and points at `sync` and `doctor`. Each changed file gets a label:

| Label | Meaning |
|-------|---------|
| Out of date | The file holds what the last sync wrote, so the specs changed. With no sync record, as in fresh CI, every changed file reads this way. |
| Edited locally | The file changed since the last sync. The next sync saves it as `<path>.bak`, then writes over it. |
| Line endings only | Git checked the file out with CRLF. Run `git config core.autocrlf false`, or add a `.gitattributes` rule such as `* text eol=lf`, then `agnostic-ai sync`. With `--against`, Git stores CRLF or an `eol` rule converts it: drop the rule, then stage or commit the regenerated file. |

<details>
<summary>Shared files, links, and output formats</summary>

- A link Git tracks but checks out as a plain file (`core.symlinks=false`) gets a message naming that setting.
- A file several targets read, such as `.agents/skills/<name>/SKILL.md`, is listed once under the first target, as `(shared with <targets>)`. It is counted once in `--diff`, `--format=github`, and `status`. `--json` keeps one record per target.
- `--diff` prints changed lines, a one-line summary for a missing file, and a count when it truncates a large diff.
- `--format=github` prints `::error file=...,line=...::` annotations on the pull request.
- Neither has a config-file key.
- With `--json`, the once-per-run [LINT038 permission note](@/docs/spec-format/settings.md#permission-rules) goes to stderr.

</details>

### Credential previews

Dry-run, render, import, and migration previews hide literal credential values. Written files and checksums keep them.

<details>
<summary>What is hidden and what is not</summary>

- Hidden: literal values in credential fields, environment maps, headers, and credential flags, assignments, and URLs in recognized commands. Environment references with defaults or literal text are hidden; plain ones show.
- A hidden-value change still reports drift.
- JSON, JSON with comments, YAML, and TOML previews keep valid syntax. A recognized document or sensitive command that cannot be shown safely is withheld.
- API-key helper paths stay visible. Helper commands with literal arguments or shell operations are hidden, since they can contain the key.
- Markdown and other prose previews are withheld whole when a credential label carries a value: `password:` or `token =` with a literal, a table cell or XML tag named `token`, `session`, or `password`, a sentence such as "the password is ...", `mysql -psecret`, or a private key block.
- Text that only names the words ("never commit secrets", "token budget") still shows, as does a label set to `null`, a boolean, a number, or an environment reference.
- The web interface's file view is stricter. It withholds a whole file that names a strong credential word (password, secret, credential, or a key such as API key), even in guidance. See [the source endpoint](@/docs/cli-reference/serve-api.md#get-api-views-source).
- Not detected: secrets in freeform prompts, prose with no label, URL path segments made only of letters, unknown scripts, and binary assets.

</details>

### Watch mode

`sync --watch` re-writes when `agnostic-ai.yaml`, `agnostic-ai.local.yaml`, `.agnostic-ai/AGNOSTIC_AI.md`, `.agnostic-ai/local/`, `.agnostic-ai/overlays/`, or any `sources.*` directory changes. That includes absolute paths outside the project, linked roots, and directories created later. Changes within 50 ms are grouped. Ctrl+C exits.

<details>
<summary>Watch mode details</summary>

- A spec edit re-syncs only the targets that write that kind.
- Config edits, overlay edits, deletions, and renames re-sync all targets and update the watched paths.
- Where file notifications fail, watch polls every 200 ms. It also polls a missing external source with no safe parent to watch (a parent holding the project is never watched). Edits during a re-sync are caught on the next tick.
- Without `--target`, `--only`, or `--except`, each re-sync reads `targets` again. Added tools get their files, and removed tools lose the files sync wrote for them.
- With those flags, watch keeps its initial list. `--all` does not fix the list.
- An empty `targets` (such as `targets:` saved mid-edit) makes watch warn and keep every file from the first pass, even with `--target`. A missing key uses the default targets, as plain `sync` does.

</details>

### Parallel emission {#parallel-emission}

Targets write in parallel. Use `--jobs 1` to debug or pin ordering. `--jobs <n>` bounds the workers and has no config-file key. Output never depends on it.

### Profiling a slow sync

Use `sync -v` or `-vv` for timing, or write a CPU profile with `--profile`.

{% <details summary="Verbosity and profile details"> %}

- `sync -v` shows counts and elapsed time for targets that wrote files, and summarizes unchanged targets.
- `sync -vv` shows every target, path, and repeated coverage note, accepted ones included. It runs sync again.
- `--profile <file>` or `AGNOSTIC_AI_PROFILE=<file>` writes a CPU profile. Under `--jobs > 1` target times overlap.

{% </details> %}

### JSON output

`sync --json`, also with `--plan`, `--dry-run`, or `--check`, writes one object to stdout. The first-sync picker still saves your choice, prompting on stderr.

{% <details summary="JSON fields and action values"> %}

| Field | Description |
|-------|-------------|
| `version` | JSON format version, currently `"1"`. A breaking change bumps it. |
| `command` | `"sync"`, `"sync --plan"`, `"sync --dry-run"`, or `"sync --check"`. |
| `writes` | What sync wrote or removed; for `--check`, what needs attention. |
| `skipped` | Files and links sync left alone. Empty for `--check`. |
| `warnings`, `notes` | The capability warnings and coverage notes a plain `sync` prints, one per target, minus [accepted notes](@/docs/configuration-reference.md#coverageaccept). Always complete, including notes a plain `sync` hides as unchanged. Each has `target`, `kind`, `count`, `message`, and an optional `suggestion` (the plain `fix:`). A project-wide note has target `agnostic-ai`, empty `kind`, and `count` 0. |
| `errors` | Per-target errors with `target` and `message`. `sync --json` goes on with the other targets. A failed target keeps what it wrote before the error, listed in `writes` and `.sync-state`. |

`writes` and `skipped` entries have `target`, `path`, `action`, and `bytes`, such as `{"target": "claude", "path": "CLAUDE.md", "action": "create", "bytes": 1284}`.

`writes` actions:

| `action` | Meaning |
|----------|---------|
| `create`, `update` | File written. |
| `delete` | Orphan removed. |
| `link` | Shared skill folder linked or retargeted. |
| `missing`, `stale`, `edited` | `--check`: not written yet, specs changed, changed since the last sync. |
| `orphan` | `--check`: no longer generated. |
| `leftover` | `--check`: no longer generated; the next full sync removes it. Target `ledger`, or `unledgered` when no record proves sync wrote it. |
| `untracked` | `--untrack`: path removed from the index. Target `agnostic-ai`. |

`skipped` actions:

| `action` | Meaning |
|----------|---------|
| `skip` | File already matches, or link unchanged. |
| `unmanaged` | User-owned. |
| `orphan` | Orphan sync cannot prove it wrote; also an unledgered scope document. |
| `leftover` | Unledgered leftover kept, for `doctor --fix` to remove. |
| `edited` | `--keep-edits`: hand edit left in place. |
| `tracked` | Path git tracks and ignores, without `--untrack`. Target `agnostic-ai`. |

- A write over or removal of a hand edit names its copy in `"backup"`.
- `--plan --json` and `--dry-run --json` write nothing and exit 0. A leftover they would remove is `"delete"`. Kept files are `"orphan"` or `"leftover"`.
- `--dry-run --json` also lists unchanged outputs as `"skip"`. Count `writes` by `target` for the `--plan` numbers.
- `sync --global --json` carries `warnings` and `notes` too, or sends them to stderr when a run fails before printing JSON.

{% </details> %}
