# Configuration

`agnostic-ai.yaml` lives at the project root. It is read from the current working directory at command time. Every section is optional. Defaults are listed below.

Legacy filename: `agnostic.config.yaml` still loads, with a deprecation warning. Rename to `agnostic-ai.yaml` when convenient.

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

## Full schema

```yaml
version: 1

# Source directories, relative to this file. The whole `sources:` block is
# optional: any omitted kind defaults to `.agnostic-ai/<kind>`, so the values
# below are the defaults. Set a field only to point that kind somewhere else.
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
  hooks: .agnostic-ai/hooks
  mcps: .agnostic-ai/mcps
  commands: .agnostic-ai/commands
  settings: .agnostic-ai/settings
  reviews: .agnostic-ai/reviews
  environments: .agnostic-ai/environments
  ignore: .agnostic-ai/ignore

# AI CLIs to emit configs for. These 12 are the default set (used when
# `targets` is omitted). Two more adapters exist, `amp` and `warp`. They
# share the root `AGENTS.md` pointer body with `codex` (written once via
# auto-dedup, since the body is byte-identical), so they add no new
# entry-point and are left out of the default set. Enable them explicitly
# if you use those tools. A real collision only happens when two targets
# set a conflicting `outputs.<target>.rules-file: AGENTS.md`.
targets:
  - claude
  - codex
  - gemini
  - cursor
  - copilot
  - aider
  - cline
  - windsurf
  - continue
  - zed
  - opencode
  - antigravity

# Per-target output overrides. Each target accepts only the fields
# relevant to it. Defaults shown in comments. The root entry-point file
# is written centrally by `sync` and is not adapter-configurable (see
# the Entry-point files section below).
#
# Set `outputs.<target>.rules-file: <path>` on any target to opt back
# into the legacy concatenated rules layout at that path; `sync` then
# skips the pointer-body write for that target.
outputs:
  claude:
    dir: .claude                 # default
    rules-dir: .claude/rules     # default. One .md per rule.
    commands-dir: .claude/commands # default. One .md per command (slash prompt).
    mcp-file: .mcp.json          # default
    # rules-mode: import         # legacy opt-in: also wire .claude/rules/*.md into CLAUDE.md via
    #                            # @-imports, for Claude Code versions without native rules loading.
    # rules-file: CLAUDE.md      # opt-in: legacy concatenated rules layout.
  codex:
    agents-dir: .codex/agents    # default. One TOML per agent. Set to .agents/agents for the shared community layout.
    skills-dir: .agents/skills   # default. One folder per skill, the path Codex CLI scans (shared with Amp).
    # shared-subagents: <bool>  # default: true (emit skills at skills-dir).
    #                            # Set false to skip codex skill emission entirely.
    # commands-dir: .codex/prompts # opt-in: legacy project prompts layout. Codex reads prompts
    #                            # from ~/.codex/prompts only and deprecates them for skills.
    mcp-file: .codex/config.toml # default. Holds both [[hooks.<event>]] and [mcp_servers.<name>].
    # exec-policies:              # opt-in. Renders .codex/rules/default.rules.
    #   - pattern: ["composer", "test"]
    #     decision: allow          # allow | forbidden | ask
    #     justification: Run tests
    #     match: ["composer test"]
    # exec-policies-file: ./agnostic-ai/codex.exec-policies.yaml  # alt: external YAML list
  gemini:
    commands-dir: .gemini/commands   # default. One .toml per agent and per command (and per skill when opted in).
    emit-skills-as-commands: false   # default
    mcp-file: .gemini/settings.json  # default. Holds both mcpServers and hooks.
    ignore-file: .aiexclude          # default. Agent ignore patterns (gitignore syntax).
  cursor:
    rules-dir: .cursor/rules     # default
    agents-dir: .cursor/agents   # default. One native subagent .md per agent.
    skills-dir: .cursor/skills   # default. One folder per skill (<name>/SKILL.md + bundled assets).
    commands-dir: .cursor/commands  # default. One .md per command.
    mcp-file: .cursor/mcp.json   # default
    review-file: BUGBOT.md       # default basename. Lands at .cursor/BUGBOT.md (root and per scope).
    environment-file: .cursor/environment.json  # default. Background-agent bootstrap config.
    ignore-file: .cursorignore   # default. Agent ignore patterns (gitignore syntax).
    hooks-file: .cursor/hooks.json  # default. version + per-event arrays ({command, matcher?}).
  copilot:
    instructions-dir: .github/instructions  # default. One .instructions.md per scoped rule, agent, skill.
    mcp-file: .vscode/mcp.json              # default
    # chatmodes-dir: .github/chatmodes      # opt-in: also emit each agent as a Copilot Custom Chat Mode.
  aider:
    ignore-file: .aiderignore    # default. Agent ignore patterns (gitignore syntax).
    # Opt-in: also merge .aider.conf.yml so Aider auto-loads CONVENTIONS.md.
    # conf-file: .aider.conf.yml
    # model: gpt-4o
    # weak-model: gpt-4o-mini
  cline:
    rules-dir: .clinerules       # default. One .md per rule, agent, and skill.
    # workflows-dir: .clinerules/workflows  # opt-in: also emit each agent as a Cline Workflow (/<name>.md).
  windsurf:
    rules-dir: .windsurf/rules   # default. One .md per rule, agent, and skill.
    # workflows-dir: .windsurf/workflows    # opt-in: also emit each agent as a Windsurf Workflow (/<name>).
  continue:
    rules-dir: .continue/rules        # default
    mcp-dir: .continue/mcpServers     # default. One YAML per MCP server.
  amp:
    commands-dir: .agents/commands  # default. One .md per agent and per command.
    skills-dir: .agents/skills      # default. One folder per skill (<name>/SKILL.md).
    mcp-file: .amp/settings.json    # default. amp.mcpServers (dotted key).
  zed:
    file: .rules                  # default
    mcp-file: .zed/settings.json  # default. context_servers (stdio command/args/env, HTTP/SSE native url/headers).
    # tasks-file: .zed/tasks.json # opt-in: emit each hook as an on-demand Zed Task.
  warp:
    # workflows-dir: .warp/workflows  # opt-in: emit Warp Workflow YAMLs per agent.
    mcp-file: .warp/.mcp.json     # default. Standard mcpServers schema.
  opencode:
    commands-dir: .opencode/commands     # default. One .md per agent and per command (and per skill when opted in).
    emit-skills-as-commands: false       # default
    mcp-file: opencode.json              # default. mcp map with type: local|remote.
  antigravity:
    rules-dir: .agent/rules             # default. One .md per rule and per agent.
    skills-dir: .agent/skills           # default. One folder per skill (<name>/SKILL.md).
    # rules-file: .agent/AGENTS.md      # opt-in: legacy merged doc; skips the pointer-body write.

# What to do when a spec kind is unsupported by a target
# (e.g. hooks for Cursor, or mcps for Cline).
on-unsupported: warn   # warn | error | silent

# Auto-manage a block in .gitignore listing every generated path.
# When enabled, `sync` rewrites the block; `--gitignore on|off` overrides
# this setting for one run.
gitignore:
  enabled: false
  path: .gitignore   # default
  allow:             # keep these tracked even when a broader ignore matches
    - "**/testdata/AGENTS.md"
```

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Reserved for future migrations. |
| `sources` | map | see below | Per-kind source directories (relative to config file). |
| `targets` | list | 12 default adapters | Adapter names to emit. Defaults to every adapter except `amp` and `warp`. Unknown targets log a warning and skip. |
| `outputs` | map | see below | Per-target output path overrides. |
| `on-unsupported` | string | `warn` | How to react when a kind is unsupported by a target. One of `warn`, `error`, `silent`. |
| `gitignore` | map | `enabled: false` | Auto-manage a block in `.gitignore` listing generated paths. See [`gitignore`](#gitignore). |
| `sync` | map | see below | Sync-level knobs. See [`sync`](#sync). |

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

Per-target paths. Each target reads only the fields it understands. Irrelevant fields are ignored.

| Target | Field | Default | Notes |
|--------|-------|---------|-------|
| `claude` | `dir` | `.claude` | Holds `agents/`, `skills/`, `settings.json`. |
| `claude` | `rules-dir` | `.claude/rules` | One `.md` per rule. |
| `claude` | `rules-mode` | _empty_ | Set to `import` to append a sentinel-marked block of `@.claude/rules/<name>.md` imports to the `CLAUDE.md` pointer body. Only needed on Claude Code versions without native `.claude/rules/` loading; current versions auto-load the directory. Keeps the pointer body intact. Ignored when `rules-file` is set. |
| `claude` | `rules-file` | _empty_ | When set, switches to the legacy concatenated single-file layout at that path (typically `CLAUDE.md`). `sync` skips the pointer-body write for `claude`. |
| `claude` | `mcp-file` | `.mcp.json` | Standard `mcpServers` schema. |
| `claude` | `settings` | _empty_ | First-class block mirroring `.claude/settings.json` keys. See [Claude settings](#claude-settings). |
| `codex` | `agents-dir` | `.codex/agents` | One TOML per agent (Codex subagent schema). Override to `.agents/agents` for the community shared layout. |
| `codex` | `skills-dir` | `.agents/skills` | One folder per skill (Codex skills layout, the path Codex CLI scans; shared with Amp, identical bytes dedupe). |
| `codex` | `commands-dir` | _empty_ | When set (e.g. `.codex/prompts`), emits one `.md` per command at that path. Codex reads custom prompts from `~/.codex/prompts` only and deprecates them for skills, so this stays opt-in. |
| `codex` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `codex`. |
| `codex` | `mcp-file` | `.codex/config.toml` | Holds `[mcp_servers.<name>]` tables. Hooks moved out into `hooks-file`. |
| `codex` | `hooks-file` | `.codex/hooks.json` | Per-event hook arrays in Claude `settings.json`-shaped JSON. Preferred over the legacy TOML `[[hooks.<event>]]` schema: preserves per-hook `timeout` + `statusMessage` and dedupes overlapping matchers. |
| `codex` | `config` | _empty_ | First-class block for `.codex/config.toml` global keys. See [Codex config](#codex-config). |
| _any_ | `provenance-header` | `true` | When `false`, suppresses the `Generated by agnostic-ai` comment in this target's files. Useful for byte-stable round-trip from hand-authored sources. Loses the legacy-file detection that comment enables, so opt out per-target rather than project-wide. |
| `gemini` | `commands-dir` | `.gemini/commands` | One TOML per agent (one per skill when `emit-skills-as-commands: true`). |
| `gemini` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.gemini/commands/skill-<name>.toml`. |
| `gemini` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `gemini`. |
| `gemini` | `mcp-file` | `.gemini/settings.json` | Holds both `mcpServers` and `hooks`. HTTP MCP entries use `httpUrl`. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule. |
| `cursor` | `agents-dir` | `.cursor/agents` | One native Cursor subagent `.md` per agent (`name`, `description`, optional `model`/`readonly`/`is_background`). |
| `cursor` | `skills-dir` | `.cursor/skills` | One folder per skill (`<name>/SKILL.md` + bundled assets, the Agent Skills layout Cursor discovers). |
| `cursor` | `commands-dir` | `.cursor/commands` | One `.md` per command spec (Cursor's standard commands location). |
| `cursor` | `mcp-file` | `.cursor/mcp.json` | Standard `mcpServers` schema. |
| `copilot` | `instructions-dir` | `.github/instructions` | One `.instructions.md` per scoped rule, agent, skill. `applyTo:` frontmatter derived from `globs` or scope. |
| `copilot` | `chatmodes-dir` | _empty_ | When set, each agent also emits as a Copilot Custom Chat Mode at `<dir>/<name>.chatmode.md`. The `agent-<name>.instructions.md` emission still happens. Opt-in. |
| `copilot` | `rules-file` | _empty_ | When set, writes always-on rules concatenated at that path (legacy layout). `sync` skips the pointer-body write for `copilot`. |
| `copilot` | `mcp-file` | `.vscode/mcp.json` | VS Code schema: top-level `servers` with `type` field per entry. |
| `aider` | `rules-file` | _empty_ | When set, writes a legacy merged document at that path (typically `CONVENTIONS.md`). `sync` skips the pointer-body write for `aider`. |
| `aider` | `conf-file` | _empty_ | When set, merges `.aider.conf.yml` so Aider auto-loads `CONVENTIONS.md`. Pre-existing keys preserved; `read:` list de-duplicates. Opt-in. |
| `aider` | `model` | _empty_ | Optional `model:` value written into the conf file. |
| `aider` | `weak-model` | _empty_ | Optional `weak-model:` value written into the conf file. |
| `cline` | `rules-dir` | `.clinerules` | One `.md` per rule, agent, and skill (`skill-<name>.md`). |
| `cline` | `workflows-dir` | _empty_ | When set, each agent also emits as a Cline Workflow at `<dir>/<name>.md` (invokable as `/<name>.md`). The rule-form emission still happens. Opt-in. |
| `windsurf` | `rules-dir` | `.windsurf/rules` | One `.md` per rule, agent, and skill (`skill-<name>.md`). |
| `windsurf` | `workflows-dir` | _empty_ | When set, each agent also emits as a Windsurf Workflow at `<dir>/<name>.md` (invokable as `/<name>`). The rule-form emission still happens. Opt-in. |
| `continue` | `rules-dir` | `.continue/rules` | One `.md` per rule, agent, and skill (`skill-<name>.md`). |
| `continue` | `mcp-dir` | `.continue/mcpServers` | One YAML per MCP server. |
| `continue` | `assistants-dir` | _empty_ | When set, each agent also emits as a Continue local Assistant YAML at `<dir>/<name>.yaml`. The rule-form emission still happens. Opt-in. |
| `amp` | `commands-dir` | `.agents/commands` | One `.md` per agent. |
| `amp` | `skills-dir` | `.agents/skills` | One folder per skill with a `SKILL.md` (Amp's native skills layout). |
| `amp` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `amp`. |
| `amp` | `mcp-file` | `.amp/settings.json` | Writes `amp.mcpServers` (dotted key). Pre-existing keys preserved. |
| `zed` | `file` | `.rules` | Single merged document. |
| `zed` | `mcp-file` | `.zed/settings.json` | `context_servers` map. Stdio uses `command`/`args`/`env`; HTTP/SSE use a native `url`/`headers` shape. |
| `zed` | `tasks-file` | _empty_ | When set, each hook emits as an on-demand Zed Task (`sh -c "<command>"`). Zed has no lifecycle-hook surface, so tasks run from the command palette. Opt-in. |
| `warp` | `workflows-dir` | _empty_ | When set, each agent emits as a Warp Workflow YAML at `<dir>/<name>.yaml`. |
| `warp` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `warp`. |
| `warp` | `mcp-file` | `.warp/.mcp.json` | Standard `mcpServers` schema. |
| `opencode` | `commands-dir` | `.opencode/commands` | One `.md` per agent (one per skill when `emit-skills-as-commands: true`). Frontmatter filtered to `description`, `agent`, `model`, `subtask`. |
| `opencode` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.opencode/commands/skill-<name>.md`. |
| `opencode` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `opencode`. |
| `opencode` | `mcp-file` | `opencode.json` | `mcp` map with `type: "local"\|"remote"`. Pre-existing user keys preserved. |
| `antigravity` | `rules-dir` | `.agent/rules` | One `.md` per rule and per agent. |
| `antigravity` | `skills-dir` | `.agent/skills` | One folder per skill (`<name>/SKILL.md`, Antigravity's native skills layout). |
| `antigravity` | `rules-file` | _empty_ | When set, writes a legacy merged document at that path. `sync` skips the pointer-body write for `antigravity`. |

## `targets`

When omitted: the 12 default adapters above (every adapter except `amp` and `warp`, which share `codex`'s root `AGENTS.md` pointer body and so add no new entry-point). Enabling `amp` or `warp` alongside `codex` is safe: the identical body is written once via auto-dedup. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

### Interactive target selection

`agnostic-ai init` opens a multi-select prompt when stdin is a TTY:

    agnostic-ai init

Use ↑/↓ to move, space to toggle, enter to confirm. The resulting `targets:` list contains only the chosen targets, in canonical order.

For scripted use (CI, integration tests), pipe a comma-separated line of target names:

    echo "claude,codex" | agnostic-ai init

Unknown names produce a clear error and leave the working tree untouched. Pass `--all` (`-a`) to skip the picker and enable every supported target. A non-TTY stdin with no piped data falls back to the full target list silently.

## `sync`

Sync-level knobs applied globally. Per-target overrides live in `outputs.<target>`.

### `sync.collision-policy`

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

### `sync.target-overview`

Off by default. When `true`, each target entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gains a generated appendix listing where that tool's generated artifacts live (rules dir, agents dir, MCP file, ...). The locations honor your `outputs.<target>.*` overrides.

```yaml
# In agnostic-ai.yaml
sync:
  target-overview: true
```

The canonical body stays identical across every target; only the appendix differs per file. An entry-point shared by several targets (codex, amp, and warp all read `AGENTS.md`) lists each consumer in its own section. The appendix sits between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->` markers; `import` strips it, so the `AGNOSTIC_AI.md` round-trip stays lossless. `.agnostic-ai/AGNOSTIC_AI.md` itself never carries the appendix. Do not hand-edit the block: every sync regenerates it.

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
  note: 3 skills reach warp only in the source dir (no native skill surface)
```

A note fires only when specs of that kind are present and the opt-in is inactive. Setting the named key clears the note and emits the content. Notes that match the previous sync are suppressed; delete `.agnostic-ai/.sync-state` to re-show them.

The instrumented gaps:

| Target | Kind | Set this to emit |
|--------|------|------------------|
| `gemini` | skills | `outputs.gemini.emit-skills-as-commands` |
| `opencode` | skills | `outputs.opencode.emit-skills-as-commands` |
| `warp` | agents | `outputs.warp.workflows-dir` |
| `warp` | skills | no key; Warp has no native skill surface (stays source-dir only) |
| `aider` | agents, skills | `outputs.aider.rules-file` |
| `zed` | hooks | `outputs.zed.tasks-file` |

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when the key is absent; `agnostic-ai init` writes `true` | When true, every `sync` rewrites a managed block in `.gitignore` listing every path the configured adapters emit. |
| `path` | `.gitignore` | Override the file location. Useful for monorepos or local-only ignore files. |
| `allow` | empty | Re-allow patterns emitted as `!`-prefixed lines at the end of the managed block. Keeps a tracked file (e.g. a `testdata/AGENTS.md` fixture) from being ignored by a broader rule, without hand-editing. Patterns are gitignore globs, emitted verbatim. |

The managed block is delimited by `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Two comment lines head the block: the first says to edit specs not the block, the second warns that the listed paths are not committed and a fresh clone or `git worktree` lacks them until `sync` runs (wire it into a [post-checkout hook](git-hooks.md#regenerate-on-checkout)). Lines outside the block are preserved as-is. Re-running `sync` with no spec changes is a no-op (file mtime unchanged). Every generated entry is root-anchored (`/AGENTS.md`, not `AGENTS.md`), so a generated file never ignores a same-named file nested elsewhere. Generated files under a tool subdirectory collapse to that subdirectory (`/.claude/rules/`, not one line per file), but the collapse stops at the generated subdir, so a hand-authored sibling such as `.claude/settings.json` or `.claude/hooks/` is never swallowed by a `/.claude/` ignore. For cases a root-anchored ignore can't express, add the glob to `allow`; its `!` line is written last so it overrides the ignores above it.

The block also owns the three fixed agnostic-ai paths that are never generated by an adapter: `agnostic-ai.local.yaml` (the per-machine override), `/.agnostic-ai/.sync-state`, and `/.agnostic-ai/packs/`. `init` seeds them even with `gitignore.enabled: false`, since they must never be committed; `sync` keeps them alongside the generated entries. Projects created by an older version carried these as loose lines outside the block (with a duplicated `.sync-state`); the next `init`, `sync`, or `packs add` strips the loose copies and folds them into the block.

A target may also contribute local artifacts it creates but agnostic-ai never emits, so `sync` keeps them ignored without hand maintenance. When `claude` is an enabled target, the block includes `/.claude/agent-memory/` (the agent memory store) and `/.claude/settings.local.json` (per-user local settings); both follow the `outputs.claude.dir` override. These entries are target-scoped: a project that does not enable `claude` never sees them.

Override per-run with `--gitignore on|off` on `sync`.

### Picking the default at `init`

`agnostic-ai init` writes `gitignore.enabled: true` by default, so a fresh project keeps its generated outputs out of git and `.agnostic-ai/` stays the single committed source. When stdin is a TTY the confirm prompt defaults to "Yes, ignore them"; pick "No" to commit emitted files instead (useful when teammates lack the CLI). Non-interactive runs (CI, piped stdin) take the default without prompting. Opt out with `--gitignore=false`:

    agnostic-ai init --all --gitignore=false

Note this only affects newly scaffolded configs. An existing `agnostic-ai.yaml` with no `gitignore` key keeps the absent-key default (`false`); add the block by hand to opt in.

## Claude settings

The `outputs.claude.settings` block declares first-class `.claude/settings.json` keys. The full layering, low to high precedence, is: captured overlay (from `import claude`) < agnostic `settings` specs (`.agnostic-ai/settings/`, the cross-tool source for `permissions` + `model`) < this `outputs.claude.settings` config < spec-derived `hooks` block. Keys you do not set fall through to the lower layers, so partial adoption works.

```yaml
outputs:
  claude:
    settings:
      model: claude-opus-4-7
      outputStyle: verbose
      apiKeyHelper: ./bin/keyhelper.sh
      cleanupPeriodDays: 30
      includeCoAuthoredBy: false
      enabledPlugins:
        - plugin-a
        - plugin-b
      env:
        FOO: bar
      statusLine:
        type: command
        command: echo status
        padding: 2
      permissions:
        allow:
          - "Read(*)"
        deny:
          - "Shell(rm *)"
        ask:
          - "Edit(*)"
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Default model preference for new sessions. |
| `outputStyle` | string | One of the Claude Code output styles. |
| `apiKeyHelper` | string | Path to a script that prints an API key on stdout. |
| `cleanupPeriodDays` | integer | Days of conversation history to retain. |
| `includeCoAuthoredBy` | boolean | Append `Co-Authored-By: Claude` to git commits Claude creates. |
| `enabledPlugins` | list of strings | Plugin identifiers Claude Code should load. |
| `env` | map of strings | Environment variables exported into Claude sessions. |
| `statusLine` | object | `type`, `command`, and optional `padding`. |
| `permissions` | object | `allow`, `deny`, `ask` lists of tool-pattern strings. |

Any setting not declared here round-trips through the overlay captured during `agnostic-ai import claude` (written to `.agnostic-ai/overlays/claude.settings.json`). When both the overlay and `outputs.claude.settings.*` declare the same scalar key, the first-class config block wins. The `permissions` lists are the exception: their `allow`/`deny`/`ask` entries are unioned across the overlay, any `settings` spec, and this config, so no layer silently drops another's rules.

## Codex config

The `outputs.codex.config` block declares first-class `.codex/config.toml` global keys, written into the project-tier config on each sync. Keys not listed here belong in the user-global `~/.codex/config.toml`, which Codex merges last.

```yaml
outputs:
  codex:
    config:
      model: o4-mini
      sandbox: workspace
      approval-policy: on-failure
      model-reasoning-effort: high
      model-reasoning-summary: auto
      history-persistence: project
      notify: ["python3", "/etc/codex/notify.py"]
      profiles:
        work:
          model: o4-mini
          sandbox: workspace-write
          approval-policy: on-failure
        oss:
          model: gpt-oss-20b
          model-provider: ollama
```

| Field | Type | Notes |
|-------|------|-------|
| `model` | string | Model identifier Codex uses for this project. |
| `sandbox` | string | Sandbox profile (e.g. `workspace`). |
| `approval-policy` | string | When Codex asks for approval: `never`, `on-failure`, or `always`. |
| `model-reasoning-effort` | string | Reasoning effort for o-series models: `low`, `medium`, `high`. |
| `model-reasoning-summary` | string | Reasoning summary verbosity: `auto`, `concise`, `detailed`. |
| `history-persistence` | string | Conversation history scope: `project`, `global`, or `none`. |
| `notify` | string array | External program Codex invokes on session events. First element is the executable; rest are arguments. |
| `profiles` | map | Named `[profiles.<name>]` blocks. Each entry overrides top-level fields when Codex runs with `--profile <name>`. Supported keys: `model`, `sandbox`, `approval-policy`, `model-reasoning-effort`, `model-reasoning-summary`, `model-provider`. |
| `model-providers` | map | Named `[model_providers.<id>]` blocks declaring backends Codex can call. Supported keys: `name`, `base-url`, `wire-api`, `api-key-env`, `env-key`. Reference an `id` from `profiles.<name>.model-provider`. |

### Codex exec-policies

`outputs.codex.exec-policies` (list) or `outputs.codex.exec-policies-file` (path to a YAML list) declares Codex CLI's Skylark-flavored exec-policy DSL, rendered into `.codex/rules/default.rules` on sync. Each entry allow- or forbid-lists a shell command prefix.

```yaml
outputs:
  codex:
    exec-policies:
      - pattern: ["composer", "test"]
        decision: allow           # allow | forbidden | ask
        justification: Composer scripts are project entrypoints.
        match: ["composer test", "composer test -- --filter Foo"]
      - pattern: ["rm", "-rf", "/"]
        decision: forbidden
        justification: Never remove the filesystem root.
```

| Field | Required | Notes |
|-------|----------|-------|
| `pattern` | yes | Shell command prefix tokens (`["composer", "test"]`). Becomes the `prefix_rule(pattern = [...])` argument. |
| `decision` | yes | One of `allow`, `forbidden`, `ask`. |
| `justification` | no | Free-form comment emitted above the rule as a `#` line. |
| `match` | no | Example matches rendered as commented `# match: ...` lines below the rule. Documentation only; Codex CLI ignores them. |

For many policies, keep them in a separate YAML file and point `exec-policies-file: ./.agnostic-ai/codex.exec-policies.yaml`. Inline entries render first, then file entries. Order matters: Codex evaluates rules top-down.

`agnostic-ai import codex` against a project that ships `.codex/rules/default.rules` captures every `prefix_rule(...)` call into `.agnostic-ai/overlays/codex.exec-policies.yaml`. The codex emitter auto-loads that overlay when no inline list and no explicit `exec-policies-file` is set, so the round-trip is byte-content-preserving without extra config.

The file is written only when at least one policy is declared. Otherwise nothing under `.codex/rules/` is created.

The codex emitter also reads `.agnostic-ai/overlays/codex.config.toml` (captured by `agnostic-ai import codex`) and prepends its body before the spec-derived `[[hooks.*]]` and `[mcp_servers.*]` sections. The overlay carries every other `.codex/config.toml` key the user has configured (`model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, ...) so wiping `.codex/` between `import` and `sync` no longer drops them. On conflict with `outputs.codex.config.*`, the overlay wins and the first-class key is dropped to keep the TOML valid.

## Watched inputs

`sync --watch` re-emits whenever any of these change on disk:

- `agnostic-ai.yaml` (and `agnostic-ai.local.yaml` when present)
- every directory listed under `sources` (agents, skills, rules, hooks, mcps, commands)
- `.agnostic-ai.local/` (the project-user spec layer)
- `.agnostic-ai/overlays/`: captured per-target settings (`claude.settings.json`, `codex.config.toml`). Hand-edit an overlay to change something the spec layer does not own (Claude `statusLine`, Codex `[profiles.*]`, ...) and watch re-runs `sync` within the 50 ms debounce window.

See [`sync --watch`](cli-reference.md#sync) for the polling fallback and debounce details.

## Path semantics

- All `sources`/`outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.
- Add generated outputs to `.gitignore` to keep specs as the single source of truth (recommended).

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus one root entry-point file per enabled target, all sharing the canonical pointer body. See the [per-target table](targets.md#entry-point-files) for which file each target uses.

To opt back into the legacy concatenated layout for a target, set `outputs.<target>.rules-file: <path>`. The adapter writes a single merged document at `<path>` and `sync` skips the pointer-body write for that target so the two do not collide.

Real collisions where two adapters write different content to the same path (e.g. both `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`) fail fast with an `output collision` error by default. Set `sync.collision-policy: prefer-spec` to skip the check and let the last adapter win, useful in CI. See [`sync.collision-policy`](#synccollision-policy).

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml` (deep-merged over the base when present)
4. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Specs load from up to three layers, low- to high-precedence:

| Layer | Root | Loaded when |
|-------|------|-------------|
| `user-global` | `$AGNOSTIC_AI_HOME` if set, else `~/.agnostic-ai` | directory exists |
| `project` | `agnostic-ai.yaml` `sources` paths | always |
| `project-user` | `<project>/.agnostic-ai.local` | directory exists |

Higher layers override by spec name (per kind). New names append.

`user-global` and `project-user` use a fixed source layout: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/` under the layer root. Only the `project` layer honors custom `sources` paths.

Add `.agnostic-ai.local/` to your `.gitignore` so personal overrides stay local.

`agnostic-ai list` prints each spec's source layer for debugging.
