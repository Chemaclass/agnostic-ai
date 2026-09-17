+++
title = "Configuration"
description = "Configure sources, targets, output overrides, sync behavior, imports, and global defaults."
weight = 130

[extra]
group = "Reference"
+++

# Configuration


`agnostic-ai.yaml` lives at the project root. It is read from the current working directory at command time. Every section is optional. Defaults are listed below.

Legacy filename: `agnostic.config.yaml` still loads, with a deprecation warning. Rename to `agnostic-ai.yaml` when convenient.

## Minimal project config

```yaml
version: 1
targets: [claude, cursor]
gitignore:
  enabled: true
```

Run `agnostic-ai init` to create a config with your selected tools. Source paths default to `.agnostic-ai/<kind>/`; add overrides only when needed.

For directory-specific instructions, keep this one project config and add `scope` to a rule. `agnostic-ai new rule payments-context --scope services/payments` scaffolds it. See [scoped context](@/docs/scoped-context.md) for target compatibility and output override limits. Set `on-unsupported: error` when every selected target must preserve scope.

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
| Understand which value wins | [Precedence](#precedence) and [Layered specs](#layered-specs) |
| Share personal instructions across projects | [Global configuration](#global-configuration) |
| Inspect all fields | [Top-level fields](#top-level-fields) or [JSON Schema](https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json) |

## Local overrides

Add `agnostic-ai.local.yaml` next to the base config for per-machine tweaks. It loads after the base and deep-merges: scalars and lists replace the base, maps merge recursively. `agnostic-ai init` adds the local filename to `.gitignore`.

```yaml
# agnostic-ai.local.yaml (never committed)
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

## Editor validation

The JSON Schema is published at `docs/schemas/config.schema.json`. `init` embeds this comment in the generated file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json
```

Editors with YAML Language Server support validate keys and values as you type.

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Reserved for future migrations. |
| `sources` | map | see below | Per-kind source directories (relative to config file). |
| `targets` | list | 20 default adapters | Adapter names to emit. Defaults to every adapter except `amp`, `warp`, `jules`, `goose`, and `augment`. Unknown targets log a warning and skip. |
| `outputs` | map | see below | Per-target output path overrides. |
| `on-unsupported` | string | `warn` | How to react when a kind is unsupported by a target. One of `warn`, `error`, `silent`. |
| `gitignore` | map | `enabled: false` | Auto-manage a block in `.gitignore` listing generated paths. See [`gitignore`](#gitignore). |
| `sync` | map | see below | Sync-level knobs. See [`sync`](#sync). |
| `verify` | map | disabled | External behavior gate. See [`verify`](#verify). |

## `sources`

Paths are relative to the config file. Missing directories are skipped silently.

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | `agents` | `*.md` agent specs. |
| `skills` | `skills` | `*.md` skill specs (or nested `<name>/SKILL.md`). |
| `rules` | `rules` | `*.md` rule specs. |
| `hooks` | `hooks` | `*.yaml` hook specs. |
| `mcps` | `mcps` | `*.yaml` MCP server specs. |

## `outputs`

`outputs.<target>.*` overrides where one target writes its files. Each target reads only the fields it understands and ignores the rest. Every target's page lists its keys and defaults, starting from the [targets index](@/docs/targets/_index.md). Claude Code and Codex also accept first-class settings blocks, documented on the [Claude Code](@/docs/targets/claude.md#claude-settings) and [Codex](@/docs/targets/codex.md#codex-config) pages.

```yaml
outputs:
  claude:
    rules-dir: .claude/rules
  cursor:
    mcp-file: .cursor/mcp.json
```

## `targets`

When omitted: the 20 default adapters (every adapter except `amp`, `warp`, `jules`, `goose`, and `augment`, left out of the default set so enabling them is a deliberate choice). Enabling any of them alongside `codex` is safe: the identical shared-path body is written once via auto-dedup. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

### Interactive target selection

`agnostic-ai init` opens a multi-select prompt when stdin is a TTY:

    agnostic-ai init

Use ↑/↓ to move, space to toggle, enter to confirm. The resulting `targets:` list contains only the chosen targets, in canonical order.

For scripted use (CI, integration tests), pipe a comma-separated line of target names:

    echo "claude,codex" | agnostic-ai init

Unknown names produce a clear error and leave the working tree untouched. Pass `--all` (`-a`) to skip the picker and enable every supported target. A non-TTY stdin with no piped data falls back to the full target list silently.

## `sync`

Sync-level knobs applied globally. Per-target overrides live in `outputs.<target>`.

### `sync.collision-policy` {#synccollision-policy}

Controls what happens when two enabled targets emit to the same output path.

| Value | Behavior |
|-------|----------|
| `prompt` | Default. Error with a resolution hint. On non-interactive stdin (CI), appends a hint to set a non-interactive policy. |
| `prefer-spec` | Skip the collision pre-flight. Last adapter wins. Use in CI when overlapping targets are intentional and last-writer-wins is acceptable. |
| `fail` | Hard error with no resolution hint. |

Per-target override: `outputs.<target>.collision-policy`.

```yaml
# In agnostic-ai.yaml
sync:
  collision-policy: prefer-spec   # CI-safe: skip collision check
```

### `sync.target-overview` {#synctarget-overview}

Off by default. When `true`, each target entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gains a generated appendix listing where that tool's generated artifacts live (rules dir, agents dir, MCP file, ...). The locations honor your `outputs.<target>.*` overrides.

```yaml
# In agnostic-ai.yaml
sync:
  target-overview: true
```

The canonical body stays identical across every target; only the appendix differs per file. An entry-point shared by several targets (codex, amp, warp, cline, windsurf, junie, kiro, crush, trae, jules, goose, augment, qoder, openhands, factory, kilo, and opencode all read `AGENTS.md`) lists each consumer in its own section. The appendix sits between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->` markers; `import` strips it, so the `AGNOSTIC_AI.md` round-trip stays lossless. `.agnostic-ai/AGNOSTIC_AI.md` itself never carries the appendix. Do not hand-edit the block: every sync regenerates it.

### `sync.resolve-imports`

Controls how `@path` file-import lines in the shared entry-point body (`AGNOSTIC_AI.md`) reach targets whose CLI does not resolve them. Claude resolves `@`-imports natively, so `CLAUDE.md` always keeps them verbatim. Every other target (codex and the rest reading `AGENTS.md`, `GEMINI.md`, ...) would otherwise carry a dead reference line.

```yaml
# In agnostic-ai.yaml
sync:
  resolve-imports: inline   # passthrough (default) | strip | inline
```

- `passthrough` (default): copy the `@`-line verbatim. The non-resolving target carries a dead reference, but nothing is dropped. Backward-compatible.
- `strip`: drop the `@`-line for non-resolving targets so the file is not littered with unfollowable references.
- `inline`: replace the `@`-line with the referenced file's content for non-resolving targets, wrapped between `<!-- agnostic-ai:import:start <path> -->` and `<!-- agnostic-ai:import:end -->`. `import` restores the lone `@`-line, so the `AGNOSTIC_AI.md` round-trip stays lossless. A missing or unreadable referenced file fails the sync.

Only a line that is a lone `@path` token is treated as an import; an `@mention` inside a sentence is left untouched. Paths resolve relative to the project root.

### `sync.dropped-summary`

Prints a per-target summary after sync of what each target could not fully emit: kinds it has no surface for (dropped) and kinds it emits only behind an opt-in key or keeps source-dir only (downgraded). The same information already prints grouped by kind (capability warnings and coverage notes); this regroups it by target so you can scan one tool at a time. Off by default.

```yaml
# In agnostic-ai.yaml
sync:
  dropped-summary: true
```

Example output:

```
  dropped summary (per target):
    cursor: 2 hooks dropped (unsupported)
    gemini: 3 skills via outputs.gemini.emit-skills-as-commands
```


Targets whose artifacts all flow through the entry-point pointer (aider) get no appendix. External adapters (`agnostic-ai-adapter-<name>` binaries) have no native-artifacts protocol yet, so their section is absent from the appendix.

### `sync.shared-skills` {#syncshared-skills}

Collapses byte-identical emitted skill folders into one canonical copy. Targets that share the Agent Skills layout (`<dir>/<name>/SKILL.md` plus bundled assets: Claude, Cursor, Codex, Amp) otherwise each get a full real copy of every skill. With the opt-in, sync keeps one real tree and replaces the others with per-skill relative symlinks. Off by default.

```yaml
# In agnostic-ai.yaml
sync:
  shared-skills: true
```

- The canonical copy prefers `.agents/skills/<name>` (the shared path Codex and Amp scan natively); otherwise the first emitted target keeps the real tree.
- Only folders whose rendered bytes are identical across targets are linked. A skill with per-target overrides (`x-cursor` frontmatter keys, codex-only `agents/openai.yaml`) keeps real copies for the diverging targets while identical ones still link.
- Links are per skill folder, not per skills directory, so hand-authored skills next to managed ones are never touched.
- Turning the option off (or a skill starting to diverge) converts links back to real trees on the next sync. Removing a skill sweeps its canonical tree and every link.
- On filesystems without symlink support (Windows without the privilege), sync warns once and keeps real copies.

### `sync.unmanaged` {#syncunmanaged}

Paths you own. `sync` never writes, merges, copies, or removes them.

```yaml
# In agnostic-ai.yaml
sync:
  unmanaged:
    - .cursor/rules/legacy.mdc        # exact path
    - .claude/agents/hand-*.md        # glob; `*` stays inside one path segment
    - .claude/skills/legacy/          # trailing slash: everything under the directory
```

- Entries are project-relative and use forward slashes; `\` is a glob escape, not a separator. A leading `./` or `/` is ignored. Globs use Go `path.Match`; `**` is not supported. A malformed glob or an entry that names no path (`.`, `./`, `/`, empty) fails config load.
- `sync` skips each matching output and prints `~ skip (unmanaged) <path>`. `--json` lists it under `skipped` with action `"unmanaged"`.
- `sync --check`, `status`, and `doctor` never report it as drift. `doctor` lists the entries in a `User-owned` block and leaves them out of the `Unmanaged config` block.
- The path stays out of the sync ledger, so removing the entry later deletes nothing. The next sync rewrites the file with the provenance header.
- `revert` and `doctor --fix` never restore or delete it.
- The managed `.gitignore` block does not collapse a directory that could hold a matching file, even one no spec renders. That directory's generated files are listed one per line instead, so git sees your file.
- With `sync.shared-skills`, a skill folder that could hold a matching file is never linked. An existing link there becomes a real copy of its current files on the next sync, so a file you edited through the link survives.
- The list is project-wide, not per target: one path such as `AGENTS.md` has many readers. `agnostic-ai.local.yaml` replaces the whole list, like every list in the local override.
- Not covered yet: hook script bodies copied from `.agnostic-ai/scripts/` into `.<tool>/hooks/`.

### Parallel emission (`--jobs`)

`sync` emits each target on its own worker. The `--jobs <n>` flag bounds how many run at once: `0` (the default) uses one worker per CPU, and `1` forces the old serial path. There is no config-file key; it is a per-run flag on `sync`.

The output never depends on the value. The emitted file tree, the summary counts, the JSON result, the `.gitignore` block, and the capability warnings are byte-identical whether one worker or many ran, so parallelism is safe to leave on. Drop to `--jobs 1` only when you want deterministic goroutine-free ordering for debugging. Targets that write a shared path (the `AGENTS.md` pointer family) are coordinated so exactly one target creates the file and the rest skip it, matching serial emission.

Parallelism helps most on large projects with many targets. On a small spec set or a single target it is close to free either way, since the worker count is capped at the number of targets.

### Actionable `--check` output (`--diff`, `--format`)

`sync --check` is the CI drift gate. Two per-run flags make a red build self-explanatory; there is no config-file key for either.

- `--diff` prints a unified diff per drifted file (on-disk vs what sync would write) instead of only counts. Off by default so CI logs stay lean; large diffs truncate with a summary line.
- `--format=github` emits GitHub Actions `::error file=...,line=...::` annotations so drift surfaces inline on the pull request. The default `human` format keeps the per-target table. `--json` still selects the machine-readable report and takes precedence.

A failing `--check` prints the reconcile command (`agnostic-ai sync`) on stderr and points its error at `agnostic-ai doctor` for a full diagnosis. Exit codes are unchanged: non-zero on drift in every format.

```bash
agnostic-ai sync --check --diff            # unified diff of every drifted file
agnostic-ai sync --check --format=github   # inline PR annotations in CI
```

## `verify`

`verify.command` is the external behavior gate run by `agnostic-ai verify`. Write it as an argv list. agnostic-ai starts the executable directly, without a shell, so pipes and redirects belong inside your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

The command runs once per selected target. Each run receives one JSON document through stdin. agnostic-ai preserves its stdout and stderr, then returns the same non-zero exit code when the verifier rejects the harness.

Before invoking the command, agnostic-ai runs the same drift check as `sync --check` for the selected targets. A missing or stale generated file stops verification. The verifier never sees an identity for files that do not match the specs.

The JSON names a `configured_model` only when agnostic-ai can read one from the rendered native target config. The value can come from portable Settings, first-class target config, or an imported native overlay. It describes configuration. It does not claim which model the CLI used at runtime. When a known CLI binary is present, `cli` includes its command, resolved path, and `--version` output. Missing or unreadable CLI identities are omitted.

The `harness_fingerprint` is a stable SHA-256 digest of target-relevant canonical specs and rendered target files. It identifies the harness under test. It is not an approval record, result cache, or score.

## `import`

Per-source knobs for the `import` command. Empty blocks fall back to per-source defaults.

### `import.codex.shred`

Controls how `agnostic-ai import codex` treats `AGENTS.md`.

| Value | Behavior |
|-------|----------|
| `true` | Default. Split each `AGENTS.md` into one rule spec per `##` heading. |
| `false` | Keep each `AGENTS.md` as a single rule spec (full body verbatim). Use when codex `AGENTS.md` duplicates policy already authored as standalone rules and you want it as a reference doc. |

```yaml
# In agnostic-ai.yaml
import:
  codex:
    shred: false   # one rule per AGENTS.md, no H2 sharding
```

## `on-unsupported`

Fires when an adapter receives spec kinds it does not support (e.g. `hooks` for Cursor or `mcps` for Cline).

| Value | Behavior |
|-------|----------|
| `warn` | Default. Log to stderr and continue. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

## Coverage notes

A target may declare support for a kind yet emit it only behind an opt-in key, or not at all by default. The content is not dropped, but it does not reach the target until you set the key. `sync` prints a `note:` line per gap so the gap is visible:

```
  note: 2 skills reach gemini, opencode only via outputs.<target>.emit-skills-as-commands
  note: 1 agent reaches warp only via outputs.warp.workflows-dir
```

A note fires only when specs of that kind are present and the opt-in is inactive. Setting the named key clears the note and emits the content. Notes that match the previous sync are suppressed; delete `.agnostic-ai/.sync-state` to re-show them.

The instrumented gaps:

| Target | Kind | Set this to emit |
|--------|------|------------------|
| `gemini` | skills | `outputs.gemini.emit-skills-as-commands` |
| `opencode` | skills | `outputs.opencode.emit-skills-as-commands` |
| `warp` | agents | `outputs.warp.workflows-dir` |
| `aider` | agents, skills | `outputs.aider.rules-file` |
| `zed` | hooks | `outputs.zed.tasks-file` |
| `kilo` | agents (only those with `tools` set) | no key; Kilo Code has no `tools` frontmatter key (use `x-kilo: {permission: {...}}` for native per-tool access control) |
| `gemini` | agents (only those whose `tools` names fall outside Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch) | no key; Gemini names its own tools, so write them with `x-gemini: {tools: [...]}`, or drop the field to inherit every tool from the parent session |
| `crush` | hooks (only those targeting an event other than `PreToolUse`) | no key; Crush's own runtime consumes `PreToolUse` only today |

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when the key is absent; `agnostic-ai init` writes `true` | When true, every `sync` rewrites a managed block in `.gitignore` listing every path the configured adapters emit. |
| `path` | `.gitignore` | Override the file location. Useful for monorepos or local-only ignore files. |
| `allow` | empty | Re-allow patterns emitted as `!`-prefixed lines at the end of the managed block. Keeps a tracked file (e.g. a `testdata/AGENTS.md` fixture) from being ignored by a broader rule, without hand-editing. Patterns are gitignore globs, emitted verbatim. |

The managed block is delimited by `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside the block are preserved as-is, and re-running `sync` with no spec changes is a no-op, leaving the file mtime unchanged.

Two comment lines head the block. The first says to edit specs, not the block. The second warns that the listed paths are not committed, so a fresh clone or `git worktree` lacks them until `sync` runs; wire it into a [post-checkout hook](@/docs/git-hooks.md#regenerate-on-checkout).

Every generated entry is root-anchored (`/AGENTS.md`, not `AGENTS.md`), so a generated file never ignores a same-named file nested elsewhere. Generated files under a tool subdirectory collapse to that subdirectory (`/.claude/rules/`, not one line per file). The collapse stops at the generated subdir, so a hand-authored sibling such as `.claude/settings.json` or `.claude/hooks/` is never swallowed by a `/.claude/` ignore.

For cases a root-anchored ignore cannot express, add the glob to `allow`. Its `!` line is written last, so it overrides the ignores above it.

The block also owns the three fixed agnostic-ai paths that are never generated by an adapter: `agnostic-ai.local.yaml` (the per-machine override), `/.agnostic-ai/.sync-state`, and `/.agnostic-ai/packs/`. `init` seeds them even with `gitignore.enabled: false`, since they must never be committed; `sync` keeps them alongside the generated entries. Projects created by an older version carried these as loose lines outside the block (with a duplicated `.sync-state`); the next `init`, `sync`, or `packs add` strips the loose copies and folds them into the block.

A target may also contribute local artifacts it creates but agnostic-ai never emits, so `sync` keeps them ignored without hand maintenance. When `claude` is an enabled target, the block includes `/.claude/agent-memory/` (the agent memory store) and `/.claude/settings.local.json` (per-user local settings); both follow the `outputs.claude.dir` override. These entries are target-scoped: a project that does not enable `claude` never sees them.

Override per-run with `--gitignore on|off` on `sync`.

### Picking the default at `init`

`agnostic-ai init` writes `gitignore.enabled: true` by default, so a fresh project keeps its generated outputs out of git and `.agnostic-ai/` stays the single committed source. When stdin is a TTY the confirm prompt defaults to "Yes, ignore them"; pick "No" to commit emitted files instead (useful when teammates lack the CLI). Non-interactive runs (CI, piped stdin) take the default without prompting. Opt out with `--gitignore=false`:

    agnostic-ai init --all --gitignore=false

Note this only affects newly scaffolded configs. An existing `agnostic-ai.yaml` with no `gitignore` key keeps the absent-key default (`false`); add the block by hand to opt in.

## Watched inputs

`sync --watch` re-emits whenever any of these change on disk:

- `agnostic-ai.yaml` (and `agnostic-ai.local.yaml` when present)
- every directory listed under `sources` (agents, skills, rules, hooks, mcps, commands, settings, reviews, environments, ignore)
- `.agnostic-ai.local/` (the project-user spec layer)
- `.agnostic-ai/overlays/`: captured per-target settings (`claude.settings.json`, `codex.config.toml`). Hand-edit an overlay to change something the spec layer does not own (Claude `statusLine`, Codex `[profiles.*]`, ...) and watch re-runs `sync` within the 50 ms debounce window.

See [`sync --watch`](@/docs/cli-reference.md#sync) for the polling fallback and debounce details.

## Path semantics

- All `sources`/`outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.
- Add generated outputs to `.gitignore` to keep specs as the single source of truth (recommended).

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus one root entry-point file per enabled target, all sharing the canonical pointer body. See the [per-target table](@/docs/targets/_index.md#entry-point-files) for which file each target uses.

### Per-target paragraphs

`.agnostic-ai/AGNOSTIC_AI.md` accepts the same `::target` / `::targets` / `::end` fences as spec bodies (see [Per-target body fences](@/docs/spec-format.md#per-target-body-fences)).

```md
Shared conventions for every tool.

::target gemini
Gemini reads `GEMINI.md` only. Load rules from `.gemini/rules/`.
::end

::targets codex amp
Run `make preflight` before you stop.
::end
```

- Content outside a fence goes to every entry-point file.
- A fenced block goes to a file when any target reading that file is listed. `AGENTS.md` has many readers (codex, amp, warp, cline, ...), so a `::target codex` block reaches all of them. A shared file is never split.
- Marker lines never reach the output. `.agnostic-ai/AGNOSTIC_AI.md` keeps them; it is the source.
- Markers must start at column 0. Indent a code sample that shows a marker.
- `agnostic-ai validate` flags a fence naming an unknown target, or a built-in target that reads no entry-point file.
- `agnostic-ai import <tool>` leaves a fenced source untouched when the imported entry point equals the view sync renders for the enabled targets (the default `sync.resolve-imports: passthrough`), so re-importing an untouched file never erases other tools' blocks. Otherwise import overwrites the source and warns that the fences are gone.

To opt back into the legacy concatenated layout for a target, set `outputs.<target>.rules-file: <path>`. The adapter writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so the two do not collide.

Real collisions where two adapters write different content to the same path (e.g. both `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`) fail fast with an `output collision` error by default. Set `sync.collision-policy: prefer-spec` to skip the check and let the last adapter win, useful in CI. See [`sync.collision-policy`](#synccollision-policy).

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml` (deep-merged over the base when present)
4. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Project specs load from three tiers, low- to high-precedence:

| Layer | Root | Loaded when |
|-------|------|-------------|
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai.local` | directory exists |

Higher layers override by spec name (per kind). New names append.

`project-user` uses the fixed kind directories under its root. Only the `project` layer honors custom `sources` paths. `$AGNOSTIC_AI_HOME` is not a project layer; `sync --global` reads it through the separate global workflow above.

Add `.agnostic-ai.local/` to your `.gitignore` so personal overrides stay local.

`agnostic-ai list` prints each spec's source layer for debugging.

## Global configuration

`agnostic-ai sync --global` syncs user-level instructions, rules, hooks, and skills to 22 of the 25 targets. It works from any directory and does not load `agnostic-ai.yaml`, packs, local overrides, or project specs. See [global output](@/docs/targets/_index.md#global-output) for the per-target paths and for the three targets that document no user-level surface.

The source root is `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when `AGNOSTIC_AI_HOME` is unset:

```text
~/.agnostic-ai/
├── AGNOSTIC_AI.md
├── rules/*.md
├── hooks/*.yaml
└── skills/<name>/SKILL.md
```

Run `agnostic-ai sync --global`. Every supported target is enabled by default. `--target`, `--only`, and `--except` narrow the set; naming a target with no user-level surface fails with the supported list. `--dry-run`, `--check`, and `--backup` retain their normal meaning. Project-only flags such as `--watch`, `--plan`, `--json`, `--gitignore`, and `--jobs` are rejected before any write.

Global rules must be unconditional. A nested rule or a rule with scope, path, glob, or target conditions is rejected rather than flattened. Global mode does not support agents, commands, MCP servers, settings, inheritance, or merging with project specs.

Generated files are real files, never symlinks. Managed instruction blocks, hook entries, and skill assets are recorded per target under `$AGNOSTIC_AI_HOME/state/global.json`. Sync preserves unrelated text, JSON keys, hooks, and skills. It removes only artifacts recorded as managed, and only for the targets in the current run, so `--only` never sweeps another target's output. An unmanaged skill or rule collision, damaged managed marker, invalid native JSON file, or corrupt ownership state stops the whole operation before writes. Native tool precedence still applies when both global and project configuration exist.

Nothing is created for a surface with no content. A target whose instructions body and rules are both empty gets no instructions file, and one already recorded is removed rather than left empty. A target with no hooks gets no hooks file seeded into its config directory.

Migration: ordinary `agnostic-ai sync` no longer loads specs from `~/.agnostic-ai/`. Rules, hooks, and skills already stored there become native global inputs when you run `sync --global`. Move defaults intended only for project output into each project's `.agnostic-ai/` tree or a shared pack. Move unsupported old global kinds such as agents, MCP servers, commands, settings, reviews, environments, and ignore specs into projects or packs because global mode does not load them.

A repository's `.agnostic-ai/` directory remains project-specific, even though the default user root has the same basename. Keep organization or team defaults in committed project specs or a pinned pack. `.agnostic-ai.local/` remains the uncommitted personal override for one project.
