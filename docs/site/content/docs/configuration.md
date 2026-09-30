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

For directory-specific instructions, give a rule a `scope`; see [scoped context](@/docs/scoped-context.md). Set `on-unsupported: error` when every target must preserve scope.

## Find a setting

| Change | Section |
|---|---|
| Select tools | [Targets](#targets) |
| Change paths | [Sources](#sources), [Outputs](#outputs) |
| Keep generated files out of Git | [Gitignore](#gitignore) |
| Tune sync | [Sync](#sync), [`sync.unmanaged`](#syncunmanaged) |
| Gate on project checks | [Verify](#verify) |
| Block older binaries | [`requires`](#requires) |
| Instruction size warnings | [`lint`](#lint) |
| Per-machine or personal overrides | [Local overrides](#local-overrides), [Local spec layers](@/docs/local-overrides.md) |
| Which value wins | [Precedence](#precedence), [Layered specs](#layered-specs) |
| Cross-project instructions | [Global configuration](#global-configuration) |
| All fields | [Top-level fields](#top-level-fields), [JSON Schema](https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json) |

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

`init` adds this comment so editors validate the file against the schema of the release that wrote it.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.72.0/docs/schemas/config.schema.json
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
| [`doctor`](#doctor) | map | see section | Opt-in diagnostic checks. |

## `requires`

Set it when your specs rely on behavior from a specific release, or when the files you commit must come from one:

```yaml
requires: ">=0.71.0"    # this release or newer
requires: "0.73.0"      # exactly this release; "=0.73.0" means the same
requires: ">=0.73.0 <0.74.0"   # a range
```

Every command that reads your specs stops on a binary outside the value before it reads specs or writes files. The error is [AAI-005](@/docs/errors.md#aai-005-installed-version-older-than-requires) and names the command that installs a fitting release: `agnostic-ai upgrade` for a minimum, `agnostic-ai upgrade --version vX.Y.Z` when the value pins or bounds a release. A binary installed into the project's `node_modules` gets its package manager's command instead, such as `pnpm install`, picked from the lockfile. `sync --watch` stops when a pulled config puts the binary outside `requires`. Releases before 0.70.0 ignore the key. Releases before 0.74.0 read only `>=X.Y.Z` and stop on an exact or range value with AAI-004, whose text suggests `>=X.Y.Z`. Install the release the value names instead of editing `requires`.

A value is one or more terms separated by spaces, and every term must hold: `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`. Anything else fails as AAI-004 naming the file. A build from source (`go run`, or a commit after a tag) is not a release and warns once instead. `agnostic-ai.local.yaml` can replace the value, and an empty `requires:` there turns the check off. The [global home config](#global-configuration) accepts the key too.

To test a release candidate against a project that pins it, build it with the release it stands for:

```bash
go build -ldflags "-X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=X.Y.Z" -o agnostic-ai ./cmd/agnostic-ai
```

That build is checked as release X.Y.Z. Setting `main.version` changes only what `--version` prints.

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

`outputs.codex.nested-glob-rules` defaults to `true`: exact whole-subtree rule selectors write nested `AGENTS.md` files. Set it to `false` to inline those rules in the root as before. Filename filters and root-file selectors stay inline with an always-loaded note under `on-unsupported`. See [Codex rules](@/docs/targets/codex.md).

`outputs.<target>.*` overrides where one target writes; unknown fields are ignored. Target pages list the keys and defaults, starting at the [targets index](@/docs/targets/_index.md). [Claude Code](@/docs/targets/claude.md#claude-settings) and [Codex](@/docs/targets/codex.md#codex-config) also accept settings blocks.

```yaml
outputs:
  claude:
    rules-dir: .claude/rules
  cursor:
    mcp-file: .cursor/mcp.json
```

For Codex command rules, set `outputs.codex.exec-policies-from-permissions: true` to translate simple Bash entries from portable Settings specs and `outputs.claude.settings.permissions`. It defaults to `false`. Explicit inline, file, or imported Codex policies take precedence. Every translated rule matches a command prefix, including extra arguments, and sync notes each exact `allow` rule that [Codex widens](@/docs/targets/codex.md#translate-bash-permissions). See [Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions) for limits and LINT021 drift checks.

## `targets`

Default: every adapter except `amp`, `warp`, `jules`, `goose`, and `augment` (20 in total). Enabling those alongside `codex` is safe; the shared `AGENTS.md` body is written once. Unknown targets log a warning and are skipped. `-t/--target` overrides the list for one run.

`init` and the first `sync` can write this list through a [picker](@/docs/cli-reference/sync.md#first-sync-target-picker).

## `sync`

Per-run flags such as `--diff`, `--format`, and `--jobs` have no config key; see [`sync`](@/docs/cli-reference/sync.md#sync) and [reading a failing `--check`](@/docs/cli-reference/sync.md#reading-a-failing---check).

| Key | Default | Effect |
|-----|---------|--------|
| [`collision-policy`](#synccollision-policy) | `prompt` | What happens when two targets write the same path. |
| [`target-overview`](#synctarget-overview) | `false` | Append a generated-locations section to each entry-point file. |
| [`resolve-imports`](#syncresolve-imports) | `passthrough` | How `@path` lines reach targets that cannot resolve them. |
| [`dropped-summary`](#syncdropped-summary) | `false` | Print a per-target summary of dropped and downgraded kinds. |
| [`shared-skills`](#syncshared-skills) | `false` | Symlink byte-identical skill folders to one copy. |
| [`unmanaged`](#syncunmanaged) | empty | Paths sync never touches. |
| [`output-manifest`](#syncoutput-manifest) | `false` | Write `.agnostic-ai/outputs.lock`, the committed list of generated paths. |

### `sync.collision-policy` {#synccollision-policy}

Applies when two targets write different content to one path, such as `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`. Per-target override: `outputs.<target>.collision-policy`.

| Value | Behavior |
|-------|----------|
| `prompt` | Default. Fail with an `output collision` error and a hint (in CI, a non-interactive policy). |
| `prefer-spec` | Skip the collision check. Last adapter wins. Use in CI when the overlap is intentional. |
| `fail` | Hard error with no hint. |

```yaml
sync:
  collision-policy: prefer-spec
```

### `sync.target-overview` {#synctarget-overview}

When `true`, each entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gets an appendix listing where that tool's generated artifacts live. It honors `outputs.<target>.*` overrides.

```yaml
sync:
  target-overview: true
```

- A shared entry point such as `AGENTS.md` lists each reader in its own section.
- Every sync regenerates the appendix (between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->`), so do not hand-edit it. `import` strips it.
- Aider and external adapters get no appendix.

### `sync.resolve-imports` {#syncresolve-imports}

Controls how a line holding only an `@path` import in `AGNOSTIC_AI.md` reaches targets that cannot resolve it. `CLAUDE.md` always keeps it. An `@mention` inside a sentence is untouched. Paths resolve from the project root.

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

### `sync.shared-skills` {#syncshared-skills}

When `true`, targets sharing the Agent Skills layout (`<dir>/<name>/SKILL.md` plus assets: Claude, Cursor, Codex, Amp) keep one real tree per skill; the others get relative symlinks.

```yaml
sync:
  shared-skills: true
```

- The canonical copy is `.agents/skills/<name>` when emitted, otherwise the first emitted target's tree.
- Only identical rendered folders link. Per-target overrides such as `x-cursor` keep real copies.
- Turning the option off, or divergence, restores real trees on the next sync.
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

- Entries are project-relative with forward slashes. Globs use Go `path.Match`; `**` is not supported. A malformed glob or an empty entry fails config load.
- `sync` prints `~ skip (unmanaged) <path>`. `sync --check`, `status`, and `doctor` never count it as drift. `revert` and `doctor --fix` never touch it.
- Removing the entry deletes nothing; the next sync rewrites the file.
- `sync.shared-skills` never links such a skill folder.
- The list is project-wide. `agnostic-ai.local.yaml` replaces it whole.
- Hook script bodies copied from `.agnostic-ai/scripts/` are not covered.

### `sync.output-manifest` {#syncoutput-manifest}

For a repository that commits its generated files. A full `sync` writes `.agnostic-ai/outputs.lock`, one line per generated path with the content sum sync wrote, and `sync --check` fails when it is missing or out of date. Commit it with the outputs.

```yaml
sync:
  output-manifest: true
```

`.sync-state` is never committed, so CI has no record of what sync wrote, and a JSON output carries no header to prove it. The manifest is that record. With no `.sync-state`, `doctor` and `sync --check` count a tracked file the manifest lists but no spec produces as a leftover, and `doctor --fix` removes it. A file edited since sync wrote it no longer matches its sum, so it is left for you. `sync --check --against` reads the manifest of the state it checks, so it catches a deleted spec's outputs even in a one-commit shallow clone, as long as the manifest was not regenerated in the same commit.

## `verify`

`verify.command` is the argv list `agnostic-ai verify` runs. It starts the executable directly, without a shell, so pipes and redirects belong in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

See the [`verify` command](@/docs/cli-reference/check.md#verify) for the drift check, the JSON input, and exit codes.

## `import`

Per-source options for the `import` command. Empty blocks use per-source defaults.

### `import.codex.shred`

Controls how `agnostic-ai import codex` treats a nested `AGENTS.md`. The root file always lands in `.agnostic-ai/AGNOSTIC_AI.md`.

| Value | Behavior |
|-------|----------|
| unset | Default. A hand-written file becomes one rule with its full body, so sync writes it back as it was. A file `sync` wrote splits back into the rules it came from. |
| `true` | One rule spec per `##` heading, also for a hand-written file. |
| `false` | One rule spec per `AGENTS.md`, full body verbatim. |

```yaml
import:
  codex:
    shred: false
```

## `lint`

Budgets for the text each target loads in every session. Past a budget, `agnostic-ai lint` warns, and `lint --strict` exits 1.

| Key | Default | Effect |
|-----|---------|--------|
| `instructions-words` | `2000` | Words one target loads every session: entry-point file, always-on rule files, and skill and agent descriptions (LINT011). |
| `description-chars` | `1024` | Characters in one skill or agent description (LINT012). A skill description past 1024 still warns under a higher value. |
| `codex-chain-bytes` | `32768` | Bytes of `AGENTS.md` Codex reads for one directory: the root file plus each scoped file down to it, review sections included (LINT011). |

```yaml
lint:
  instructions-words: 3000
  description-chars: 500
  codex-chain-bytes: 24576
```

The defaults follow vendor limits: Claude Code [200 lines](https://code.claude.com/docs/en/memory) for `CLAUDE.md`, the [Agent Skills specification](https://agentskills.io/specification) for descriptions. Lint also warns past Codex's [32 KiB](https://developers.openai.com/codex/guides/agents-md) `AGENTS.md` cap and Antigravity's [24,000-byte](https://antigravity.google/docs/rules) rule cap, whatever the word budget.

A missing key or `0` keeps the default; a negative value fails as AAI-004. [`lint` in the CLI reference](@/docs/cli-reference/check.md#lint) shows the finding. The [global home config](#global-configuration) accepts the key too.

## `doctor`

### `doctor.check-references.ignore` {#doctorcheck-referencesignore}

Link destinations [`doctor --check-references`](@/docs/cli-reference/check.md#doctor) never reports as broken, whatever is on disk.

```yaml
doctor:
  check-references:
    ignore:
      - url                # exact destination, as written in the source Markdown
      - gcp-url
      - "*-placeholder"     # glob; `*` stays inside one path segment
```

An entry is the destination text as written (such as `url` for a placeholder link `[Logs](url)`) or a `path.Match` glob. Matching rules are the same as [`sync.unmanaged`](#syncunmanaged).

## `on-unsupported`

Applies when an adapter receives a spec kind it does not support (e.g. `hooks` for Cursor or `mcps` for Cline).

| Value | Behavior |
|-------|----------|
| `warn` | Default. Log to stderr and continue. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

Imported Claude hook root references that cannot be translated also follow this policy, including exec-form placeholders and complex shell expansions. See [project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths).

## Coverage notes

`sync` prints a `note:` line when specs of a kind exist but a target emits them only behind an inactive opt-in key, or not at all:

```
  note: 1 agent reaches warp only via outputs.warp.workflows-dir
```

Setting the named key clears the note. Repeated warnings collapse into one count line; `sync -v` shows them again.

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
| `commit` | empty | Kinds of generated output to keep in Git, for every target or as `<target>:<kind>` for one. The block leaves out their paths. |
| `allow` | empty | Gitignore globs written verbatim as `!` lines at the end of the block, so a hand-written file at a generated path (e.g. a `testdata/AGENTS.md` fixture) is not ignored. |
| `worktree-include` | `true` | With `claude` in `targets`, keep the same block in `.worktreeinclude`, so Claude Code copies the ignored outputs and the local layer into each worktree it creates. |
| `ignore-worktree-include` | `false` | Keep managing `.worktreeinclude` and list it in the managed ignore block. Requires `worktree-include: true` and `claude` in `targets`. |

`sync --gitignore` and `init --gitignore` override it per run; see the [CLI reference](@/docs/cli-reference/start.md#init).

`commit` accepts `instructions`, `agents`, `skills`, `commands`, `hooks`, `mcps`, `settings`, `reviews`, `environments`, and `ignores`. `instructions` covers entry-point files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) and rule outputs, root and scoped. Every other kind is named after its spec source. An unknown kind fails config loading. Use it to keep the files review bots and a fresh worktree read before any sync runs:

```yaml
gitignore:
  enabled: true
  commit: [instructions, hooks]
```

A file written by several kinds, such as `.claude/settings.json`, is committed when any of them is listed. A file only the config or an [overlay](#watched-inputs) produces belongs to no kind and stays ignored.

Prefix a kind with a target to commit it for that target only. A cloud agent or review bot reads its files from Git, while local tools regenerate theirs:

```yaml
gitignore:
  enabled: true
  commit: [cursor:reviews, cursor:environments]
```

This keeps `.cursor/environment.json`, `.cursor/worktrees.json`, and every `BUGBOT.md` in Git and ignores `.claude/launch.json` and `.codex/environments/environment.toml`. `lint` warns (LINT017) when the target is not in `targets`.

The block sits between `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside it are kept.

Claude Code builds a CLI `--worktree`, a subagent's, or a Desktop worktree from a checkout with no gitignored files, and Desktop runs no `WorktreeCreate` hook. It copies the gitignored files [`.worktreeinclude`](https://code.claude.com/docs/en/worktrees#copy-gitignored-files-into-worktrees) lists from the main checkout, so with `claude` in `targets` sync keeps the block there without `.sync-state`, Claude worktree directories, or the task lock. The local layer and packs come along, so the worktree renders what the main checkout does, and its first `sync --keep-edits` rewrites a copied output its specs no longer match. Set `gitignore.ignore-worktree-include: true` to keep the managed file out of Git. It then stops appearing under files to commit. If the file is already tracked, run `agnostic-ai sync --untrack` to remove it from the index while keeping the file. Set `gitignore.worktree-include: false` to manage the file yourself; sync then removes only its own block. A fresh clone or `git worktree` lacks these paths until `sync` runs (see [checkout and merge hooks](@/docs/git-hooks.md#regenerate-on-checkout)).

- Entries are root-anchored (`/AGENTS.md`), so nested same-named files are not ignored.
- Files collapse to their generated subdirectory (`/.claude/rules/`), never higher, so siblings such as `.claude/settings.json` stay visible. A per-kind dir such as `outputs.<target>.rules-dir` collapses at the dir itself.
- Output under a rule or review `scope` stays one line per file (`/services/api/AGENTS.md`), so new files in that directory are not ignored.
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai/local/`, seeded by `init` even with `gitignore.enabled: false`. `init`, `sync`, or `packs add` moves old loose copies into the block.
- A partial sync keeps every configured target's entries. When `.sync-state` lacks the skipped targets' outputs, as in a fresh clone, sync renders those targets in memory to list them.
- Kept orphans stay ignored until removed, even when a deleted spec narrows the emitted paths.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings.

## Watched inputs

`sync --watch` re-emits when the config files, any `sources` directory, `.agnostic-ai/local/`, or `.agnostic-ai/overlays/` change. Overlays hold keys the spec layer does not own, such as Claude `statusLine` or Codex `[history]`. See [`sync --watch`](@/docs/cli-reference/sync.md#sync).

## Path semantics

- `sources` and `outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.

## Entry-point files

`sync` copies `.agnostic-ai/AGNOSTIC_AI.md` into one root entry-point file per enabled target. See the [per-target table](@/docs/target-behavior.md#entry-point-files). An ignored `.agnostic-ai/local/AGNOSTIC_AI.md` [extends that body](@/docs/local-overrides.md#extend-the-instructions) on one machine.

Write the project instructions every tool shares in `AGNOSTIC_AI.md`. When the file is missing, sync seeds it with one line saying the tool files are generated from `.agnostic-ai/`, plus a placeholder comment. Every session loads this text, so keep it to what an agent cannot infer from the code. Rules, agents, and skills stay in their own `sources` folders. Sync keeps whatever you write here; it overwrites only the root entry points.

Projects created by an earlier release may still hold the old default text, a long description of agnostic-ai. `doctor` points it out; replace it with your own instructions.

Setting `outputs.<target>.rules-file: <path>` restores the legacy layout: the adapter writes one merged document at `<path>`. Two adapters writing different content to one path fail unless you set `sync.collision-policy: prefer-spec`.

Sync skips the pointer body only when `<path>` is the target's own entry-point file (`outputs.claude.rules-file: CLAUDE.md`). Any other path keeps the entry-point file. For `claude` the pointer body also gains an `@<path>` import.

### Per-target paragraphs

`.agnostic-ai/AGNOSTIC_AI.md` accepts the `::target` / `::targets` / `::end` fences from [spec bodies](@/docs/spec-format/_index.md#per-target-body-fences).

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
- Markers never reach output and must start at column 0.
- `validate` flags a fence naming an unknown target or one that reads no entry-point file.
- `import <tool>` keeps a fenced source when the imported file equals what sync renders (default `passthrough`); otherwise it overwrites the source and warns.

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Specs load from three layers, lowest first. Higher layers override by spec name per kind; new names append. The `project-user` layer merges into the shared spec field by field; see [local overrides](@/docs/local-overrides.md#override-fields). `agnostic-ai list` shows each spec's layer.

| Layer | Root | Loaded when |
|-------|------|-------------|
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai/local` | directory exists |

Only `project` honors custom `sources` paths. `.agnostic-ai/local/` stays out of Git by default and can extend `AGNOSTIC_AI.md`; see [local overrides](@/docs/local-overrides.md). `$AGNOSTIC_AI_HOME` is not a project layer.

## Global configuration

`agnostic-ai sync --global` installs user-level instructions, rules, hooks, and skills for 22 of the 25 targets ([global output](@/docs/target-behavior.md#global-output) lists paths). It works from any directory and loads no packs or project specs.

Source root: `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when unset.

```text
~/.agnostic-ai/
├── agnostic-ai.yaml        # optional: targets, requires, lint, on-unsupported
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── settings/*.yaml         # default model and effort
├── mcps/*.yaml             # user-level MCP servers
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer, same layout
```

Specs in `local/` merge into shared specs of the same kind and name, field by field, and a `::parent` line extends the shared body; see [local overrides](@/docs/local-overrides.md#override-fields). New names append. `local/AGNOSTIC_AI.md` comes last in the managed instructions block. Add `/local/` to the source root's `.gitignore` before adding personal files.

To sync a fixed set of tools without `--only` on every run, list them in the source root's `agnostic-ai.yaml`:

```yaml
targets: [claude, codex, cursor]
```

- `sync --global`, `lint --global`, and `validate --global` then use only those targets. A `targets` list in `local/agnostic-ai.yaml` replaces the shared one.
- `--only` and `--except` narrow the list for one run and must name configured targets. `--target` replaces it and skips the home config's `targets`.
- A target with no user-level surface, such as `aider` or `continue`, is skipped with one warning. An unknown name stops the run.
- [`requires`](#requires) stops the `--global` commands on an older binary. `local/agnostic-ai.yaml` replaces the shared value.
- `lint` sets the budgets `lint --global` uses.
- [`on-unsupported`](#on-unsupported) sets what `sync --global` does with a skill line that Claude Code expands and a target reads as plain text (see [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax)): `warn` prints a note, `error` fails the sync, `silent` hides it. `local/agnostic-ai.yaml` replaces the shared value.
- Other keys except `version` print a warning and are ignored.
- A target dropped from the list keeps its synced files and ownership records until you remove them by hand.

Run `agnostic-ai list --global` to see effective specs with their `global` or `global-local` layer, and `validate --global` and `lint --global` to check before a sync writes. Global layers never merge with project specs. [Local overrides](@/docs/local-overrides.md) compares this layer with the project one.

- Accepted `sync` flags are in the [CLI reference](@/docs/cli-reference/sync.md#sync).
- Nested rules, rules with scope, path, glob, or target conditions, commands, settings `permissions`, inheritance, and merging with project specs are unsupported.
- Skills render native frontmatter and copy bundled assets verbatim. Claude resolves skill `model` and `effort`, including per-target maps and `x-claude` overrides. Shared directories such as `~/.agents/skills/` keep neutral frontmatter.
- Codex skills also get `agents/openai.yaml`, so `disable-model-invocation: true` keeps a skill manual-only there. Targets whose copy stays model-invocable print a coverage note.
- Hooks and skills honor `target`, `targets`, and `targets-exclude`. Set hook events per target; sync does not translate event names.
- Eighteen targets have global agent output; see [global output](@/docs/target-behavior.md#global-output). Others warn and skip agents. `readonly: true` maps to Codex's read-only sandbox and Claude's `disallowedTools`.
- Output is real files, never symlinks. A user file that is itself a symlink (such as a dotfiles-managed `CLAUDE.md`) is written through. A symlink inside a skills, agents, or rules directory stops the run. Removing a spec behind a written-through symlink removes the link and the file it points at.
- Ownership is recorded per target in `$AGNOSTIC_AI_HOME/state/global.json`. Sync keeps unrelated content and removes only recorded artifacts for the targets in the run, so `--only` never sweeps another target.
- A managed hook or hooks file gone from disk is written again with a warning. A managed hook with the same matcher and command but other edits stops the run.
- A hand-written hook that exactly matches a source hook satisfies it and is never copied, recorded, or removed. One with the same matcher and command but other settings stops the run, even inside a group.
- An unrecorded file that holds exactly what sync would write is adopted. A file that differs by one byte stops the run.
- These stop the run before writes: an unmanaged agent, skill, or rule collision, a damaged marker, invalid native JSON, corrupt state, or state recorded under another `HOME`.
- A hand edit to a file sync owns, or to an instructions file's managed block, stops the run and names the file. Move the edit into the source, or rerun with `--backup` to overwrite it and keep `<path>.bak`. Text outside the managed block never counts.
- Without `--only`, explicit targets, or a home `targets` list, a target with a relative root variable or an invalid agent name is skipped with a warning. Naming the target makes it an error.
- Empty surfaces create nothing: no instructions file (a recorded one is removed) and no hooks file.
- Native tool precedence applies when global and project configuration both exist. Sync Goose and OpenHands together to update their shared agent file.

Ordinary `agnostic-ai sync` does not load `~/.agnostic-ai/`. Run inside the global source root (or under it), it stops before any write and points at `sync --global`. When `AGNOSTIC_AI_HOME` is your home directory itself, only that directory is guarded. A path through a symlink counts. `init`, `import`, `new`, `packs`, `cleanup`, `revert`, and `install-hook` stop the same way. Read-only commands such as `lint`, `validate`, and `doctor` still run there. Put project-only defaults in a project's `.agnostic-ai/` or a pack.

For a home kept in git, `install-hook --global` writes a pre-commit hook that runs `lint --global --strict`, `validate --global`, and `sync --global --check`. To start a home from what your tools already hold, run `agnostic-ai import --global`; see [import](@/docs/cli-reference/start.md#import).

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
| claude | `~/.claude/settings.json` | `model`, `effortLevel`, `permissions.defaultMode` |
| copilot | `~/.copilot/settings.json` | `model`, `effortLevel` |
| qoder | `~/.qoder/settings.json` | `model.name`, `model.reasoningEffort` |
| gemini | `~/.gemini/settings.json` | `model.name` |

Other targets and global permission lists raise a coverage note. Augment has no default model or effort key but takes `x-augment` keys, such as `shell`. Gemini has no default effort key, so `effort` raises a note there.

Accepted `effort` values:

- Claude and Copilot: `low`, `medium`, `high`, `xhigh`.
- Qoder: `disabled`, `off`, `none`, `low`, `medium`, `high`, `xhigh`, `max`.
- Codex: any string.

A dotted key is a nested JSON object: sync sets `name` inside `model` and leaves the other keys alone. `lint --global` (LINT014) and `validate --global` flag a value a target cannot take.

Set Claude Code's starting permission mode in a home settings spec:

```yaml
# ~/.agnostic-ai/settings/defaults.yaml
permissions:
  default-mode: acceptEdits
```

`sync --global --only claude` writes `permissions.defaultMode` in `~/.claude/settings.json` (or under `CLAUDE_CONFIG_DIR`). It accepts `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, and `bypassPermissions`. A later settings spec wins. Sync owns only this key, so hand-written `allow`, `deny`, and `ask` rules stay. Removing the field removes the managed mode unless you changed it by hand.

Other targets report a coverage note. For Codex, set `x-codex.approval_policy` and `x-codex.sandbox_mode` instead. Claude's `auto` and `bypassPermissions` need user, managed, or session settings; project settings cannot enable them. See [Claude's mode reference](https://code.claude.com/docs/en/settings-reference#permissions-defaultmode).

Each layer overrides the one before it:

1. `settings/*.yaml` in the home.
2. `local/settings/*.yaml`. A same-named file merges into the shared one field by field. A new file comes after the shared ones, so its `model` and `effort` win.
3. The project tier: a project's settings file, such as `.codex/config.toml` or `.claude/settings.json`, wins through the tool's own precedence.
4. An agent's own `model` and `effort`, for that agent.
5. The tool's flag for one run, such as `codex -m` or `claude --model`.

Sync edits only the keys it writes and records them in `state/global.json`. Every other line stays byte for byte. Removing the spec removes only those keys, and a file sync created goes once nothing is left in it.

- A key that already holds the value sync would write is adopted.
- A key with another value stops the run and names the file, key, and both values. Codex's `/model` picker saves to `config.toml`, so this is normal: the message prints the line to put in the spec. `--backup` overwrites the key and keeps `<path>.bak`.
- `--dry-run` lists each key a write sets or removes, and `--check` fails on a changed key.
- An `x-<target>` block sets that target's own keys in the same file. Nested objects merge leaf by leaf, and an `x-claude` key wins over the portable field it shares a key with. Global sync writes Codex top-level scalars and arrays, such as `x-codex.notify`, to `~/.codex/config.toml`; tables such as `profiles`, `x-claude.hooks`, and `x-claude.permissions` raise a coverage note. Project sync does not route `x-codex` into `.codex/config.toml`; it raises a coverage note instead. A later spec wins key by key, and `null` drops an earlier key.
- `agnostic-ai explain --global settings/defaults.yaml` names the file and key each target gets.

### MCP servers {#global-mcp-servers}

MCP specs in the home's `mcps/` install each server in the user MCP file of every target that has one, rendered as the target's project MCP file renders it:

| Target | File | Where |
|---|---|---|
| claude | `~/.claude.json` (`$CLAUDE_CONFIG_DIR/.claude.json` when set) | top-level `mcpServers.<name>` |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]` table |
| cursor | `~/.cursor/mcp.json` | `mcpServers.<name>` |
| copilot | `~/.copilot/mcp-config.json` | `mcpServers.<name>`, with `tools: ["*"]` when the spec sets none |
| gemini | `~/.gemini/settings.json` | `mcpServers.<name>` |
| qoder | `~/.qoder/settings.json` | `mcpServers.<name>` |
| augment | `~/.augment/settings.json` | `mcpServers.<name>` |
| openhands | `~/.openhands/mcp.json` (`$OPENHANDS_PERSISTENCE_DIR/mcp.json` when set) | `mcpServers.<name>`: `{command, args, env}` or `{url, transport, headers, auth}` |

Each server follows the per-key rules above. A hand-written server that means the same as the spec is adopted; a different one with the same name stops the run (`--backup` overwrites it). A server sync wrote goes when its spec goes; servers you add under other names stay.

- Sync edits only its own `mcpServers` entries in `~/.claude.json`, keeps every other key and the file's permissions, and creates a missing file at `0600`. It stops without writing if the file changed after it was read; rerun.
- A spec with `disabled: true` stays out of the Augment, Claude, Cursor, Copilot, and OpenHands user files, where a listed server is live in every project.
- When `CLAUDE_CONFIG_DIR` or another root variable moves a file, the next sync removes its entries from the old one.

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

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`. If a manually copied file occupies an output path, save its edits in the source and move it aside first. `--backup` does not bypass unmanaged collisions.
