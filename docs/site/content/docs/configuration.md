+++
title = "Configuration"
description = "Choose tools, source folders, generated file paths, and settings for your project or machine."
weight = 130

[extra]
group = "Reference"

[extra.moved]
sync = "@/docs/sync-settings.md#sync"
synccollision-policy = "@/docs/sync-settings.md#synccollision-policy"
synctarget-overview = "@/docs/sync-settings.md#synctarget-overview"
syncresolve-imports = "@/docs/sync-settings.md#syncresolve-imports"
syncdropped-summary = "@/docs/sync-settings.md#syncdropped-summary"
syncshared-skills = "@/docs/sync-settings.md#syncshared-skills"
syncunmanaged = "@/docs/sync-settings.md#syncunmanaged"
syncoutput-manifest = "@/docs/sync-settings.md#syncoutput-manifest"
syncallow-global-names = "@/docs/sync-settings.md#syncallow-global-names"
syncglobal-name-clash = "@/docs/sync-settings.md#syncglobal-name-clash"
gitignore = "@/docs/sync-settings.md#gitignore"
claude-code-worktrees = "@/docs/sync-settings.md#claude-code-worktrees"
block-contents = "@/docs/sync-settings.md#block-contents"
+++

# Configuration

## sync {#sync .legacy-anchor}

## sync.collision-policy {#synccollision-policy .legacy-anchor}

## sync.target-overview {#synctarget-overview .legacy-anchor}

## sync.resolve-imports {#syncresolve-imports .legacy-anchor}

## sync.dropped-summary {#syncdropped-summary .legacy-anchor}

## sync.shared-skills {#syncshared-skills .legacy-anchor}

## sync.unmanaged {#syncunmanaged .legacy-anchor}

## sync.output-manifest {#syncoutput-manifest .legacy-anchor}

## sync.allow-global-names {#syncallow-global-names .legacy-anchor}

## sync.global-name-clash {#syncglobal-name-clash .legacy-anchor}

## gitignore {#gitignore .legacy-anchor}

## Claude Code worktrees {#claude-code-worktrees .legacy-anchor}

## Block contents {#block-contents .legacy-anchor}

The [`sync` and `gitignore` keys](@/docs/sync-settings.md) have their own page.

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

`agnostic-ai.local.yaml` holds changes for one machine. `init` adds it to `.gitignore`. Personal specs go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

Local values replace or merge with shared values as follows:

