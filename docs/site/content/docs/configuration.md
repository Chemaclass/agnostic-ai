+++
title = "Configuration"
description = "Configure sources, targets, output overrides, sync behavior, imports, and global defaults."
weight = 130

[extra]
group = "Reference"
+++

# Configuration


`agnostic-ai.yaml` lives at the project root and is read from the current working directory. Every section is optional. The legacy filename `agnostic.config.yaml` still loads, with a deprecation warning.

## Minimal project config

```yaml
version: 1
targets: [claude, cursor]
gitignore:
  enabled: true
```

`agnostic-ai init` creates one with your selected tools.

For directory-specific instructions, add `scope` to a rule: `agnostic-ai new rule payments-context --scope services/payments`. See [scoped context](@/docs/scoped-context.md). Set `on-unsupported: error` when every target must preserve scope.

## Find a setting

| Change | Section |
|---|---|
| Select tools | [Targets](#targets) |
| Change source or output paths | [Sources](#sources) and [Outputs](#outputs) |
| Keep generated files out of Git | [Gitignore](#gitignore) |
| Customize sync behavior | [Sync](#sync) |
| Run project behavior checks after model or CLI changes | [Verify](#verify) |
| Keep a hand-written file at a generated path | [`sync.unmanaged`](#syncunmanaged) |
| Stop an older agnostic-ai from syncing your specs | [`requires`](#requires) |
| Change when `lint` warns about instruction size | [`lint`](#lint) |
| Override settings on one machine | [Local overrides](#local-overrides) |
| Keep personal specs and instructions out of Git | [Local spec layers](@/docs/local-overrides.md) |
| Understand which value wins | [Precedence](#precedence) and [Layered specs](#layered-specs) |
| Share personal instructions across projects | [Global configuration](#global-configuration) |
| Inspect all fields | [Top-level fields](#top-level-fields) or [JSON Schema](https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json) |

## Local overrides

`agnostic-ai.local.yaml` holds per-machine tweaks. It deep-merges over the base: scalars and lists replace, maps merge recursively. `agnostic-ai init` adds it to `.gitignore`. Personal specs and instructions go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

```yaml
# agnostic-ai.local.yaml (never committed)
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

## Editor validation

`init` adds this comment so YAML Language Server editors validate against `docs/schemas/config.schema.json`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json
```

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version, reserved for migrations. |
| [`requires`](#requires) | string | none | Oldest agnostic-ai release the specs work with. |
| [`sources`](#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | 20 adapters | Adapters to emit. |
| [`outputs`](#outputs) | map | per target | Output path overrides. |
| [`on-unsupported`](#on-unsupported) | string | `warn` | Unsupported kind handling. |
| [`gitignore`](#gitignore) | map | `enabled: false` | Managed `.gitignore` block. |
| [`sync`](#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | External behavior gate. |
| [`import`](#import) | map | per source | Import behavior. |
| [`lint`](#lint) | map | see section | Budgets for always-loaded text. |

## `requires`

Set it when your specs rely on behavior from a specific release:

```yaml
requires: ">=0.71.0"
```

Every command that reads your specs, such as `sync`, `sync --check`, `lint`, `validate`, `doctor --fix`, `revert`, and `cleanup`, then stops on an older binary before it reads specs or writes files. The error is [AAI-005](@/docs/errors.md#aai-005-installed-version-older-than-requires) and names the command to upgrade. A running `sync --watch` stops syncing when a pulled config raises `requires` above it. The check applies from agnostic-ai 0.70.0; older releases ignore the key.

The only form is `>=X.Y.Z`; anything else fails as AAI-004 naming the file. A build from source, such as `go run` or a commit after a tag, is not a release and warns once instead. `agnostic-ai.local.yaml` can replace the value, and `requires:` with no value there turns the check off. The [global home config](#global-configuration) accepts the key too.

## `sources`

Missing directories are skipped silently. See [path semantics](#path-semantics).

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | `agents` | `*.md` agent specs. |
| `skills` | `skills` | `*.md` skill specs (or nested `<name>/SKILL.md`). |
| `rules` | `rules` | `*.md` rule specs. |
| `hooks` | `hooks` | `*.yaml` hook specs. |
| `mcps` | `mcps` | `*.yaml` MCP server specs. |

## `outputs`

Gemini settings share the `outputs.gemini.mcp-file` destination with MCP servers and hooks. Factory `outputs.factory.skills-dir` applies below each skill scope as well as at the root. Claude disabled MCP policy requires project `.mcp.json`; its generated rejection ownership state follows `outputs.claude.dir`.

`outputs.<target>.*` overrides where one target writes; unknown fields are ignored. Target pages list the keys and defaults, starting at the [targets index](@/docs/targets/_index.md). [Claude Code](@/docs/targets/claude.md#claude-settings) and [Codex](@/docs/targets/codex.md#codex-config) also accept settings blocks.

```yaml
outputs:
  claude:
    rules-dir: .claude/rules
  cursor:
    mcp-file: .cursor/mcp.json
```

## `targets`

Default: every adapter except `amp`, `warp`, `jules`, `goose`, and `augment` (20 in total). Enabling those alongside `codex` is safe; the shared `AGENTS.md` body is written once. Unknown targets log a warning and are skipped. `-t/--target` overrides the list for one run.

`agnostic-ai init` and the first `agnostic-ai sync` can write this list through a picker. See [`init`](@/docs/cli-reference.md#init) and the [first-sync target picker](@/docs/cli-reference.md#first-sync-target-picker).

## `sync`

Per-target overrides live in `outputs.<target>`. Per-run flags such as `--diff`, `--format`, and `--jobs` have no config key; see [`sync`](@/docs/cli-reference.md#sync), [parallel emission](@/docs/cli-reference.md#parallel-emission), and [reading a failing `--check`](@/docs/cli-reference.md#reading-a-failing---check).

| Key | Default | Effect |
|-----|---------|--------|
| [`collision-policy`](#synccollision-policy) | `prompt` | What happens when two targets write the same path. |
| [`target-overview`](#synctarget-overview) | `false` | Append a generated-locations section to each entry-point file. |
| [`resolve-imports`](#syncresolve-imports) | `passthrough` | How `@path` lines reach targets that cannot resolve them. |
| [`dropped-summary`](#syncdropped-summary) | `false` | Print a per-target summary of dropped and downgraded kinds. |
| [`shared-skills`](#syncshared-skills) | `false` | Symlink byte-identical skill folders to one copy. |
| [`unmanaged`](#syncunmanaged) | empty | Paths sync never touches. |

### `sync.collision-policy` {#synccollision-policy}

Applies when two targets write different content to one path, such as `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`. Per-target override: `outputs.<target>.collision-policy`.

| Value | Behavior |
|-------|----------|
| `prompt` | Default. Fail with an `output collision` error and a hint, which in CI suggests a non-interactive policy. |
| `prefer-spec` | Skip the collision check. Last adapter wins. Use in CI when the overlap is intentional. |
| `fail` | Hard error with no resolution hint. |

```yaml
sync:
  collision-policy: prefer-spec
```

### `sync.target-overview` {#synctarget-overview}

When `true`, each entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gets an appendix listing where that tool's generated artifacts live (rules dir, agents dir, MCP file, ...). It honors `outputs.<target>.*` overrides.

```yaml
sync:
  target-overview: true
```

- Only the appendix differs per file. A shared entry point such as `AGENTS.md` lists each reader in its own section.
- The appendix sits between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->`. `import` strips it, so the `AGNOSTIC_AI.md` round-trip stays lossless. `.agnostic-ai/AGNOSTIC_AI.md` never carries it. Every sync regenerates it, so do not hand-edit it.
- Aider, whose artifacts all flow through the entry point, gets no appendix. External adapters (`agnostic-ai-adapter-<name>` binaries) get no section.

### `sync.resolve-imports` {#syncresolve-imports}

Controls how a line holding only an `@path` import in `AGNOSTIC_AI.md` reaches targets that cannot resolve it. `CLAUDE.md` always keeps it, since Claude resolves imports. An `@mention` inside a sentence is untouched. Paths resolve from the project root.

| Value | Non-resolving targets get |
|-------|---------------------------|
| `passthrough` | Default. The `@`-line verbatim, as a dead reference. |
| `strip` | Nothing: the line is dropped. |
| `inline` | The referenced file's content between `<!-- agnostic-ai:import:start <path> -->` and `<!-- agnostic-ai:import:end -->`. `import` restores the `@`-line. A missing or unreadable file fails the sync. |

```yaml
sync:
  resolve-imports: inline
```

### `sync.dropped-summary` {#syncdropped-summary}

When `true`, sync ends by listing, per target, kinds with no surface (dropped) or emitted only behind an opt-in key or source-dir only (downgraded). It regroups capability warnings and [coverage notes](#coverage-notes) by target.

```yaml
sync:
  dropped-summary: true
```

```
  dropped summary (per target):
    cursor: 2 hooks dropped (unsupported)
    gemini: 3 skills via outputs.gemini.emit-skills-as-commands
```

### `sync.shared-skills` {#syncshared-skills}

When `true`, targets sharing the Agent Skills layout (`<dir>/<name>/SKILL.md` plus assets: Claude, Cursor, Codex, Amp) keep one real tree per skill; the others get relative symlinks.

```yaml
sync:
  shared-skills: true
```

- The canonical copy is `.agents/skills/<name>` when emitted (Codex and Amp scan it natively), otherwise the first emitted target's tree.
- Only identical rendered folders link. Overrides such as `x-cursor` keys or codex-only `agents/openai.yaml` keep real copies for diverging targets.
- Links are per skill folder, so hand-authored skills are untouched. Turning the option off, or divergence, restores real trees on the next sync. Removing a skill sweeps its tree and links.
- Without symlink support (Windows without the privilege), sync warns once and keeps real copies.

### `sync.unmanaged` {#syncunmanaged}

Paths you own. `sync` never writes, merges, copies, or removes them.

```yaml
sync:
  unmanaged:
    - .cursor/rules/legacy.mdc        # exact path
    - .claude/agents/hand-*.md        # glob; `*` stays inside one path segment
    - .claude/skills/legacy/          # trailing slash: everything under the directory
```

- Entries are project-relative with forward slashes; `\` is a glob escape. A leading `./` or `/` is ignored. Globs use Go `path.Match`; `**` is not supported. A malformed glob or an entry naming no path (`.`, `./`, `/`, empty) fails config load.
- `sync` prints `~ skip (unmanaged) <path>` (`--json`: under `skipped`, action `"unmanaged"`). `sync --check`, `status`, and `doctor` never count it as drift; `doctor` lists it under `User-owned`, not `Unmanaged config`. `revert` and `doctor --fix` never touch it.
- It stays out of the sync ledger, so removing the entry deletes nothing; the next sync rewrites the file with the provenance header.
- The `.gitignore` block lists generated files one per line in a directory that could hold a match, instead of collapsing it.
- With `sync.shared-skills`, such a skill folder is never linked; an existing link becomes a real copy, keeping edits.
- The list is project-wide. `agnostic-ai.local.yaml` replaces it whole.
- Not covered yet: hook script bodies copied from `.agnostic-ai/scripts/` into `.<tool>/hooks/`.

## `verify`

`verify.command` is the argv list `agnostic-ai verify` runs. It starts the executable directly, without a shell, so pipes and redirects belong in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

See the [`verify` command](@/docs/cli-reference.md#verify) for the drift check, the JSON input, and exit codes.

## `import`

Per-source options for the `import` command. Empty blocks use per-source defaults.

### `import.codex.shred`

Controls how `agnostic-ai import codex` treats a nested `AGENTS.md`. The root file always lands in `.agnostic-ai/AGNOSTIC_AI.md`.

| Value | Behavior |
|-------|----------|
| `true` | Default. One rule spec per `##` heading. |
| `false` | One rule spec per `AGENTS.md`, full body verbatim. Use when it duplicates standalone rules and you want it as a reference doc. |

```yaml
import:
  codex:
    shred: false
```

## `lint`

Budgets `agnostic-ai lint` checks the text each target loads in every session against. Past a budget, lint warns, and `lint --strict` exits 1.

| Key | Default | Effect |
|-----|---------|--------|
| `instructions-words` | `2000` | Words one target loads every session: its entry-point file, always-on rule files, and skill and agent descriptions (LINT011). |
| `description-chars` | `1024` | Characters in one skill or agent description (LINT012). A skill description past 1024 still warns under a higher value. |

```yaml
lint:
  instructions-words: 3000
  description-chars: 500
```

Where the defaults come from:

- Claude Code asks for a `CLAUDE.md` [under 200 lines](https://code.claude.com/docs/en/memory): "Longer files consume more context and reduce adherence." That is about 2000 words of prose.
- Codex stops reading `AGENTS.md` once the files reach `project_doc_max_bytes`, [32 KiB by default](https://developers.openai.com/codex/guides/agents-md), and Antigravity truncates a rule file [past 24,000 bytes](https://antigravity.google/docs/rules). 2000 words sits well under both. Lint also warns past either cap, whatever the word budget.
- The [Agent Skills specification](https://agentskills.io/specification) caps a skill description at 1024 characters.

A missing key or `0` keeps the default; a negative value fails as AAI-004. [`lint` in the CLI reference](@/docs/cli-reference.md#lint) shows the finding. The [global home config](#global-configuration) accepts the key too.

## `on-unsupported`

Applies when an adapter receives a spec kind it does not support (e.g. `hooks` for Cursor or `mcps` for Cline).

| Value | Behavior |
|-------|----------|
| `warn` | Default. Log to stderr and continue. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

## Coverage notes

`sync` prints a `note:` line when specs of a kind exist but a target emits them only behind an inactive opt-in key, or not at all:

```
  note: 2 skills reach gemini, opencode only via outputs.<target>.emit-skills-as-commands
  note: 1 agent reaches warp only via outputs.warp.workflows-dir
```

Setting the named key clears the note. Warnings and notes that match the previous sync collapse into one count line; `sync -v` shows them again.

| Target | Kind | Set this to emit |
|--------|------|------------------|
| `gemini` | skills | `outputs.gemini.emit-skills-as-commands` |
| `opencode` | skills | `outputs.opencode.emit-skills-as-commands` |
| `warp` | agents | `outputs.warp.workflows-dir` |
| `aider` | agents, skills | `outputs.aider.rules-file` |
| `zed` | hooks | `outputs.zed.tasks-file` |
| `kilo` | agents with `tools` | None. Use `x-kilo: {permission: {...}}`. |
| `gemini` | agents with `tools` beyond Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch | None. Use `x-gemini: {tools: [...]}` or drop `tools`. |
| `crush` | hooks not on `PreToolUse` | None. Crush runs `PreToolUse` only. |

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when absent; `agnostic-ai init` writes `true` | Every `sync` rewrites a managed `.gitignore` block listing every path the configured adapters emit. |
| `path` | `.gitignore` | Another file, for monorepos or local-only ignore files. |
| `allow` | empty | Gitignore globs written verbatim as `!` lines at the end of the block, so a tracked file (e.g. a `testdata/AGENTS.md` fixture) is not ignored. |

`sync --gitignore` and `init --gitignore` override it per run; see the [CLI reference](@/docs/cli-reference.md#init).

The block sits between `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside it are kept, and an unchanged sync keeps the file mtime. Its header says to edit specs, and that a fresh clone or `git worktree` lacks these paths until `sync` runs (see [post-checkout hook](@/docs/git-hooks.md#regenerate-on-checkout)).

- Entries are root-anchored (`/AGENTS.md`, not `AGENTS.md`), so nested same-named files are not ignored.
- Files collapse to their generated subdirectory (`/.claude/rules/`), never higher, so siblings such as `.claude/settings.json` or `.claude/hooks/` stay visible. The subdirectory is measured below the resolved output dir, so a nested `outputs.<target>.dir: vendor/.claude` collapses to `/vendor/.claude/rules/`. A nested per-kind dir such as `outputs.<target>.rules-dir` is generated end to end, so it collapses at the dir itself.
- Scoped output in a project directory stays one line per file (`/services/api/AGENTS.md`), so new files in that directory are not ignored.
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai/local/`, seeded by `init` even with `gitignore.enabled: false`. `init`, `sync`, or `packs add` moves old loose copies into the block.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings and agent memory.

## Watched inputs

`sync --watch` re-emits when the config files, any `sources` directory, `.agnostic-ai/local/`, or `.agnostic-ai/overlays/` change. Overlays hold keys the spec layer does not own, such as Claude `statusLine` or Codex `[profiles.*]`. See [`sync --watch`](@/docs/cli-reference.md#sync).

## Path semantics

- `sources` and `outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus one root entry-point file per enabled target, all sharing the canonical pointer body. See the [per-target table](@/docs/target-behavior.md#entry-point-files). An ignored `.agnostic-ai/local/AGNOSTIC_AI.md` [extends that body](@/docs/local-overrides.md#extend-the-instructions) on one machine.

Setting `outputs.<target>.rules-file: <path>` restores the legacy layout: the adapter writes one merged document at `<path>`. Two adapters writing different content to one path fail unless you set `sync.collision-policy: prefer-spec`.

Sync skips the pointer body only when `<path>` is the target's own entry-point file (`outputs.claude.rules-file: CLAUDE.md`, `outputs.codex.rules-file: AGENTS.md`), since the adapter already owns that exact write. Point it anywhere else and the target keeps its entry-point file, with rule bodies delivered by the adapter alone. For `claude` the pointer body also gains an `@<path>` import, because a merged file outside `.claude/rules/` is on no Claude Code auto-load path.

### Per-target paragraphs

`.agnostic-ai/AGNOSTIC_AI.md` accepts the `::target` / `::targets` / `::end` fences from [spec bodies](@/docs/spec-format.md#per-target-body-fences).

```md
Shared conventions for every tool.

::target gemini
Gemini reads `GEMINI.md` only. Load rules from `.gemini/rules/`.
::end

::targets codex amp
Run `make preflight` before you stop.
::end
```

- Unfenced content goes to every entry-point file.
- A fenced block reaches a file when any of its readers is listed, so `::target codex` reaches every `AGENTS.md` reader. A shared file is never split.
- Markers never reach output and must start at column 0; indent a sample that shows one.
- `agnostic-ai validate` flags a fence naming an unknown target, or a built-in target that reads no entry-point file.
- `agnostic-ai import <tool>` keeps a fenced source when the imported file equals what sync renders (under the default `sync.resolve-imports: passthrough`); otherwise it overwrites the source and warns.

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Specs load from three layers, lowest first. Higher layers override by spec name per kind; new names append. The `project-user` layer merges into the shared spec field by field instead of replacing it; see [local overrides](@/docs/local-overrides.md#override-fields). `agnostic-ai list` shows each spec's layer.

| Layer | Root | Loaded when |
|-------|------|-------------|
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai/local` | directory exists |

Only `project` honors custom `sources` paths. `.agnostic-ai/local/` stays out of Git by default and can extend `AGNOSTIC_AI.md`; see [local overrides](@/docs/local-overrides.md). `$AGNOSTIC_AI_HOME` is not a project layer.

## Global configuration

`agnostic-ai sync --global` installs user-level instructions, rules, hooks, and skills for 22 of the 25 targets ([global output](@/docs/target-behavior.md#global-output) lists paths). It works from any directory and loads no packs or project specs. Personal overrides live in the source root's `local/` directory.

Source root: `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when `AGNOSTIC_AI_HOME` is unset.

```text
~/.agnostic-ai/
├── agnostic-ai.yaml        # optional: targets, requires, lint
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── settings/*.yaml         # default model and effort
├── mcps/*.yaml             # user-level MCP servers
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer
    ├── agnostic-ai.yaml
    ├── AGNOSTIC_AI.md
    ├── agents/*.md
    ├── rules/*.md
    ├── hooks/*.yaml
    ├── settings/*.yaml
    ├── mcps/*.yaml
    └── skills/<name>/SKILL.md
```

Specs in `local/` merge into shared specs with the same kind and name, field by field, and a `::parent` line extends the shared body; see [local overrides](@/docs/local-overrides.md#override-fields). New names append. `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after shared agreements and effective rules. Without `local/`, sync uses the shared home alone.

Before adding personal files, add this entry to the source root's `.gitignore`:

```gitignore
/local/
```

To sync a fixed set of tools without `--only` on every run, list them in the source root's `agnostic-ai.yaml`:

```yaml
targets: [claude, codex, cursor]
```

`sync --global` and `sync --global --check` then touch those targets only, and `lint --global` and `validate --global` check against them. A `targets` list in `local/agnostic-ai.yaml` replaces the shared one. `--only` and `--except` narrow the list for one run and must name configured targets. `--target` replaces it and skips the home config's `targets`, so a broken list never blocks it. A repeated name counts once. A target with no user-level surface, such as `aider` or `continue`, is skipped with one warning, so a project-shaped `agnostic-ai.yaml` keeps working. A name that is no target at all stops the run with the closest supported one. A [`requires`](#requires) key stops `sync --global`, `list --global`, `lint --global`, and `validate --global` on an older binary, and one in `local/agnostic-ai.yaml` replaces the shared one. With `--target`, and for `list --global`, a home config that does not parse only warns, but a `requires` that parsed still holds. A `lint` key sets the budgets `lint --global` uses; each key in `local/agnostic-ai.yaml` replaces the shared one. Global mode reads no other key: `version` passes, and any other key prints a warning and is ignored. A target dropped from the list keeps its synced files and ownership records, as a run with `--only` does, until you remove them by hand.

For example, `local/skills/reviewer/SKILL.md` replaces `skills/reviewer/SKILL.md`. Run `agnostic-ai list --global` to see the effective specs with their `global` or `global-local` layer. Run `agnostic-ai validate --global` and `agnostic-ai lint --global` to check both layers before a sync writes them. Global layers never merge with project specs. [Local overrides](@/docs/local-overrides.md) compares this layer with the project one.

- It targets every supported tool by default, or the home config's `targets`. Which `sync` flags it accepts is in the [CLI reference](@/docs/cli-reference.md#sync).
- Nested rules and rules with scope, path, glob, or target conditions are rejected. Commands, settings `permissions`, inheritance, and merging with project specs are unsupported.
- Global skills render native frontmatter and copy bundled assets verbatim. Claude resolves skill `model` and `effort`, including per-target maps and `x-claude` overrides. Shared directories such as `~/.agents/skills/` keep neutral frontmatter, even when syncing one target: target overrides are omitted.
- Global Codex skills also get `agents/openai.yaml`, so `disable-model-invocation: true` keeps a skill manual-only there too. A skill marked `disable-model-invocation: true` prints a coverage note for each target whose global copy stays model-invocable. Syncing one target keeps the files another target placed in a shared skills directory, so `--only amp` leaves Codex's policy in place.
- Hooks and skills honor `target`, `targets`, and `targets-exclude`. Set hook events for each target explicitly; sync does not translate event names.
- Agents use each target's native format, metadata overrides, and include/exclude filters. Eighteen targets have global agent output; see [global output](@/docs/target-behavior.md#global-output) for paths and discovery limits. Unsupported targets warn and skip agents. `readonly: true` maps to Codex's read-only sandbox and to Claude's `disallowedTools`; targets that drop `readonly` report a coverage note.
- Output is real files, never symlinks. A user file that is itself a symlink, such as a `CLAUDE.md` or `settings.json` kept in a dotfiles repository, is written through: the link stays, its target gets the change, and sync names each one. Removing such a file removes the link and the file it points at. A symlink inside a skills, agents, or rules directory still stops the run. Ownership is recorded per target in `$AGNOSTIC_AI_HOME/state/global.json`. Sync keeps unrelated text, JSON keys, hooks, skills, and agents, and removes only recorded artifacts for the targets in the run, so `--only` never sweeps another target. A hooks file whose managed entries did not change is left byte for byte; a rewrite keeps its key order and indent. A managed hook gone from its file, or a hooks file gone altogether, counts as removed: sync warns and writes it again from the source. A managed hook with the same matcher and command but other edits stops the run; restore or remove it, then sync.
- An unmanaged agent, skill, or rule collision, damaged marker, invalid native JSON, or corrupt state stops the run before writes. So does state recorded under another `HOME`: sync under that home, or remove the state file and the files it lists.
- An unrecorded file that holds exactly what sync would write is adopted, not a collision, and so is a skill folder whose every file matches. Sync names each one. So after a lost or deleted state file, a sync of the same targets records its earlier output again. A file that differs by one byte still stops the run.
- A hook entry sync did not write that matches a source hook exactly satisfies it: sync adds no copy and never records or removes it. One with the same matcher and command but other settings, even inside a group with other commands, stops the run. Remove it, or give the command its own entry that matches the source.
- A hand edit to a file sync owns, or to the managed block of an instructions file, also stops the run before writes and names the file. Move the edit into the source, or rerun with `--backup` to overwrite it and keep `<path>.bak`. Text outside the managed block is yours and never counts. State written before this check has nothing to compare, so the first sync after upgrading proceeds as before.
- A run without `--only`, explicit targets, or a home config `targets` list skips a target whose configuration root variable is relative, or whose native format rejects an agent name, and warns. Naming the target, on the command line or in the home config, turns either into an error.
- Empty surfaces create nothing: no instructions file (a recorded one is removed) and no hooks file.
- Native tool precedence applies when global and project configuration both exist. Shared agent files remain until every owning target removes them. To update a file shared by Goose and OpenHands, sync both targets together.

Ordinary `agnostic-ai sync` does not load `~/.agnostic-ai/`. Run inside the global source root, or any directory under it such as `local/`, it stops before any write and points at `sync --global`, since the home config would otherwise read as a project config. That covers `--check`, `--dry-run`, `--plan`, `--json`, and `--watch` too. `init`, `import`, `new`, `packs add`, `packs remove`, `packs update`, `cleanup`, `revert`, and `install-hook` stop the same way. For a home kept in git, `install-hook --global` writes a pre-commit hook that runs `lint --global --strict`, `validate --global`, and `sync --global --check`. A path through a symlink counts. When `AGNOSTIC_AI_HOME` is your home directory itself, only that directory is guarded, so projects under it still work. Read-only commands such as `lint`, `validate`, and `doctor` still run there. Move project-only defaults into a project's `.agnostic-ai/` or a pack, along with any agents, commands, settings permissions, reviews, environments, or ignore specs. A repository's `.agnostic-ai/` stays project-specific despite the shared basename.

To start a home from what your tools already hold, run `agnostic-ai import --global`; see [import](@/docs/cli-reference.md#import).

### Default model and effort {#global-default-model-and-effort}

Settings specs in the home set each tool's default model and effort in its user settings file. `model` and `effort` each take a string for every target, or a map per target with an optional `default`:

```yaml
# ~/.agnostic-ai/settings/defaults.yaml
model:
  claude: opus
  codex: gpt-6-luna
effort:
  claude: high
  codex: high
```

| Target | File | Keys |
|---|---|---|
| codex | `~/.codex/config.toml` | `model`, `model_reasoning_effort` |
| claude | `~/.claude/settings.json` | `model`, `effortLevel` |
| copilot | `~/.copilot/settings.json` | `model`, `effortLevel` |
| qoder | `~/.qoder/settings.json` | `model.name`, `model.reasoningEffort` |
| gemini | `~/.gemini/settings.json` | `model.name` |

Other targets raise a coverage note, and so does `permissions` in a global settings spec. Augment's `~/.augment/settings.json` has no default model or effort key but takes `x-augment` keys, such as `shell`. Gemini sets thinking per model, with no default effort key, so `effort` raises a note there. Claude's and Copilot's `effortLevel` take `low`, `medium`, `high`, or `xhigh`; Qoder takes `disabled`, `off`, `none`, `low`, `medium`, `high`, `xhigh`, or `max`; Codex takes any string. A dotted key is a nested JSON object: sync sets `name` inside `model` and leaves the object's other keys alone. `lint --global` (LINT014) and `validate --global` flag a value a target cannot take.

Each layer overrides the one before it:

1. `settings/*.yaml` in the home.
2. `local/settings/*.yaml`. A file with the same name merges into the shared one field by field. A new file comes after the shared ones, so its `model` and `effort` win.
3. The project tier. A project's settings file, such as `.codex/config.toml` or `.claude/settings.json`, wins through the tool's own precedence.
4. An agent's own `model` and `effort`, for that agent.
5. The tool's flag for one run, such as `codex -m` or `claude --model`.

Sync edits only the keys it writes and records them in `state/global.json`. Every other line stays byte for byte, comments and tables included. A new Codex key goes after the last top-level key, before the first table. A missing file is created with only these keys. Removing the spec and syncing again removes only those keys, and a file sync created goes entirely once nothing is left in it.

- A key that already holds the value sync would write is adopted, and sync names it. Moving a setting you set by hand into the home produces no diff.
- A key with another value stops the run before writes and names the file, the key, and both values. Codex's `/model` picker saves its choice to `config.toml`, so this is normal use: the message prints the target line to put in the spec to keep the new value. `--backup` overwrites the key instead and keeps `<path>.bak`.
- `--dry-run` lists each key a write sets or removes, and `--check` fails on a changed key.
- An `x-<target>` block sets that target's own keys in the same file, with the same per-key ownership. A nested object merges leaf by leaf, so `x-claude.statusLine.command` leaves a `statusLine.padding` you set by hand alone, and an `x-claude` key wins over the portable field it shares a key with. Codex takes top-level scalars and arrays, such as `x-codex.model_reasoning_summary` or `x-codex.notify`; a table such as `profiles` raises a coverage note. `x-claude.hooks` and `x-claude.permissions` raise one too: hook specs own the first, and the second needs its own design. A later spec wins key by key, and `null` drops a key an earlier spec set.
- `agnostic-ai explain --global settings/defaults.yaml` names the file and key each target gets from that spec. A key a later spec overrides is not listed. `explain --global` takes any global spec, so `explain --global agents/reviewer.md` lists each user-level agent file.

### MCP servers {#global-mcp-servers}

MCP specs in the home's `mcps/` install each server in the user MCP file of every target that has one, rendered the way the target's project MCP file renders it:

| Target | File | Where |
|---|---|---|
| claude | `~/.claude.json` (`$CLAUDE_CONFIG_DIR/.claude.json` when set) | top-level `mcpServers.<name>` |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]` table |
| cursor | `~/.cursor/mcp.json` | `mcpServers.<name>` |
| copilot | `~/.copilot/mcp-config.json` | `mcpServers.<name>`, with `tools: ["*"]` when the spec sets none |
| gemini | `~/.gemini/settings.json` | `mcpServers.<name>` |
| qoder | `~/.qoder/settings.json` | `mcpServers.<name>` |
| augment | `~/.augment/settings.json` | `mcpServers.<name>` |
| openhands | `~/.openhands/mcp.json` (`$OPENHANDS_PERSISTENCE_DIR/mcp.json` when set) | `mcpServers.<name>`: `{command, args, env}` or `{url, transport, headers, auth}`; the fields OpenHands adds when it saves the file count as the same server |

Each server is one record with the per-key rules above: a hand-written server that means the same as the spec is adopted as written, even when it leaves out an implied `type` or Copilot's default `tools`, a different one with the same name stops the run (`--backup` overwrites it), and a server sync wrote goes when its spec goes. Servers you add by hand under other names stay. A Codex table sync replaces takes its subtables, such as `[mcp_servers.<name>.env]`, with it. Claude Code rewrites `~/.claude.json` itself, with sign-in and trust state, so sync edits only its own `mcpServers` entries in place and keeps every other key and the file's permissions, creating a missing file at `0600`. A sync stops without writing if the file changed after sync read it; rerun it. A spec with `disabled: true` stays out of the Augment, Claude, Cursor, Copilot, and OpenHands user files, where a listed server is live in every project. When `CLAUDE_CONFIG_DIR` or another root variable moves a file, the next sync takes its servers and keys out of the old one. Claude's own writer keeps those entries. If a running session writes the file back without them, the next sync adds them again.

For a personal agent shared by Claude Code and Codex, create `~/.agnostic-ai/agents/reviewer.md`:

```markdown
---
name: reviewer
description: Review code for correctness
targets: [claude, codex]
---
Review the changes and report actionable findings.
```

Then run from any directory:

```console
agnostic-ai sync --global --only claude,codex
agnostic-ai sync --global --only claude,codex --check
```

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`. If a manually copied file already occupies an output path, preserve its edits in the source and move it aside before syncing. `--backup` saves managed files before replacement; it does not bypass unmanaged collisions.
