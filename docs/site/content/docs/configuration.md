+++
title = "Configuration"
description = "Configure sources, targets, output overrides, sync behavior, imports, and global defaults."
weight = 130

[extra]
group = "Reference"
+++

# Configuration

`agnostic-ai.yaml` lives at the project root. Commands read it from the current directory. Every section is optional. The old filename `agnostic.config.yaml` still loads with a warning. Run `agnostic-ai migrate` to rename it.

## Minimal project config

```yaml
version: 1
targets: [claude, cursor]
gitignore:
  enabled: true
```

`agnostic-ai init` creates one with your selected tools.

For directory-specific instructions, give a rule a `scope`. See [scoped context](@/docs/scoped-context.md).

## Find a setting

| Change | Section |
|---|---|
| Select tools | [Targets](#targets) |
| Share session state across tools | [Built-ins](#built-ins) |
| Change paths | [Sources](#sources), [Outputs](#outputs) |
| Keep generated files out of Git | [Gitignore](#gitignore) |
| Tune sync | [Sync](#sync) |
| Pin the agnostic-ai release | [`requires`](#requires) |
| Silence a known coverage note, or fail on the rest | [`coverage`](#coverage) |
| Per-machine or personal overrides | [Local overrides](#local-overrides) |
| Which value wins | [Which value wins](#precedence), [Layered specs](#layered-specs) |
| Cross-project instructions | [Global configuration](#global-configuration) |
| All fields | [Top-level fields](#top-level-fields), [JSON Schema](https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json) |

## Local overrides

`agnostic-ai.local.yaml` holds per-machine changes. A single value or list replaces the base value. A group of settings merges key by key. `init` adds the file to `.gitignore`. Personal specs go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

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
| `version` | int | `1` | Schema version, reserved for migrations. |
| [`requires`](#requires) | string | none | Releases the specs work with: a minimum, one release, or a range. |
| [`sources`](#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | 20 tools | Tools to write files for. |
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

`builtins` turns on specs bundled with the binary. Existing projects have none on. `init` writes `builtins: [handoff]` for new projects.

```yaml
requires: ">=0.80.0"
builtins: [handoff]
```

The names are `handoff` (the skill), `handoff-hook` (optional Git snapshots and resume notices), `memory` (a project memory every tool shares), `rtk` (a Claude hook that shortens command output), and `caveman` (a response skill copied from a fixed upstream version). See [session handoffs](@/docs/handoff.md), [shared memory](@/docs/memory.md), and [RTK and Caveman](@/docs/rtk-and-caveman.md) for supported tools and any programs you need to install. Unknown names fail as AAI-004 and list the valid names. A project, pack, or personal spec with the same kind and name wins over the built-in.

To add the hooks, enable both:

```yaml
builtins: [handoff, handoff-hook]
```

RTK and Caveman are off unless named, including in projects created by `init`. Enable either independently, or both:

```yaml
builtins: [rtk, caveman]
```

`rtk` writes a Claude `PreToolUse` hook, which runs before `Bash` commands. It requires a shell compatible with POSIX, such as `sh`. It does not handle the PowerShell tool. Install RTK separately. If the hook cannot find RTK on its `PATH`, it leaves the command unchanged. Sync does not search for, run, download, or install RTK. It writes the same hook whether RTK is installed or not. Other targets receive no RTK hook.

`caveman` adds a response skill for tools that support skills. Invoke `/caveman` on Claude, or use your tool's way of invoking skills. No Caveman program is needed. This skill does not shorten command output, route requests through another server, or add session hooks. Its source, notices, and licenses come unchanged from a fixed upstream version. If you already have a global hook, installed skill, or example pack, choose which copy to keep before enabling the built-in. Keeping two copies can run RTK twice or show duplicate skills. See [choose one setup](@/docs/rtk-and-caveman.md). When an existing project hook exactly matches the RTK hook sync would generate, sync takes over managing it. Removing `rtk` later removes that hook. Leave the built-in off to keep managing the existing hook yourself, or save a copy before enabling it.

The same list works in the [global home config](#global-configuration). A `builtins` list in a local config replaces the shared list, and `builtins: []` turns them all off there. Removing a name removes the files or tool settings it manages on the next sync. Unrelated settings and project overrides remain.

Set `requires` to at least 0.80.0 when you enable built-ins, so an older binary cannot skip them. An upgrade that changes built-in text makes `sync --check` fail until you sync.

## `memory`

`memory.personal` sets where the `memory` built-in keeps personal memory. It is one user's choice, so it belongs in `agnostic-ai.local.yaml`. In `agnostic-ai.yaml` it fails as AAI-004.

| Value | Personal memory folder |
|-------|----------------|
| `checkout` (default) | `.agnostic-ai/local/memory/` in each checkout. |
| `repo` | `~/.agnostic-ai/local/memory/<repository name>-<hash>/`, one folder that every worktree of the repository shares (under `$AGNOSTIC_AI_HOME` when set). |

```yaml
# agnostic-ai.local.yaml
memory:
  personal: repo
```

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

A binary outside the value stops every command that reads your specs, before it writes anything, with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires). The fix depends on the installed release:

- **Newer than the project permits:** `agnostic-ai upgrade --requires` adopts the installed release. It sets an exact pin and matching schema URL, then syncs. Run it after your package-manager upgrade, with that manager's CLI, such as `pnpm exec agnostic-ai upgrade --requires`.
- **Older than the project needs:** install a fitting release with `agnostic-ai upgrade`, `upgrade --version vX.Y.Z`, or the project's package manager. The message names the command.

`upgrade --requires` updates the base config and any existing local override. It handles project config only. For a global home pin, edit the file the error names. `sync --watch` stops when a pulled config puts the binary outside `requires`.

A value is one or more space-separated terms. Every term must match: `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`. Anything else fails as AAI-004, naming the file. A build from source (`go run`, or a commit after a tag) is not a release, so it only warns once.

`agnostic-ai.local.yaml` can replace the value. An empty `requires:` there turns the check off. The [global home config](#global-configuration) accepts the key too.

{% <details summary="Older releases"> %}
Releases before 0.70.0 ignore the key. Releases before 0.74.0 read only `>=X.Y.Z` and stop on an exact or range value with AAI-004. Install the release the value names instead of editing `requires`.
{% </details> %}

## `sources`

Missing directories are skipped. See [path semantics](#path-semantics). A source root can be a directory symlink (macOS, Linux) or junction (Windows). Links below a source root are not followed.

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | `agents` | `*.md` agent specs. |
| `skills` | `skills` | `*.md` skill specs (or nested `<name>/SKILL.md`). |
| `rules` | `rules` | `*.md` rule specs. |
| `hooks` | `hooks` | `*.yaml` hook specs. |
| `mcps` | `mcps` | `*.yaml` MCP server specs. |

## `outputs`

`outputs.<target>.*` overrides where one target writes. An unknown field fails with AAI-004. Each target page lists its keys and defaults, starting at the [targets index](@/docs/targets/_index.md). [Claude Code](@/docs/targets/claude.md#claude-settings) and [Codex](@/docs/targets/codex.md#codex-config) also accept settings blocks.

```yaml
outputs:
  claude:
    rules-dir: .claude/rules
  cursor:
    mcp-file: .cursor/mcp.json
```

`outputs.<target>.agents: skill` writes agents as on-demand skills on Amp, Crush, Warp, and Zed, which have no subagents. See [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

`outputs.kiro.commands-dir` overrides `.kiro/prompts/` for project commands. See [Kiro commands](@/docs/targets/kiro.md#commands).

Two Codex keys change how rules and permissions are written:

- `outputs.codex.nested-glob-rules` defaults to `true`: a rule scoped to exactly one whole directory tree gets a nested `AGENTS.md`. Set it to `false` to put those rules in the root file. See [Codex rules](@/docs/targets/codex.md).
- `outputs.codex.exec-policies-from-permissions` defaults to `false`. Set it to `true` to turn simple Bash entries from portable settings specs and `outputs.claude.settings.permissions` into Codex command rules. Codex policies you write inline, in a file, or import win. See [Bash permission translation](@/docs/targets/codex.md#translate-bash-permissions) for widening, limits, and LINT021 drift checks.

## `models`

Name each model role once, then write the role in specs instead of a vendor's model id:

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
| `terra` | `gpt-5.6-terra` |

On v0.77.0 or earlier, write `terra` as `gpt-5.6-terra`.

Each release fixes the ids. An upgrade can change them, and the next `sync` prints `note: codex: sol now resolves to <new id> (was <old id>)` once. An alias works anywhere Codex reads a `model`: a tier, `model.codex`, a shared `model`, or a settings spec. `x-codex.model` is written as given.

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

Default: every tool except `amp`, `warp`, `jules`, `goose`, and `augment` (20 in total). `-t/--target` overrides the list for one run.

A likely typo of a built-in target, such as `cursr`, fails before any file is written. Any other unknown name logs a warning and is skipped, since it may be an external adapter missing from this machine's PATH.

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
| `prefer-spec` | Skip the collision check. The last tool to write the file wins. Use in CI when the overlap is intentional. |
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

Sets what tools that cannot resolve `@path` imports get for a line in `AGNOSTIC_AI.md` that holds only an `@path` import. `CLAUDE.md` always keeps it. An `@mention` inside a sentence or a fenced code block is untouched. Paths resolve from the project root.

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
- Removing the entry deletes nothing. The next sync rewrites the file.
- `sync.shared-skills` never links such a skill folder.
- `agnostic-ai.local.yaml` replaces the list whole.
- Hook script bodies copied from `.agnostic-ai/scripts/` are not covered.

### `sync.output-manifest` {#syncoutput-manifest}

For repositories that commit their generated files. A full `sync` writes `.agnostic-ai/outputs.lock`: one line per generated path, with the content sum sync wrote. `sync --check` fails when the file is missing or out of date. Commit it with the outputs.

```yaml
sync:
  output-manifest: true
```

- With no `.sync-state`, `doctor` and `sync --check` count a tracked file the generated-path list includes but no spec produces as a leftover. `doctor --fix` removes it.
- A file edited since sync wrote it no longer matches its sum, so it is left for you.
- `sync --check --against` reads the generated-path list from the state it checks. It catches a deleted spec's outputs even in a shallow clone, unless that list was regenerated in the same commit.

### `sync.allow-global-names` {#syncallow-global-names}

Skill and agent names this project shares with the [global home](#global-shared-names) on purpose. `sync` prints no warning for a listed name. Other shared names still warn.

```yaml
# agnostic-ai.local.yaml
sync:
  allow-global-names: [gh-issue, gh-issues]
```

- The home exists only on your machine, so `agnostic-ai.local.yaml` is the usual place for the list. It replaces the list whole.
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

`verify.command` is the argv list `agnostic-ai verify` runs, without a shell. Put pipes and redirects in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

See the [`verify` command](@/docs/cli-reference/check.md#verify) for the drift check, input, and exit codes.

## `import`

Per-source options for the `import` command.

### `import.codex.shred`

Sets how `agnostic-ai import codex` treats a nested `AGENTS.md`. The root file always lands in `.agnostic-ai/AGNOSTIC_AI.md`.

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

Budgets for the text each tool loads in every session. Past a budget, `agnostic-ai lint` warns, and `lint --strict` exits 1.

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

Applies when a tool does not support a spec kind, such as `hooks` for Cursor or `mcps` for Cline.

| Value | Behavior |
|-------|----------|
| `warn` | Default. Log to stderr and continue. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

Any other value fails with AAI-004.

It also covers imported Claude hook root references that cannot be translated. See [project-root paths](@/docs/spec-format/hooks.md#imported-project-root-paths).

## Coverage notes

`sync` prints a `note:` line when specs of a kind exist but a tool does not write them, because it needs an opt-in key or has no files for them:

```
  note: 1 agent reaches warp only via outputs.warp.workflows-dir
```

Setting the named key clears the note. `sync -v` shows every note.

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

`on-unsupported` does not cover coverage notes, and `error` never fails on them. These keys do. Both are project-only: `sync --global` ignores a `coverage:` key in the home config and warns.

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

Default `false`. When `true`, sync fails on each coverage note that names a target and is not accepted. It prints the notes and rolls back the writes, even under `-q`. This applies to `sync`, `sync --dry-run`, `sync --watch`, `sync --json`, `sync --check`, `sync --check --json`, and `sync --plan`.

### `coverage.accept` {#coverageaccept}

List a note the project already chose to live with under `coverage.accept`, with the reason. Each entry names one note with exactly one of `field`, `via`, or `surface`.

| Field | Required | Description |
|-------|----------|-------------|
| `target` | yes | One target name, or a list. A name must be a built-in target or listed in `targets`. |
| `kind` | yes | `agents`, `skills`, `rules`, `hooks`, `mcps`, `commands`, `settings`, `reviews`, `environments`, or `ignores`. |
| `field` | one of three | Matches "`<field>` on N specs has no effect on `<target>`" notes for that field, whatever the reason. |
| `via` | one of three | Matches "N specs reach `<target>` only via `<via>`" notes, and "only in the source dir (`<via>`)" notes, with that exact text. |
| `surface` | one of three | Matches "N specs reach `<target>` but not `<surface>`" notes with that surface. |
| `reason` | yes | Why the note is expected. `sync -v` prints it. |

An accepted note no longer prints on `sync`, and `fail-on-notes` ignores it. `sync -v` shows it as `accepted:` with its reason. `doctor` counts it, and `doctor --json` reports `coverage_accepted`.

`lint` warns with LINT024 when an entry matches no note on one of its targets. Config loading fails on an unknown target, a repeated entry, or an entry with no `field`, `via`, or `surface`.

{% <details summary="What coverage.accept cannot match"> %}
Notes that start with a target name, such as `note: codex: outputs.codex.config.notify is not written`, cover the whole setup. `coverage.accept` cannot match them, and `fail-on-notes` ignores them. A failure from `on-unsupported: error`, such as a Claude model name on another target, is not a note either.
{% </details> %}

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when absent; `agnostic-ai init` writes `true` | Every `sync` rewrites a managed `.gitignore` block listing every path the configured adapters write. |
| `path` | `.gitignore` | Another file, for monorepos or local-only ignore files. |
| `commit` | empty | Kinds of generated output to keep in Git, for every target or as `<target>:<kind>` for one. The block leaves out their paths. |
| `allow` | empty | Gitignore globs written as `!` lines at the end of the block, so a hand-written file at a generated path (such as a `testdata/AGENTS.md` fixture) is not ignored. |
| `worktree-include` | `true` | With `claude` in `targets`, keep the same block in `.worktreeinclude`, so Claude Code copies the ignored outputs and your local files into each worktree. |
| `ignore-worktree-include` | `false` | Keep managing `.worktreeinclude` and list it in the managed ignore block. Requires `worktree-include: true` and `claude` in `targets`. |

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

So with `claude` in `targets`, sync also keeps the block in `.worktreeinclude`, minus `.sync-state`, `.command-lock`, Claude worktree directories, and the task lock. Your local files and packs come along, so the worktree renders what the main checkout does.

- Set `gitignore.ignore-worktree-include: true` to keep the managed file out of Git. If it is already tracked, run `agnostic-ai sync --untrack` to remove it from the index and keep the file.
- Set `gitignore.worktree-include: false` to manage the file yourself. Sync then removes only its own block.

A fresh clone or `git worktree` lacks these paths until `sync` runs. See [checkout and merge hooks](@/docs/git-hooks.md#regenerate-on-checkout).

### Block contents

- Entries are root-anchored (`/AGENTS.md`), so nested same-named files are not ignored.
- Files collapse to their generated subdirectory (`/.claude/rules/`), never higher, so siblings such as `.claude/settings.json` stay visible. A per-kind dir such as `outputs.<target>.rules-dir` collapses at the dir itself.
- Output under a rule or review `scope` stays one line per file (`/services/api/AGENTS.md`).
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.command-lock`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai/local/`. `init` seeds them even with `gitignore.enabled: false`.
- A leftover file from a deleted spec stays ignored until you remove it.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings.

## Watched inputs

`sync --watch` runs again when the config files, any `sources` directory, `.agnostic-ai/local/`, or `.agnostic-ai/overlays/` change. Overlays hold keys that specs do not cover, such as Claude `statusLine` or Codex `[history]`. See [`sync --watch`](@/docs/cli-reference/sync.md#sync).

## Path semantics

- Relative `sources` and `outputs` paths start at the directory holding `agnostic-ai.yaml`. Absolute `sources` paths are used as written.
- Sync creates output directories as needed and overwrites existing files.

## Entry-point files

`sync` copies `.agnostic-ai/AGNOSTIC_AI.md` into one root entry-point file per enabled tool. See the [per-target table](@/docs/target-behavior.md#entry-point-files). `.agnostic-ai/local/AGNOSTIC_AI.md` [extends that body](@/docs/local-overrides.md#extend-the-instructions) on one machine.

Write the instructions every tool shares in `AGNOSTIC_AI.md`. Every session loads this text, so keep it to what an agent cannot infer from the code. Rules, agents, and skills stay in their own `sources` folders. Sync never overwrites this file, only the root entry points.

When the file is missing, sync seeds it with one line saying the tool files are generated from `.agnostic-ai/`. A project created by an earlier release may hold the old long default text. `doctor` points it out. Replace it with your own instructions.

{% <details summary="Legacy merged rules file"> %}
Setting `outputs.<target>.rules-file: <path>` restores the legacy layout: the adapter writes one merged document at `<path>`, including the built-in skill instructions. Two adapters writing different content to one path fail unless you set `sync.collision-policy: prefer-spec`.

When `<path>` is the target's own entry-point file (`outputs.claude.rules-file: CLAUDE.md`), sync skips the pointer body. Any other path keeps the entry-point file, and for `claude` the pointer body also gains an `@<path>` import.
{% </details> %}

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
- `import <tool>` keeps a fenced source when the imported file equals what sync renders. Otherwise it overwrites the source and warns.

## Which value wins {#precedence}

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags (such as `agnostic-ai sync -t claude`)

## Layered specs

Specs load from four layers, lowest first. A higher layer overrides a spec of the same kind and name. New names append. The `project-user` layer merges into the shared spec field by field (see [local overrides](@/docs/local-overrides.md#override-fields)). `agnostic-ai list` shows each spec's layer.

| Layer | Root | Loaded when |
|-------|------|-------------|
| `builtin` | bundled specs | enabled with `builtins` |
| packs | `.agnostic-ai/packs/` from `agnostic.packs.lock` | packs are installed |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai/local` | directory exists |

Only `project` honors custom `sources` paths. `$AGNOSTIC_AI_HOME` is not a project layer.

## Global configuration

`agnostic-ai sync --global` installs user-level instructions, rules, hooks, and skills for 22 of the 25 targets. [Global output](@/docs/target-behavior.md#global-output) lists the paths. It works from any directory and loads no packs or project specs.

Source root: `$AGNOSTIC_AI_HOME`, or `~/.agnostic-ai/` when unset.

```text
~/.agnostic-ai/
├── agnostic-ai.yaml        # optional: targets, builtins, requires, lint, on-unsupported, models
├── AGNOSTIC_AI.md
├── agents/*.md
├── rules/*.md
├── hooks/*.yaml
├── settings/*.yaml         # default model and effort
├── mcps/*.yaml             # user-level MCP servers
├── skills/<name>/SKILL.md
└── local/                  # optional personal layer, same layout
```

Specs in `local/` merge into shared specs of the same kind and name, field by field, and new names append. A `::parent` line extends the shared body. See [local overrides](@/docs/local-overrides.md#override-fields). `local/AGNOSTIC_AI.md` comes last in the managed instructions block. Add `/local/` to the source root's `.gitignore` before adding personal files.

To sync a fixed set of tools without `--only` on every run, list them in the source root's `agnostic-ai.yaml`:

```yaml
targets: [claude, codex, cursor]
```

- `sync --global`, `lint --global`, and `validate --global` then use only those targets. A `targets` list in `local/agnostic-ai.yaml` replaces the shared one.
- `--only` and `--except` narrow the list for one run and must name configured targets. `--target` replaces it and skips the home config's `targets`.
- A target with no user-level files, such as `aider` or `continue`, is skipped with one warning. An unknown name stops the run.
- [`builtins`](#built-ins), [`requires`](#requires), `lint`, and [`models`](#models) work as in a project. For each key, `local/agnostic-ai.yaml` replaces the shared value. A `models` tier replaces the same-name tier whole. When a file's tiers do not load, `sync --global` stops. With `-t`, it warns, skips that file's tiers, and omits the `model` of any spec that names one.
- [`on-unsupported`](#on-unsupported) sets what `sync --global` does with a skill line that Claude Code expands and another target reads as plain text: `warn` prints a note, `error` fails the sync, `silent` hides it. See [Claude Code body syntax](@/docs/spec-format/skills.md#claude-code-body-syntax).
- Other keys except `version` print a warning and are ignored.
- A target dropped from the list keeps its synced files and ownership records until you remove them by hand.

`list --global` shows the specs in effect with their `builtin`, `global`, or `global-local` layer. `validate --global` and `lint --global` check before a sync writes. `migrate --global` rewrites old spec forms. Global layers never merge with project specs.

- Accepted `sync` flags are in the [CLI reference](@/docs/cli-reference/sync.md#sync).
- Nested rules, rules with scope, path, glob, or target conditions, commands, settings `permissions` rule lists (only `permissions.default-mode` is written), inheritance, and merging with project specs are unsupported.
- Skills copy bundled assets verbatim. Shared directories such as `~/.agents/skills/` keep neutral frontmatter. See [Codex skills](@/docs/targets/codex.md) for `disable-model-invocation`.
- Hooks and skills honor `target`, `targets`, and `targets-exclude`. Set hook events per target. Sync does not translate event names.
- Kiro takes each hook as its own file, `~/.kiro/hooks/<name>.json`, in the project hook format. Shared scripts go to `~/.kiro/scripts/`. `KIRO_HOME` moves both.
- Targets without global agent output warn and skip agents.
- Output is real files, never symlinks. A user file that is itself a symlink, such as a dotfiles-managed `CLAUDE.md`, is written through. A symlink inside a skills, agents, or rules directory stops the run.
- Sync records what it wrote in `$AGNOSTIC_AI_HOME/state/global.json` and removes only those artifacts for the targets in the run, so `--only` never sweeps another target.
- A hand edit to a file sync owns, or to an instructions file's managed block, stops the run and names the file. Move the edit into the source, or rerun with `--backup` to overwrite it and keep `<path>.bak`. Text outside the managed block never counts.
- A file or hook that already matches what sync would write is adopted. One that differs, even by one byte, stops the run. So do an unmanaged agent, skill, or rule collision, a damaged marker, invalid native JSON, corrupt state, and state recorded under another `HOME`.
- Without `--only`, explicit targets, or a home `targets` list, a target with a relative root variable or an invalid agent name is skipped with a warning. Naming it makes that an error.
- Each tool decides which settings win when global and project configuration both exist. See [shared names](#global-shared-names). Sync Goose and OpenHands together to update their shared agent file.

Ordinary `agnostic-ai sync` reads only the names of `~/.agnostic-ai/` specs, to [warn about shared names](#global-shared-names).

Inside the global source root, `sync` stops before any write and points you to `sync --global`. `init`, `import`, `new`, `packs`, `cleanup`, `revert`, and `install-hook` stop the same way. Read-only commands such as `lint`, `validate`, and `doctor` still run. Put project-only defaults in a project's `.agnostic-ai/` or a pack.

For a home kept in Git, `install-hook --global` writes a pre-commit hook that runs `lint --global --strict`, `validate --global`, and `sync --global --check`. To start a home from what your tools already hold, run `agnostic-ai import --global`. See [import](@/docs/cli-reference/start.md#import).

### Shared names {#global-shared-names}

A project skill or agent can share its `name` with one in the home. Sync writes both, and each tool decides which one it loads. `sync` and `doctor` print one warning per shared name, naming each target where one copy hides the other, which copy wins, and the fix:

```text
! .agnostic-ai/skills/gh-issue/SKILL.md: skill "gh-issue" also exists in ~/.agnostic-ai/skills/gh-issue/SKILL.md with different content; claude loads the global one, which exists only on this machine, gemini loads this one; to load both, rename the global one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --global`
```

A winning global copy changes what you run, not what others run from the repository. When only project copies win, the warning says the global one is unused here. When both copies hold the same content, it says to delete one before they drift apart.

| Target | Skill with the same name | Agent with the same name |
|--------|--------------------------|--------------------------|
| Amp | Global wins when both sync `amp`: `~/.agents/skills/` masks `.agents/skills/` ([skills](https://ampcode.com/docs/customize/skills)) | No global agents |
| Claude Code | Global wins ([skills](https://code.claude.com/docs/en/skills)) | Project wins ([subagents](https://code.claude.com/docs/en/sub-agents)) |
| Codex | Both can appear in skill selectors, so no warning ([skills](https://learn.chatgpt.com/docs/build-skills)) | Not checked |
| Gemini CLI | Project wins ([skills](https://geminicli.com/docs/cli/skills/)) | Not checked |

Other tools do not document which settings win, so sync does not warn for them.

{% <details summary="Which pairs the check compares"> %}
- Targets that both the project and the home's [`targets`](#global-configuration) write. A spec whose `target`, `targets`, or `targets-exclude` leaves one of them out is skipped.
- Claude Code skill names match ignoring case, spacing, invisible characters, and fullwidth forms. Other targets compare names exactly.
- A spec shared through a link is not a clash. Neither is a project that is the home.
{% </details> %}

To layer on purpose, give the project spec its own name, such as a `gh-issue-project` skill beside a global `gh-issue`. Both then load everywhere.

To keep one name in both places, list it in [`sync.allow-global-names`](#syncallow-global-names). To stop the warning for every shared name, set [`sync.global-name-clash: ignore`](#syncglobal-name-clash).

### Default model and effort {#global-default-model-and-effort}

Settings specs in the home set each tool's default model and effort in its user settings file. `model` and `effort` each take one string for every target, or a per-target map with an optional `default`:

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

Other targets and global permission lists raise a coverage note, and so does `effort` on Gemini. Augment has no default model or effort key but takes `x-augment` keys, such as `shell`.

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

`sync --global --only claude` writes `permissions.defaultMode` in `~/.claude/settings.json`, or under `CLAUDE_CONFIG_DIR`. It accepts `default`, `manual`, `acceptEdits`, `plan`, `auto`, `dontAsk`, and `bypassPermissions`. A later settings spec wins. Sync owns only this key, so hand-written `allow`, `deny`, and `ask` rules stay. Removing the field removes the managed mode, unless you changed it by hand.

Other targets report a coverage note. For Codex, set `x-codex.approval_policy` and `x-codex.sandbox_mode` instead. Claude's `auto` and `bypassPermissions` need user, managed, or session settings. See [Claude's mode reference](https://code.claude.com/docs/en/settings-reference#permissions-defaultmode).

Each item wins over the one before it:

1. `settings/*.yaml` in the home.
2. `local/settings/*.yaml`. A same-named file merges into the shared one field by field. A new file comes after the shared ones, so its `model` and `effort` win.
3. The project tier: a project's settings file, such as `.codex/config.toml` or `.claude/settings.json`, wins according to the tool's own settings order.
4. An agent's own `model` and `effort`, for that agent.
5. The tool's flag for one run, such as `codex -m` or `claude --model`.

Sync edits only the keys it writes. Removing the spec removes only those keys, and a file sync created is deleted once it is empty.

- A key that already holds the value sync would write is adopted.
- A key with another value stops the run and names the file, key, and both values. This is normal for Codex, whose `/model` picker saves to `config.toml`. `--backup` overwrites the key and keeps `<path>.bak`.
- `--dry-run` lists each key a write sets or removes, and `--check` fails on a changed key.
- An `x-<target>` block sets that target's own keys in the same file. Nested objects merge leaf by leaf. An `x-claude` key wins over the portable field it shares a key with. A later spec wins key by key, and `null` drops an earlier key.
- Global sync writes Codex top-level scalars and arrays, such as `x-codex.notify`, to `~/.codex/config.toml`. Tables such as `profiles`, `x-claude.hooks`, and `x-claude.permissions` raise a coverage note. Project sync does not route `x-codex` into `.codex/config.toml`.
- `agnostic-ai explain --global settings/defaults.yaml` names the file and key each target gets.

### MCP servers {#global-mcp-servers}

MCP specs in the home's `mcps/` install each server in the user MCP file of every target that has one, rendered as the target's project MCP file renders it:

| Target | File | Where |
|---|---|---|
| antigravity | `~/.gemini/config/mcp_config.json` | `mcpServers.<name>`: `{command, args, env, cwd}` or `{serverUrl, headers}` |
| claude | `~/.claude.json` (`$CLAUDE_CONFIG_DIR/.claude.json` when set) | top-level `mcpServers.<name>` |
| codex | `~/.codex/config.toml` | `[mcp_servers.<name>]` table |
| cursor | `~/.cursor/mcp.json` | `mcpServers.<name>` |
| copilot | `~/.copilot/mcp-config.json` | `mcpServers.<name>`, with `tools: ["*"]` when the spec sets none |
| gemini | `~/.gemini/settings.json` | `mcpServers.<name>` |
| qoder | `~/.qoder/settings.json` | `mcpServers.<name>` |
| augment | `~/.augment/settings.json` | `mcpServers.<name>` |
| openhands | `~/.openhands/mcp.json` (`$OPENHANDS_PERSISTENCE_DIR/mcp.json` when set) | `mcpServers.<name>`: `{command, args, env}` or `{url, transport, headers, auth}` |
| warp | `~/.warp/.mcp.json` | `mcpServers.<name>`: `{command, args, env, working_directory}` or `{url, headers}` |

Each server follows the per-key rules above:

- A hand-written server that means the same as the spec is adopted. A different one with the same name stops the run, and `--backup` overwrites it.
- A server sync wrote is removed when its spec goes. Servers you add under other names stay.
- In `~/.claude.json`, sync edits only its own `mcpServers` entries and creates a missing file at `0600`. If the file changed after sync read it, sync stops without writing. Run it again.
- A spec with `disabled: true` stays out of the Augment, Claude, Cursor, Copilot, OpenHands, and Warp user files, where a listed server is live in every project.
- When `CLAUDE_CONFIG_DIR` or another root variable moves a file, the next sync removes its entries from the old one.

A personal agent such as `~/.agnostic-ai/agents/reviewer.md` with `targets: [claude, codex]` installs with:

```console
agnostic-ai sync --global --only claude,codex
agnostic-ai sync --global --only claude,codex --check
```

Sync writes `~/.claude/agents/reviewer.md` and `~/.codex/agents/reviewer.toml`. If a hand-copied file sits at an output path, save its edits in the source and move it aside first. `--backup` does not bypass unmanaged collisions.