- A single value or list replaces the shared value. A group of settings merges key by key.
- An empty key, including one with only commented-out children, leaves a shared group unchanged and triggers a lint warning. Write `{}` to clear the group. Model tiers reject `{}`.
- Over a list or single value, an empty key replaces the shared value. An empty `targets:` also replaces default targets when no shared list exists. An empty `requires:` turns the version check off.

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
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.92.0/docs/schemas/config.schema.json
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
| [`gitignore`](@/docs/sync-settings.md#gitignore) | map | `enabled: false` | Managed `.gitignore` block. |
| [`sync`](@/docs/sync-settings.md#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | A project command that `verify` runs. |
| [`import`](#import) | map | per source | Import behavior. |
| [`lint`](#lint) | map | see section | Budgets for always-loaded text. |
| [`doctor`](#doctor) | map | see section | Opt-in diagnostic checks. |
| [`coverage`](#coverage) | map | none | Coverage notes to accept, and whether notes fail sync. |
| [`memory`](#memory) | map | `personal: checkout` | Where personal memory lives. Set it in `agnostic-ai.local.yaml` only. |

## Built-ins

`builtins` turns on specs bundled with the binary. None are on unless listed; `init` writes `builtins: [handoff]` for a new project, or `[handoff, memory]` with `--memory`.

```yaml
requires: ">=0.92.0"
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

The same list works in the [global home config](#global-configuration). A `builtins` list in a local config replaces the shared list, and `builtins: []` there turns them all off. When you remove a name, the next sync removes the files and tool settings it manages, and keeps unrelated settings and project overrides. A local `builtins` list never changes the committed ignore files; see [block contents](@/docs/sync-settings.md#block-contents).

When you enable built-ins, set `requires` to the release you use. An upgrade that changes built-in text makes `sync --check` fail until you sync.

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
requires: ">=0.92.0"          # minimum: 0.92.0 or any newer release
requires: "0.92.0"            # exact: only 0.92.0; "=0.92.0" means the same
requires: ">=0.92.0 <0.93.0"  # range: 0.92.0 and its patch releases
```

- **Minimum**: the default that `init` writes, naming the release that created the project. Newer releases keep working.
- **Exact**: choose it when you commit generated files and run `sync --check` in CI, since a newer release can write different output. `init --gitignore=off` writes a minimum and prints this advice. When npm installs the tool, pin the same release in `package.json`.
- **Range**: accept patch releases but not the next minor one.

[`upgrade --requires`](@/docs/cli-reference/upgrades.md#upgrade-options) raises the value to the installed release, keeping a minimum as a minimum and an exact pin as an exact pin. A range becomes a minimum, and the command prints the value it replaced.

An installed release outside the allowed range stops every command that reads your specs before any write, with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires). Follow that page for fixes. `sync --watch` stops when a pulled config puts the binary outside `requires`.

A value is one or more terms separated by spaces, and the binary must match every term. A term is `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`; anything else fails as AAI-004, naming the file. That includes `^`, `~`, commas, and `||`. Before 1.0, `^0.92.0` would allow only 0.92 patch releases, so write `>=0.92.0` to accept newer releases. A build from source (`go run`, or a commit after a tag) is not a release, so it warns once instead.

`agnostic-ai.local.yaml` can replace the value. An empty `requires:` there turns the check off. The [global home config](#global-configuration) accepts the key too.

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

An agent with `model: fast` gets `haiku` (Haiku 5.5 today) in Claude Code and `gpt-6-luna` in Codex, both at `low` effort. To pin a Claude version, write the exact id instead, such as `claude: claude-haiku-5-5`. Other targets keep their default model. Skills, commands, and settings specs name tiers the same way.

Claude Code resolves `opus`, `sonnet`, `haiku`, and `fable` itself. Codex takes only exact ids, so agnostic-ai resolves these aliases when it writes Codex files:

| Codex alias | Model id |
|-------------|----------|
| `sol` | `gpt-6.1-sol` |
| `luna` | `gpt-6-luna` |
| `astra` | `gpt-6-astra` |
| `terra` | `gpt-6.1-terra` |

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
- A tier in `agnostic-ai.local.yaml` replaces the entire shared tier of the same name. The [global home config](#global-configuration) accepts the key too.

## `targets`

Default: every tool except `amp`, `warp`, `jules`, `goose`, `augment`, `ellipsis`, `grok`, `codebuddy`, and `kimi`. `-t/--target` replaces the list for one run.

A likely typo of a built-in target, such as `cursr`, fails before any write. Any other unknown name is skipped with a warning, since it may be an external adapter missing from this machine's PATH.

An empty list keeps generated files: `sync` warns, `sync --check` fails while they remain, and [`--allow-empty-targets`](@/docs/cli-reference/sync.md) removes them.

`init` and the first `sync` can write this list through a [picker](@/docs/cli-reference/sync.md#first-sync-target-picker).

The web interface lists every tool with a switch under **Settings**, **Coding tools**; see [serve](@/docs/cli-reference/serve.md#serve).

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

Set limits on text loaded in every session. `agnostic-ai lint` warns when a limit is exceeded; `lint --strict` exits 1.

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

Link destinations [`doctor --check-references`](@/docs/cli-reference/doctor.md#doctor) never reports as broken.

```yaml
doctor:
  check-references:
    ignore:
      - url                # exact destination, as written in the source Markdown
      - gcp-url
      - "*-placeholder"     # glob; `*` stays inside one path segment
```

An entry is the destination text as written, such as `url` for `[Logs](url)`, or a `path.Match` glob, as in [`sync.unmanaged`](@/docs/sync-settings.md#syncunmanaged).

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
| `crush` | unknown hook events | Use one of the [13 native lifecycle events](@/docs/targets/crush.md#hooks). Matchers require a tool event. |

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

## Watched inputs

`sync --watch` runs again when the config, sources, local specs, or overlays change. See [watch mode](@/docs/cli-reference/sync.md#watch-mode) for the full list. Overlays in `.agnostic-ai/overlays/` hold keys that specs do not cover, such as Claude `statusLine` or Codex `[history]`.

## Path semantics

- Relative `sources` and `outputs` paths start at the directory holding `agnostic-ai.yaml`. Absolute `sources` paths are used as written.
- `outputs` paths and `gitignore.path` must stay inside the project and out of `.git/`, so a cloned repository cannot make sync write elsewhere. The config fails to load when one climbs above the project root or points into `.git/`, and sync stops before writing to an absolute `outputs` path outside the project. `gitignore.path` must be relative; `.git/info/exclude` is its one `.git/` exception.
- A `sync.unmanaged` entry that climbs above the project root fails to load. A leading `/` anchors an entry at the project root.
- Sync skips a `.agnostic-ai/.sync-state` entry outside the project or inside `.git/` and names it in a warning. It never removes or changes that file.
- A spec or MCP server named `.git` fails to load.
- On Windows, sync refuses to write a file whose name holds a colon, because Windows stores it as a hidden stream of the file named before the colon. Qwen Code and Gemini CLI turn each colon in a command name into a folder, so a command named `git:commit` syncs as `git/commit.md` and `git/commit.toml`, and `import qwen` and `import gemini` read a file in a subfolder back as `git:commit`. A target that writes the name as one file refuses the write, such as Claude Code for commands or Continue for assistants; `sync --check` and `--dry-run` report the same refusal. Name such a spec `git-commit` for those targets. The next sync removes the colon file an older release wrote. Windows can leave an empty file named `git` beside it: Gemini CLI sync removes it to make the `git` folder, and for other targets you can delete it. Gemini CLI sync stops and names a `git` file that holds content, that no colon output in the last sync explains, or whose old colon file you edited; copy any text you need, delete `git`, and sync again.
- On Windows, import refuses an MCP server or other spec whose file name would hold a colon, such as a server named `ns:tool`, and names the file. `import --global` skips such a server with a warning that names the home config file. Rename the server before you import it; Continue needs the rename too, since sync writes `.continue/mcpServers/<name>.yaml`. An older release reported such an import as done and may have left an empty file such as `.agnostic-ai/mcps/ns`, which you can delete.
- Project sync refuses to write or remove through a link that leads outside the project or into a `.git` folder, and names the link. A link that stays inside the project, such as `CLAUDE.md` linked to `AGENTS.md`, still works. `sync --global` follows linked dotfiles as before. A linked `.agnostic-ai` folder keeps the sync lock and ledger only when it holds `AGNOSTIC_AI.md`.
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
- For how `import` treats a fenced source, see [import](@/docs/cli-reference/import.md#files-and-source-order).

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
