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
| Override settings on one machine | [Local overrides](#local-overrides) |
| Keep personal specs and instructions out of Git | [Local spec layers](@/docs/local-overrides.md) |
| Understand which value wins | [Precedence](#precedence) and [Layered specs](#layered-specs) |
| Share personal instructions across projects | [Global configuration](#global-configuration) |
| Inspect all fields | [Top-level fields](#top-level-fields) or [JSON Schema](https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json) |

## Local overrides

`agnostic-ai.local.yaml` holds per-machine tweaks. It deep-merges over the base: scalars and lists replace, maps merge recursively. `agnostic-ai init` adds it to `.gitignore`. Personal specs and instructions go in [`.agnostic-ai.local/`](@/docs/local-overrides.md).

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
| [`sources`](#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | 20 adapters | Adapters to emit. |
| [`outputs`](#outputs) | map | per target | Output path overrides. |
| [`on-unsupported`](#on-unsupported) | string | `warn` | Unsupported kind handling. |
| [`gitignore`](#gitignore) | map | `enabled: false` | Managed `.gitignore` block. |
| [`sync`](#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | External behavior gate. |
| [`import`](#import) | map | per source | Import behavior. |

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

Controls how `agnostic-ai import codex` treats `AGENTS.md`.

| Value | Behavior |
|-------|----------|
| `true` | Default. One rule spec per `##` heading. |
| `false` | One rule spec per `AGENTS.md`, full body verbatim. Use when it duplicates standalone rules and you want it as a reference doc. |

```yaml
import:
  codex:
    shred: false
```

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

Setting the named key clears the note. Notes matching the previous sync are suppressed; delete `.agnostic-ai/.sync-state` to show them again.

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
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai.local/`, seeded by `init` even with `gitignore.enabled: false`. `init`, `sync`, or `packs add` moves old loose copies into the block.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings and agent memory.

## Watched inputs

`sync --watch` re-emits when the config files, any `sources` directory, `.agnostic-ai.local/`, or `.agnostic-ai/overlays/` change. Overlays hold keys the spec layer does not own, such as Claude `statusLine` or Codex `[profiles.*]`. See [`sync --watch`](@/docs/cli-reference.md#sync).

## Path semantics

- `sources` and `outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus one root entry-point file per enabled target, all sharing the canonical pointer body. See the [per-target table](@/docs/target-behavior.md#entry-point-files). An ignored `.agnostic-ai.local/AGNOSTIC_AI.md` [extends that body](@/docs/local-overrides.md#extend-the-instructions) on one machine.

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

Specs load from three layers, lowest first. Higher layers override by spec name per kind; new names append. `agnostic-ai list` shows each spec's layer.

| Layer | Root | Loaded when |
|-------|------|-------------|
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai.local` | directory exists |

Only `project` honors custom `sources` paths. `.agnostic-ai.local/` stays out of Git by default and can extend `AGNOSTIC_AI.md`; see [local overrides](@/docs/local-overrides.md). `$AGNOSTIC_AI_HOME` is not a project layer.

## Global configuration

`agnostic-ai sync --global` installs user-level instructions, rules, hooks, and skills for 22 of the 25 targets ([global output](@/docs/target-behavior.md#global-output) lists paths). It works from any directory and loads no `agnostic-ai.yaml`, packs, or project specs. Personal overrides live in the source root's `local/` directory.

Source root: `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when `AGNOSTIC_AI_HOME` is unset.

```text
~/.agnostic-ai/
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer
    ├── AGNOSTIC_AI.md
    ├── agents/*.md
    ├── rules/*.md
    ├── hooks/*.yaml
    └── skills/<name>/SKILL.md
```

Specs in `local/` replace shared specs with the same kind and name. The local spec replaces the whole entry, including its metadata and skill assets; fields are never merged. New names append. `local/AGNOSTIC_AI.md` comes last in the managed instructions block, after shared agreements and effective rules. Without `local/`, sync uses the shared home alone.

Before adding personal files, add this entry to the source root's `.gitignore`:

```gitignore
/local/
```

For example, `local/skills/reviewer/SKILL.md` replaces `skills/reviewer/SKILL.md`. Run `agnostic-ai list --global` to see the effective specs with their `global` or `global-local` layer. Global layers never merge with project specs. [Local overrides](@/docs/local-overrides.md) compares this layer with the project one.

- It targets every supported tool by default. Which `sync` flags it accepts is in the [CLI reference](@/docs/cli-reference.md#sync).
- Nested rules and rules with scope, path, glob, or target conditions are rejected. Commands, MCP servers, settings, inheritance, and merging with project specs are unsupported.
- Global skills render native frontmatter and copy bundled assets verbatim. Claude resolves skill `model` and `effort`, including per-target maps and `x-claude` overrides. Shared directories such as `~/.agents/skills/` keep neutral frontmatter, even when syncing one target: target overrides are omitted.
- Global Codex skills also get `agents/openai.yaml`, so `x-codex.policy.allow_implicit_invocation: false` keeps a skill manual-only. A skill marked `disable-model-invocation: true` prints a coverage note for each target whose global copy stays model-invocable. Syncing one target keeps the files another target placed in a shared skills directory, so `--only amp` leaves Codex's policy in place.
- Hooks and skills honor `target`, `targets`, and `targets-exclude`. Set hook events for each target explicitly; sync does not translate event names.
- Agents use each target's native format, metadata overrides, and include/exclude filters. Eighteen targets have global agent output; see [global output](@/docs/target-behavior.md#global-output) for paths and discovery limits. Unsupported targets warn and skip agents. `readonly: true` maps to Codex's read-only sandbox; targets that drop `readonly` report a coverage note.
- Output is real files, never symlinks. Ownership is recorded per target in `$AGNOSTIC_AI_HOME/state/global.json`. Sync keeps unrelated text, JSON keys, hooks, skills, and agents, and removes only recorded artifacts for the targets in the run, so `--only` never sweeps another target. A hooks file whose managed entries did not change is left byte for byte; a rewrite keeps its key order and indent.
- An unmanaged agent, skill, or rule collision, damaged marker, invalid native JSON, or corrupt state stops the run before writes.
- A hand edit to a file sync owns, or to the managed block of an instructions file, also stops the run before writes and names the file. Move the edit into the source, or rerun with `--backup` to overwrite it and keep `<path>.bak`. Text outside the managed block is yours and never counts. State written before this check has nothing to compare, so the first sync after upgrading proceeds as before.
- A run without `--only` or explicit targets skips a target whose configuration root variable is relative, or whose native format rejects an agent name, and warns. Naming the target turns either into an error.
- Empty surfaces create nothing: no instructions file (a recorded one is removed) and no hooks file.
- Native tool precedence applies when global and project configuration both exist. Shared agent files remain until every owning target removes them. To update a file shared by Goose and OpenHands, sync both targets together.

Ordinary `agnostic-ai sync` does not load `~/.agnostic-ai/`. Move project-only defaults into a project's `.agnostic-ai/` or a pack, along with any agents, MCP servers, commands, settings, reviews, environments, or ignore specs. A repository's `.agnostic-ai/` stays project-specific despite the shared basename.

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
