+++
title = "Configuration"
description = "Choose tools, source folders, generated file paths, and settings for your project or machine."
weight = 130

[extra]
group = "Reference"
+++

# Configuration

`agnostic-ai.yaml` lives at the project root, and commands read it from the current directory. Every section is optional. The old name `agnostic.config.yaml` still loads with a warning; `agnostic-ai migrate` renames it.

## Minimal project config

```yaml
version: 1
targets: [claude, codex]
gitignore:
  enabled: true
```

`agnostic-ai init` creates one with your selected tools. A **target** is an AI coding tool that receives generated files. **Sources** are the specs you edit; **outputs** are the files those tools read.

After you change the config, run `agnostic-ai sync --dry-run` to see the planned output, then `agnostic-ai sync` and `agnostic-ai sync --check`. In scripts and coding agents, name the tools instead of relying on a prompt.

[Top-level fields](#top-level-fields) lists every key. For which value wins, see [precedence](#precedence) and [layered specs](#layered-specs). For directory-specific instructions, give a rule a `scope`; see [scoped context](@/docs/scoped-context.md).

## Local overrides

`agnostic-ai.local.yaml` holds changes for one machine. A single value or a list replaces the base value. A group of settings merges key by key. `init` adds the file to `.gitignore`. Personal specs go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

```yaml
# agnostic-ai.local.yaml (never committed)
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

## Editor validation

`init` adds this comment so editors validate the file against the schema of your release.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.72.0/docs/schemas/config.schema.json
```

## Top-level fields

An unknown key, in `agnostic-ai.local.yaml` too, fails every command that reads the config with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed). The error names the file, line, and closest known key.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Any value other than `1` fails with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed). |
| [`requires`](#requires) | string | none | Releases the specs work with: a minimum, one release, or a range. |
| [`sources`](#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | see [targets](#targets) | Tools to write files for. |
| [`builtins`](#built-ins) | list | none | Built-in specs to enable. `init` enables `handoff`. |
| [`outputs`](#outputs) | map | per target | Output path overrides. |
| [`models`](#models) | map | none | Model tiers that specs name. |
| [`on-unsupported`](#on-unsupported) | string | `warn` | What to do with a spec kind a tool lacks. |
| [`gitignore`](#gitignore) | map | `enabled: false` | Managed `.gitignore` block. |
| [`sync`](#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | A project command that `verify` runs. |
| [`import`](#import) | map | per source | Import behavior. |
| [`lint`](#lint) | map | see section | Budgets for always-loaded text. |
| [`doctor`](#doctor) | map | see section | Opt-in diagnostic checks. |
| [`coverage`](#coverage) | map | none | Coverage notes to accept, and whether notes fail sync. |
| [`memory`](#memory) | map | `personal: checkout` | Where personal memory lives. Set it in `agnostic-ai.local.yaml` only. |

## Built-ins

`builtins` turns on specs bundled with the binary. None are on unless listed; `init` writes `builtins: [handoff]` for a new project, or `[handoff, memory]` with `--memory`.

```yaml
requires: ">=0.80.0"
builtins: [handoff]
```

| Name | Adds | Setup and supported tools |
|---|---|---|
| `handoff` | A skill to leave and resume a task | [Session handoffs](@/docs/handoff.md) |
| `handoff-hook` | Automatic Git snapshots and startup notices | [Automatic snapshots](@/docs/handoff.md#add-automatic-git-snapshots) |
| `memory` | Shared project and personal memory | [Shared memory](@/docs/memory.md) |
| `rtk` | A Claude Code hook that shortens command output; install RTK separately | [RTK setup](@/docs/rtk-and-caveman.md#turn-them-on) |
| `caveman` | An on-demand response skill from a fixed upstream version; no separate program needed | [Caveman setup](@/docs/rtk-and-caveman.md#turn-them-on) |

Before you enable RTK or Caveman, [check for an existing copy](@/docs/rtk-and-caveman.md#if-you-already-use-rtk-or-caveman).

Unknown names fail as AAI-004 and list the valid names. A project, pack, or personal spec with the same kind and name wins over the built-in.

The same list works in the [global home config](#global-configuration). A `builtins` list in a local config replaces the shared list, and `builtins: []` there turns them all off. When you remove a name, the next sync removes the files and tool settings it manages, and keeps unrelated settings and project overrides. A local `builtins` list never changes the committed ignore files; see [block contents](#block-contents).

When you enable built-ins, set `requires` to at least 0.80.0 so an older binary cannot skip them. An upgrade that changes built-in text makes `sync --check` fail until you sync.

## `memory`

`memory.personal` sets where the `memory` built-in keeps personal memory. It is one user's choice, so set it in `agnostic-ai.local.yaml`; in `agnostic-ai.yaml` it fails as AAI-004.

| Value | Personal memory folder |
|-------|----------------|
| `checkout` (default) | `.agnostic-ai/local/memory/` in each checkout. |
| `repo` | `~/.agnostic-ai/local/memory/<repository name>-<hash>/`, one folder that every worktree of the repository shares (under `$AGNOSTIC_AI_HOME` when set). |

```yaml
# agnostic-ai.local.yaml
memory:
  personal: repo
```

In a Git project with managed ignores, [`init --memory`](@/docs/cli-reference/start.md#init) writes `memory.personal: repo` to a new `agnostic-ai.local.yaml`.

See [one store per repository](@/docs/memory.md#one-store-per-repository) for what sync writes and which tools can save there.

## `requires`

Pick one form:

```yaml
requires: ">=0.71.0"           # minimum: 0.71.0 or any newer release
requires: "0.73.0"             # exact: only 0.73.0; "=0.73.0" means the same
requires: ">=0.73.0 <0.74.0"   # range: 0.73.0 and its patch releases
```

- **Minimum**: the specs use a feature from that release, and generated files stay out of Git.
- **Exact**: you commit generated files. Another release can write different content, so `sync --check` would fail in CI. When npm installs the tool, pin the same release in `package.json`.
- **Range**: accept patch releases but not the next minor one.

A binary outside the value stops every command that reads your specs, before any write, with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires). That page lists the fixes. `sync --watch` stops when a pulled config puts the binary outside `requires`.

A value is one or more terms separated by spaces, and the binary must match every term. A term is `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`; anything else fails as AAI-004, naming the file. A build from source (`go run`, or a commit after a tag) is not a release, so it warns once instead.

`agnostic-ai.local.yaml` can replace the value. An empty `requires:` there turns the check off. The [global home config](#global-configuration) accepts the key too.

<details>
<summary>Older releases</summary>

Releases before 0.70.0 ignore the key. Releases before 0.74.0 read only `>=X.Y.Z` and stop on an exact or range value with AAI-004. Install the release the value names instead of editing `requires`.

</details>

## `sources`

Sync skips missing directories. Paths follow the [path rules](#path-semantics). A source root can be a directory symlink (macOS, Linux) or junction (Windows); sync does not follow links below it.

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | `agents` | `*.md` agent specs. |
| `skills` | `skills` | `*.md` skill specs (or nested `<name>/SKILL.md`). |
| `rules` | `rules` | `*.md` rule specs. |
| `hooks` | `hooks` | `*.yaml` hook specs. |
| `mcps` | `mcps` | `*.yaml` MCP server specs. |
| `commands` | `commands` | `*.md` saved prompts. |
| `settings` | `settings` | `*.yaml` permissions and defaults. |
| `reviews` | `reviews` | `*.md` review guidance. |
| `environments` | `environments` | `*.yaml` worktree and sandbox setup. |
| `ignore` | `ignore` | `*.md` context exclusion patterns. |

## `outputs`

`outputs.<target>.*` overrides where one target writes. An unknown field fails with AAI-004. Each target page lists its keys and defaults, starting at the [targets index](@/docs/targets/_index.md). [Claude Code](@/docs/targets/claude-reference.md#claude-settings) and [Codex](@/docs/targets/codex-reference.md#codex-config) also accept settings blocks.

```yaml
outputs:
  claude:
    rules-dir: .claude/rules
  cursor:
    mcp-file: .cursor/mcp.json
```

`outputs.<target>.agents: skill` writes agents as on-demand skills on Amp, Crush, Warp, and Zed, which have no subagents. See [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

`outputs.<target>.provenance-header: false` drops the `Generated by agnostic-ai` line from the files that target writes. A shared root file such as `AGENTS.md` drops it only when every target that reads the file sets it.

Codex has two keys that change how rules and permissions are written, `nested-glob-rules` and `exec-policies-from-permissions`. See [Codex config keys](@/docs/targets/codex-reference.md#config-keys).

## `models`

Name each model role (a tier) once, then use that name in specs instead of a vendor's model id:

```yaml
models:
  frontier: {claude: fable,  codex: astra, effort: xhigh}
  strong:   {claude: opus,   codex: sol,   effort: high}
  balanced: {claude: sonnet, codex: terra, effort: medium}
  fast:     {claude: haiku,  codex: luna,  effort: low}
```

An agent with `model: fast` gets `haiku` in Claude Code and `gpt-6-luna` in Codex, both at `low` effort. Other targets keep their default model. Skills, commands, and settings specs name tiers the same way.

Claude Code resolves `opus`, `sonnet`, `haiku`, and `fable` itself. Codex takes only exact ids, so agnostic-ai resolves these aliases when it writes Codex files:

| Codex alias | Model id |
|-------------|----------|
| `sol` | `gpt-6.1-sol` |
| `luna` | `gpt-6-luna` |
| `astra` | `gpt-6-astra` |
| `terra` | `gpt-6.1-terra` |

On v0.77.0 or earlier, write `terra` as `gpt-5.6-terra`.

Each release pins the model ids. When an upgrade changes one, the next `sync` prints `note: codex: sol now resolves to <new id> (was <old id>)` once. An alias works anywhere Codex reads a `model`: a tier, `model.codex`, a shared `model`, or a settings spec. `x-codex.model` is written as given.

| Tier key | Value |
|----------|-------|
| A target name | That target's model id. |
| `default` | The model id for each target the tier does not name. |
| `effort` | One effort for every target, or a per-target map such as `{claude: xhigh, codex: high}`. |

- A `model` that names no tier stays a literal model id. A tier with only `effort` keeps each tool's default model.
- A tier with neither a model nor `effort` fails to load, and so does any other key (AAI-004, naming the closest target).
- Per-spec overrides and which value wins: [model tiers](@/docs/spec-format/agents.md#model-tiers). `explain <spec>` shows the model and effort each target gets, and how an alias resolved.
- `lint` warns when a tier a spec names has no entry and no `default` for a target that writes the spec, a tier is named like a Claude model (LINT025), or a tier's `default` is a Claude model another target cannot load (LINT026).
- A tier in `agnostic-ai.local.yaml` replaces the same-name tier whole. The [global home config](#global-configuration) accepts the key too.

## `targets`

Default: every tool except `amp`, `warp`, `jules`, `goose`, `augment`, `ellipsis`, `grok`, `codebuddy`, and `kimi`. `-t/--target` replaces the list for one run.

A likely typo of a built-in target, such as `cursr`, fails before any write. Any other unknown name is skipped with a warning, since it may be an external adapter missing from this machine's PATH.

`init` and the first `sync` can write this list through a [picker](@/docs/cli-reference/sync.md#first-sync-target-picker).

## `sync`

Per-run flags such as `--diff`, `--format`, and `--jobs` have no config key. See [`sync`](@/docs/cli-reference/sync.md#sync) and [reading a failing `--check`](@/docs/cli-reference/sync.md#reading-a-failing---check).

| Key | Default | Effect |
|-----|---------|--------|
| [`collision-policy`](#synccollision-policy) | `prompt` | What happens when two targets write the same path. |
| [`target-overview`](#synctarget-overview) | `false` | Add a section to each entry-point file that lists where generated files live. |
| [`resolve-imports`](#syncresolve-imports) | `passthrough` | What tools that cannot resolve `@path` lines get. |
| [`dropped-summary`](#syncdropped-summary) | `false` | Print a per-target summary of dropped and downgraded kinds. |
| [`shared-skills`](#syncshared-skills) | `false` | Link identical skill folders to one copy. |
| [`unmanaged`](#syncunmanaged) | empty | Paths sync never touches. |
| [`output-manifest`](#syncoutput-manifest) | `false` | Write `.agnostic-ai/outputs.lock`, the committed list of generated paths. |
| [`allow-global-names`](#syncallow-global-names) | empty | Names shared with the global home that `sync` does not warn about. |
| [`global-name-clash`](#syncglobal-name-clash) | `warn` | Whether `sync` warns about names shared with the global home. |

### `sync.collision-policy` {#synccollision-policy}

Applies when two targets write different content to one path, such as `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`. Per-target override: `outputs.<target>.collision-policy`.

| Value | Behavior |
|-------|----------|
| `prompt` | Default. Fail with an `output collision` error and a hint. |
| `prefer-spec` | Write the version of the last tool in `targets:` that writes the file. `sync --check` expects that version. Sync prints one note per winning tool naming the files it won and the tools it overrode; reorder `targets:` to change it. Use in CI when the overlap is intentional. |
| `fail` | Hard error with no hint. |

```yaml
sync:
  collision-policy: prefer-spec
```

### `sync.target-overview` {#synctarget-overview}

When `true`, each entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gets a section listing where that tool's generated files live, following `outputs.<target>.*` overrides.

- A shared entry point such as `AGENTS.md` lists each reader in its own section.
- Sync rewrites the section between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->`. `import` removes it. Aider and external adapters get no section.

### `sync.resolve-imports` {#syncresolve-imports}

Sets what a tool that cannot resolve `@path` imports gets for a line in `AGNOSTIC_AI.md` that holds only an `@path` import. `CLAUDE.md` always keeps the line. An `@mention` inside a sentence or a fenced code block is untouched. Paths resolve from the project root.

| Value | Tools that cannot resolve it get |
|-------|---------------------------|
| `passthrough` | Default. The `@`-line as written, as a dead reference. |
| `strip` | Nothing: the line is dropped. |
| `inline` | The file's content between `<!-- agnostic-ai:import:start <path> -->` and `<!-- agnostic-ai:import:end -->`. `import` restores the `@`-line. A missing file fails the sync. |

```yaml
sync:
  resolve-imports: inline
```

### `sync.dropped-summary` {#syncdropped-summary}

When `true`, sync ends with a per-target list of kinds that were dropped (the tool has no support) or downgraded (written only behind an opt-in key, or only in the source directory). It groups capability warnings and [coverage notes](#coverage-notes) by tool.

### `sync.shared-skills` {#syncshared-skills}

When `true`, tools that share the Agent Skills layout (`<dir>/<name>/SKILL.md` plus assets: Claude, Cursor, Codex, Amp) keep one real copy of each skill and link the others to it with relative symlinks.

- Only folders with identical content link. Skills with per-tool overrides such as `x-cursor` keep real copies.
- Turning the option off restores real copies on the next sync.
- Without symlink support (Windows without the privilege), sync warns once and keeps real copies.

If you commit shared skill links, Git must check them out as links. Git's `core.symlinks` setting is separate from the system's permission to create links. With `core.symlinks=false`, `sync --check --against index` and `--against HEAD` name each link Git exported as a plain file. To fix it:

- Where the system can create links, run `git config --local core.symlinks true` and rerun the check.
- To keep `core.symlinks=false`, set `sync.shared-skills: false`, run `agnostic-ai sync`, and stage the config and the restored skill copies. Commit that before you check `--against HEAD`.

### `sync.unmanaged` {#syncunmanaged}

Paths you own. `sync` never writes, merges, copies, or removes them.

```yaml
sync:
  unmanaged:
    - .cursor/rules/legacy.mdc        # exact path
    - .claude/agents/hand-*.md        # glob; `*` stays inside one path segment
    - .claude/skills/legacy/          # trailing slash: everything under the directory
```

- Entries are project-relative with forward slashes. Filename patterns use Go `path.Match`; `**` is not supported. A malformed glob or an empty entry fails config load.
- `sync` prints `~ skip (unmanaged) <path>`. `sync --check`, `status`, and `doctor` never count it as drift. `revert` and `doctor --fix` never touch it.
- Removing the entry deletes nothing. The next sync rewrites the file.
- `sync.shared-skills` never links such a skill folder.
- `agnostic-ai.local.yaml` replaces the list whole.
- Hook script bodies copied from `.agnostic-ai/scripts/` are not covered.

### `sync.output-manifest` {#syncoutput-manifest}

Use it when you commit generated files. A full `sync` writes `.agnostic-ai/outputs.lock`: one line per generated path, with a checksum of what sync wrote. Commit it with the outputs; `sync --check` fails when it is missing or out of date.

```yaml
sync:
  output-manifest: true
```

- With no `.sync-state`, `doctor` and `sync --check` count a tracked file the generated-path list includes but no spec produces as a leftover. `doctor --fix` removes it.
- A file edited since sync wrote it no longer matches its checksum, so it is left for you.
- `sync --check --against` reads the generated-path list from the state it checks. It catches a deleted spec's outputs even in a shallow clone, unless that list was regenerated in the same commit.

### `sync.allow-global-names` {#syncallow-global-names}

Skill and agent names this project shares with the [global home](#global-shared-names) on purpose. `sync` prints no warning for a listed name. Other shared names still warn.

```yaml
# agnostic-ai.local.yaml
sync:
  allow-global-names: [gh-issue, gh-issues]
```

- Put the list in `agnostic-ai.local.yaml`, since the home exists only on your machine. A local list replaces the shared one whole.
- Names match the way Claude Code folds skill names: case, spacing, and invisible characters do not count.
- `doctor` still lists each allowed name under **Global names**, marked as allowed.

### `sync.global-name-clash` {#syncglobal-name-clash}

What `sync` does with every name this project shares with the [global home](#global-shared-names): `warn` prints the warning, `ignore` prints nothing.

```yaml
# agnostic-ai.local.yaml
sync:
  global-name-clash: ignore
```

- Use it when a project shadows global skills or agents as a rule. To accept a few names, use [`sync.allow-global-names`](#syncallow-global-names).
- `doctor` still lists each shared name under **Global names**, marked as ignored.
- Any other value fails config load.

## `verify`

`verify.command` lists the program to run and its arguments. `agnostic-ai verify` runs it without a shell, so put pipes and redirects in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

See the [`verify` command](@/docs/cli-reference/check.md#verify) for the drift check, input, exit codes, and result recording, which uses flags only.

## `import`

### `import.codex.shred`

Sets how `agnostic-ai import codex` treats a nested `AGENTS.md`. The root file always imports into `.agnostic-ai/AGNOSTIC_AI.md`.

| Value | Behavior |
|-------|----------|
| unset | Default. A hand-written file becomes one rule with its full body. A file `sync` wrote splits back into the rules it came from. |
| `true` | One rule spec per `##` heading, also for a hand-written file. |
| `false` | One rule spec per `AGENTS.md`, with the full body as written. |

```yaml
import:
  codex:
    shred: false
```

## `lint`

Budgets for the text each tool loads in every session. Past a budget, `agnostic-ai lint` warns and `lint --strict` exits 1.

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

The defaults follow vendor limits: Claude Code's [200 lines](https://code.claude.com/docs/en/memory) for `CLAUDE.md`, and the [Agent Skills specification](https://agentskills.io/specification) for descriptions. Lint also warns when text passes Codex's [32 KiB](https://developers.openai.com/codex/guides/agents-md) `AGENTS.md` cap or Antigravity's [24,000-byte](https://antigravity.google/docs/rules) rule cap, whatever the word budget.

A missing key or `0` keeps the default. A negative value fails as AAI-004. See [`lint`](@/docs/cli-reference/check.md#lint). The [global home config](#global-configuration) accepts the key too.

## `doctor`

### `doctor.check-references.ignore` {#doctorcheck-referencesignore}

Link destinations [`doctor --check-references`](@/docs/cli-reference/check.md#doctor) never reports as broken.

```yaml
doctor:
  check-references:
    ignore:
      - url                # exact destination, as written in the source Markdown
      - gcp-url
      - "*-placeholder"     # glob; `*` stays inside one path segment
```

An entry is the destination text as written, such as `url` for `[Logs](url)`, or a `path.Match` glob, as in [`sync.unmanaged`](#syncunmanaged).

## `on-unsupported`

Applies when a tool does not support a spec kind, such as `hooks` for Jules or project-level `mcps` for Cline.

| Value | Behavior |
|-------|----------|
| `warn` | Default. Log to stderr and continue. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

Any other value fails with AAI-004.

It also covers imported Claude hook root references that cannot be translated. See [project-root paths](@/docs/spec-format/hook-reference.md#imported-project-root-paths).

## Coverage notes

`sync` prints a `note:` line when a tool does not write specs of a kind you have, because it needs an opt-in key or has no files for them:

```
  note: 1 agent reaches warp only via outputs.warp.workflows-dir
```

Setting the named key clears the note. `sync -vv` shows every note.

| Target | Kind | Set this to write them |
|--------|------|------------------|
| `gemini` | skills | `outputs.gemini.emit-skills-as-commands` |
| `opencode` | skills | `outputs.opencode.emit-skills-as-commands` |
| `warp` | agents | `outputs.warp.workflows-dir` |
| `aider` | agents, skills | `outputs.aider.rules-file` |
| `zed` | hooks | `outputs.zed.tasks-file` |
| `kilo` | agents with `tools` | None. Use `x-kilo: {permission: {...}}`. |
| `gemini` | agents with `tools` beyond Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch | None. Use `x-gemini: {tools: [...]}` or drop `tools`. |
| `crush` | hooks not on `PreToolUse` | None. Crush runs `PreToolUse` only. |

## `coverage`

`on-unsupported` does not cover coverage notes, and its `error` never fails on them; these keys do. Both are project-only: `sync --global` warns about and ignores a `coverage:` key in the home config.

```yaml
coverage:
  fail-on-notes: true
  accept:
    - target: codex
      kind: agents
      field: tools
      reason: Codex limits come from sandbox_mode; tools stays portable for other targets.
    - target: [codex, gemini, cursor]
      kind: skills
      field: argument-hint
      reason: Only Claude Code shows the hint.
```

### `coverage.fail-on-notes` {#coveragefail-on-notes}

Default `false`. When `true`, every form of `sync` (including `--dry-run`, `--watch`, `--json`, `--check`, and `--plan`) fails on each coverage note that names a target and is not accepted. It prints the notes and rolls back the writes, even under `-q`.

### `coverage.accept` {#coverageaccept}

List each note the project accepts and the reason. Each entry names one note with exactly one of `field`, `via`, or `surface`.

| Field | Required | Description |
|-------|----------|-------------|
| `target` | yes | One target name, or a list. A name must be a built-in target or listed in `targets`. |
| `kind` | yes | `agents`, `skills`, `rules`, `hooks`, `mcps`, `commands`, `settings`, `reviews`, `environments`, or `ignores`. |
| `field` | one of three | Matches "`<field>` on N specs has no effect on `<target>`" notes for that field, whatever the reason. |
| `via` | one of three | Matches "N specs reach `<target>` only via `<via>`" notes, and "only in the source dir (`<via>`)" notes, with that exact text. |
| `surface` | one of three | Matches "N specs reach `<target>` but not `<surface>`" notes with that surface. |
| `reason` | yes | Why the note is expected. `sync -vv` prints it. |

An accepted note no longer prints on `sync`, and `fail-on-notes` ignores it. `sync -vv` shows it as `accepted:` with its reason. `doctor` counts it, and `doctor --json` reports `coverage_accepted`.

`lint` warns with LINT024 when an entry matches no note on one of its targets. Config loading fails on an unknown target, a repeated entry, or an entry with no `field`, `via`, or `surface`.

<details>
<summary>What coverage.accept cannot match</summary>

Notes that start with a target name, such as `note: codex: outputs.codex.config.notify is not written`, cover the whole setup. `coverage.accept` cannot match them, and `fail-on-notes` ignores them. A failure from `on-unsupported: error`, such as a Claude model name on another target, is not a note either.

</details>

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when absent; `agnostic-ai init` writes `true` | Every `sync` rewrites a managed `.gitignore` block listing every path the configured adapters write. |
| `path` | `.gitignore` | Another file, for monorepos or local-only ignore files. |
| `commit` | empty | Kinds of generated output to keep in Git, for every target or as `<target>:<kind>` for one. The block leaves out their paths. |
| `allow` | empty | Gitignore globs written as `!` lines at the end of the block, so a hand-written file at a generated path (such as a `testdata/AGENTS.md` fixture) is not ignored. |
| `worktree-include` | `true` | Keep the same block in `.worktreeinclude`. See [Claude Code worktrees](#claude-code-worktrees). |
| `ignore-worktree-include` | `false` | Keep `.worktreeinclude` out of Git. See [Claude Code worktrees](#claude-code-worktrees). |

`sync --gitignore` and `init --gitignore` override it per run. See the [CLI reference](@/docs/cli-reference/start.md#init).

Use `commit` for files that review bots and a fresh worktree read before any sync runs:

```yaml
gitignore:
  enabled: true
  commit: [instructions, hooks]
```

- Kinds: `instructions`, `agents`, `skills`, `commands`, `hooks`, `mcps`, `settings`, `reviews`, `environments`, and `ignores`. An unknown kind fails config loading.
- `instructions` covers entry-point files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) and rule outputs, root and scoped. Every other kind is named after its spec source.
- A file written by several kinds, such as `.claude/settings.json`, is committed when any of them is listed.
- `ellipsis:reviews`, `ellipsis:environments`, and `ellipsis:mcps` also count the settings spec that holds `x-ellipsis`, because `.ellipsis/code-review.yaml` and `.ellipsis/environments/<name>.yaml` need both ([Ellipsis](@/docs/targets/ellipsis.md#keep-the-files-in-git)).
- A file that only the config or an [overlay](#watched-inputs) produces belongs to no kind and stays ignored.

Prefix a kind with a target to commit it for that target only:

```yaml
gitignore:
  enabled: true
  commit: [cursor:reviews, cursor:environments]
```

This keeps `.cursor/environment.json`, `.cursor/worktrees.json`, and every `BUGBOT.md` in Git, and ignores `.claude/launch.json` and `.codex/environments/environment.toml`. `lint` warns (LINT017) when the target is not in `targets`.

The block sits between `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside it are kept.

### Claude Code worktrees

A new Claude Code worktree has no gitignored files. Claude Code copies the ones that [`.worktreeinclude`](https://code.claude.com/docs/en/worktrees#copy-gitignored-files-into-worktrees) lists from the main checkout.

With `claude` in `targets`, sync also keeps the block in `.worktreeinclude`, minus `.sync-state`, `.command-lock`, Claude worktree directories, and the task lock. Your local files and packs come along, so the worktree renders what the main checkout does.

- Set `gitignore.ignore-worktree-include: true` to keep the managed file out of Git by listing it in the managed ignore block. It requires `worktree-include: true` and `claude` in `targets`. If the file is already tracked, run `agnostic-ai sync --untrack` to remove it from the index and keep the file.
- Set `gitignore.worktree-include: false` to manage the file yourself. Sync then removes only its own block.

A fresh clone or `git worktree` lacks these paths until `sync` runs. See [checkout and merge hooks](@/docs/git-hooks.md#regenerate-on-checkout).

### Block contents

- Entries are root-anchored (`/AGENTS.md`), so nested same-named files are not ignored.
- Files collapse to their generated subdirectory (`/.claude/rules/`), never higher, so siblings such as `.claude/settings.json` stay visible. A per-kind dir such as `outputs.<target>.rules-dir` collapses at the dir itself.
- Output under a rule or review `scope` stays one line per file (`/services/api/AGENTS.md`).
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.command-lock`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai/local/`. `init` seeds them even with `gitignore.enabled: false`.
- A leftover file from a deleted spec stays ignored until you remove it.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings.
- Built-ins come from `agnostic-ai.yaml` alone, so a `builtins` list in `agnostic-ai.local.yaml` leaves the block and `.worktreeinclude` unchanged. Outputs only the local list adds go in an `agnostic-ai (personal)` block in `.git/info/exclude`, and sync removes those lines when the local list stops adding them. Outputs the local list turns off stay in the block. A linked worktree shares `.git/info/exclude` with every checkout, so there sync leaves those outputs untracked and names them in a warning.

## Watched inputs

`sync --watch` runs again when the config, sources, local specs, or overlays change. See [watch mode](@/docs/cli-reference/sync.md#watch-mode) for the full list. Overlays in `.agnostic-ai/overlays/` hold keys that specs do not cover, such as Claude `statusLine` or Codex `[history]`.

## Path semantics

- Relative `sources` and `outputs` paths start at the directory holding `agnostic-ai.yaml`. Absolute `sources` paths are used as written.
- Sync creates output directories as needed and overwrites existing files.

## Entry-point files

`sync` copies `.agnostic-ai/AGNOSTIC_AI.md` into one root entry-point file per enabled tool. See the [per-target table](@/docs/target-behavior.md#entry-point-files). `.agnostic-ai/local/AGNOSTIC_AI.md` [extends that body](@/docs/local-overrides.md#extend-the-instructions) on one machine.

Write the instructions every tool shares in `AGNOSTIC_AI.md`. Every session loads this text, so keep it to what an agent cannot infer from the code. Rules, agents, and skills stay in their own `sources` folders. Sync never overwrites this file; it overwrites only the root entry points.

When the file is missing, sync creates it with one line saying the tool files are generated from `.agnostic-ai/`. A project from an earlier release can still hold the old, long default text; `doctor` points it out. Replace it with your own instructions.

<details>
<summary>Legacy merged rules file</summary>

Setting `outputs.<target>.rules-file: <path>` restores the legacy layout: the adapter writes one merged document at `<path>`, including the built-in skill instructions. Two adapters writing different content to one path fail unless you set `sync.collision-policy: prefer-spec`.

When `<path>` is the target's own entry-point file (`outputs.claude.rules-file: CLAUDE.md`), sync skips the pointer body. Any other path keeps the entry-point file, and for `claude` the pointer body also gains an `@<path>` import.

</details>

### Per-target paragraphs

`.agnostic-ai/AGNOSTIC_AI.md` accepts the `::target` / `::targets` / `::end` fences from [spec bodies](@/docs/spec-format/_index.md#per-target-body-fences), with the same rules for shared entry-point files.

- `validate` flags a fence naming an unknown target or one that reads no entry-point file.
- For how `import` treats a fenced source, see [import](@/docs/cli-reference/start.md#files-and-source-order).

## Which value wins {#precedence}

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags (such as `agnostic-ai sync -t claude`)

## Layered specs

Specs load from four layers, lowest first. A higher layer overrides a spec of the same kind and name; a new name adds a spec. The `project-user` layer merges into the shared spec field by field (see [local overrides](@/docs/local-overrides.md#override-fields)). `agnostic-ai list` shows each spec's layer.

| Layer | Root | Loaded when |
|-------|------|-------------|
| `builtin` | bundled specs | enabled with `builtins` |
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai/local` | directory exists |

Only `project` uses custom `sources` paths. `$AGNOSTIC_AI_HOME` is not a project layer.

## Global configuration

`agnostic-ai sync --global` installs your own instructions, skills, hooks, settings, agents, and MCP servers for every project. It reads `~/.agnostic-ai/`, not the current project's specs. [Global configuration](@/docs/global-configuration.md) covers the source layout, target selection, ownership, and recovery.

```bash
agnostic-ai sync --global --only claude,codex --dry-run
agnostic-ai sync --global --only claude,codex
agnostic-ai sync --global --only claude,codex --check
```

### Shared names {#global-shared-names}

A home skill can hide a project skill with the same name. See [shared names](@/docs/global-configuration.md#global-shared-names).

### Default model and effort {#global-default-model-and-effort}

Home settings specs set user defaults. See [user settings](@/docs/global-configuration.md#global-default-model-and-effort).

### MCP servers {#global-mcp-servers}

See [user MCP servers](@/docs/global-configuration.md#global-mcp-servers).
