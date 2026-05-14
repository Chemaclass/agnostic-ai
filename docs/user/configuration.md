# Configuration

`agnostic-ai.yaml` lives at the project root. Read from the current working directory at command time. Every section is optional; defaults below.

> **Legacy filename:** projects that still use `agnostic.config.yaml` continue to work. The CLI loads it with a deprecation warning. Rename to `agnostic-ai.yaml` when convenient.

## Local overrides

Drop an `agnostic-ai.local.yaml` next to the base config to layer per-machine tweaks without touching the shared file. Loaded after the base and deep-merged: scalars and lists in the local file replace the base, maps merge recursively. `agnostic-ai init` adds the local filename to `.gitignore` automatically.

```yaml
# agnostic-ai.local.yaml — never committed
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

## Editor validation

The JSON Schema for this file is published at `docs/schemas/config.schema.json`. The `init` command embeds a `yaml-language-server` comment in the generated file:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/main/docs/schemas/config.schema.json
```

Editors with YAML Language Server support pick this up automatically and validate keys and values as you type.

## Full schema

```yaml
version: 1

# Source directories, relative to this file.
sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks
  mcps: mcps
  commands: commands

# AI CLIs to emit configs for.
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
  - amp
  - zed
  - warp
  - opencode
  - antigravity

# Per-target output overrides. Each target accepts only the fields
# relevant to it. Defaults shown in comments.
outputs:
  claude:
    dir: .claude                 # default
    rules-file: CLAUDE.md        # default
    commands-dir: .claude/commands # default. One .md per command (slash prompt).
    mcp-file: .mcp.json          # default
  codex:
    file: AGENTS.md              # default. Hierarchical per scope.
    agents-dir: .agents/agents   # default. One TOML per agent.
    skills-dir: .agents/skills   # default. One folder per skill.
    commands-dir: .codex/prompts # default. One .md per command (slash prompt).
    mcp-file: .codex/config.toml # default. Holds both [[hooks.<event>]] and [mcp_servers.<name>].
  gemini:
    file: GEMINI.md                  # default. Hierarchical per scope.
    commands-dir: .gemini/commands   # default. One .toml per agent (and per skill when opted in).
    emit-skills-as-commands: false   # default
    mcp-file: .gemini/settings.json  # default. Holds both mcpServers and hooks.
  cursor:
    rules-dir: .cursor/rules     # default
    mcp-file: .cursor/mcp.json   # default
  copilot:
    file: .github/copilot-instructions.md   # default. Always-on rules.
    instructions-dir: .github/instructions  # default. One .instructions.md per scoped rule, agent, skill.
    mcp-file: .vscode/mcp.json              # default
  aider:
    file: CONVENTIONS.md         # default
    # Opt-in: also merge .aider.conf.yml so Aider auto-loads CONVENTIONS.md.
    # conf-file: .aider.conf.yml
    # model: gpt-4o
    # weak-model: gpt-4o-mini
  cline:
    rules-dir: .clinerules       # default
  windsurf:
    rules-dir: .windsurf/rules   # default
  continue:
    rules-dir: .continue/rules        # default
    mcp-dir: .continue/mcpServers     # default. One YAML per MCP server.
  amp:
    file: AGENTS.md                 # default. Hierarchical per scope; was AGENT.md.
    commands-dir: .agents/commands  # default. One .md per agent (and per skill when opted in).
    emit-skills-as-commands: false  # default
    mcp-file: .amp/settings.json    # default. amp.mcpServers (dotted key).
  zed:
    file: .rules                  # default
    mcp-file: .zed/settings.json  # default. context_servers (stdio native, HTTP via mcp-remote).
  warp:
    file: AGENTS.md             # default. Hierarchical per scope; was WARP.md.
    mcp-file: .warp/.mcp.json   # default. Standard mcpServers schema.
  opencode:
    file: .opencode/AGENTS.md            # default. Routed under .opencode/ to coexist with Codex.
    commands-dir: .opencode/commands     # default. One .md per agent (and per skill when opted in).
    emit-skills-as-commands: false       # default
    mcp-file: opencode.json              # default. mcp map with type: local|remote.
  antigravity:
    file: .agent/AGENTS.md              # default. Namespaced to avoid root AGENTS.md collision.
    rules-dir: .agent/rules             # default. One .md per rule and per agent.

# What to do when a spec kind is unsupported by a target
# (e.g. hooks for any target other than claude).
on-unsupported: warn   # warn | error | silent

# Auto-manage a block in .gitignore listing every generated path.
# When enabled, `sync` rewrites the block; `--gitignore on|off` overrides
# this setting for one run.
gitignore:
  enabled: false
  path: .gitignore   # default

# Persisted answer to the first-run auto-sync prompt. Set by
# `sync --auto-sync=yes|no` or interactively on first sync. When true,
# `sync` writes an `auto-sync` rule spec telling agents to run
# `agnostic-ai sync` whenever specs change.
autoSync: false
```

## Top-level fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Reserved for future migrations. |
| `sources` | map | see below | Per-kind source directories (relative to config file). |
| `targets` | list | all 14 adapters | Adapter names to emit. Unknown targets log a warning and skip. |
| `outputs` | map | see below | Per-target output path overrides. |
| `on-unsupported` | string | `warn` | How to react when a kind is unsupported by a target. One of `warn`, `error`, `silent`. |
| `gitignore` | map | `enabled: false` | Auto-manage a block in `.gitignore` listing generated paths. See [`gitignore`](#gitignore). |
| `autoSync` | bool | unset | Whether `sync` keeps the `auto-sync` rule spec. Set by the first-run prompt or `sync --auto-sync=yes\|no`. Unset means the prompt has not yet fired. |

## `sources`

| Field | Default | Description |
|-------|---------|-------------|
| `agents` | `agents` | Directory containing `*.md` agent specs. |
| `skills` | `skills` | Directory containing `*.md` skill specs (or nested `<name>/SKILL.md`). |
| `rules` | `rules` | Directory containing `*.md` rule specs. |
| `hooks` | `hooks` | Directory containing `*.yaml` hook specs. |
| `mcps` | `mcps` | Directory containing `*.yaml` MCP server specs. |

Paths are relative to the config file. Missing directories are skipped silently.

## `outputs`

Per-target paths. Each target reads only the fields it understands; irrelevant fields are ignored.

| Target | Field | Default | Notes |
|--------|-------|---------|-------|
| `claude` | `dir` | `.claude` | Holds `agents/`, `skills/`, `settings.json`. |
| `claude` | `rules-file` | `CLAUDE.md` | Concatenated rules document. |
| `claude` | `mcp-file` | `.mcp.json` | Standard `mcpServers` schema. |
| `codex` | `file` | `AGENTS.md` | Rules document; nested `<dir>/AGENTS.md` files share this base. Lists agents and skills with pointers to their per-target files. |
| `codex` | `agents-dir` | `.agents/agents` | One TOML file per agent (Codex subagent schema). |
| `codex` | `skills-dir` | `.agents/skills` | One folder per skill per the Codex skills layout. |
| `codex` | `mcp-file` | `.codex/config.toml` | Holds both `[[hooks.<event>]]` arrays and `[mcp_servers.<name>]` tables. |
| `gemini` | `file` | `GEMINI.md` | Root rules + agent/skill references. Hierarchical: nested `<scope>/GEMINI.md` files share this base. |
| `gemini` | `commands-dir` | `.gemini/commands` | One TOML per agent (one per skill when `emit-skills-as-commands: true`). |
| `gemini` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.gemini/commands/skill-<name>.toml`. |
| `gemini` | `mcp-file` | `.gemini/settings.json` | Holds both `mcpServers` and `hooks`. HTTP MCP entries use `httpUrl`. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule and per agent. |
| `cursor` | `mcp-file` | `.cursor/mcp.json` | Standard `mcpServers` schema. |
| `copilot` | `file` | `.github/copilot-instructions.md` | Always-on rules (`alwaysApply: true` or no scope). |
| `copilot` | `instructions-dir` | `.github/instructions` | One `.instructions.md` per scoped rule, agent, skill. `applyTo:` frontmatter derived from `globs` or scope. |
| `copilot` | `mcp-file` | `.vscode/mcp.json` | VS Code schema: top-level `servers` with `type` field per entry. |
| `aider` | `file` | `CONVENTIONS.md` | Single merged document. |
| `aider` | `conf-file` | _empty_ | When set, merges `.aider.conf.yml` so Aider auto-loads `CONVENTIONS.md`. Pre-existing keys preserved; `read:` list de-duplicates. Opt-in. |
| `aider` | `model` | _empty_ | Optional `model:` value written into the conf file. |
| `aider` | `weak-model` | _empty_ | Optional `weak-model:` value written into the conf file. |
| `cline` | `rules-dir` | `.clinerules` | One `.md` per rule and per agent. |
| `windsurf` | `rules-dir` | `.windsurf/rules` | One `.md` per rule and per agent. |
| `continue` | `rules-dir` | `.continue/rules` | One `.md` per rule and per agent. |
| `continue` | `mcp-dir` | `.continue/mcpServers` | One YAML per MCP server. |
| `amp` | `file` | `AGENTS.md` | Hierarchical: nested `<scope>/AGENTS.md` files share this base. Renamed from `AGENT.md`; legacy file migrated to `AGENT.md.bak` on first sync. |
| `amp` | `commands-dir` | `.agents/commands` | One `.md` per agent (one per skill when `emit-skills-as-commands: true`). |
| `amp` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.agents/commands/skill-<name>.md`. |
| `amp` | `mcp-file` | `.amp/settings.json` | Writes `amp.mcpServers` (dotted key). Pre-existing keys preserved. |
| `zed` | `file` | `.rules` | Single merged document. |
| `zed` | `mcp-file` | `.zed/settings.json` | `context_servers` map. Stdio native; HTTP/SSE auto-bridges via `npx mcp-remote`. |
| `warp` | `file` | `AGENTS.md` | Hierarchical: nested `<scope>/AGENTS.md` files share this base. Renamed from `WARP.md`; legacy file migrated to `WARP.md.bak` on first sync. |
| `warp` | `mcp-file` | `.warp/.mcp.json` | Standard `mcpServers` schema. |
| `opencode` | `file` | `.opencode/AGENTS.md` | Rules + agent/skill references; routed under `.opencode/` to coexist with Codex. |
| `opencode` | `commands-dir` | `.opencode/commands` | One `.md` per agent (one per skill when `emit-skills-as-commands: true`). Frontmatter filtered to `description`, `agent`, `model`, `subtask`. |
| `opencode` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.opencode/commands/skill-<name>.md`. |
| `opencode` | `mcp-file` | `opencode.json` | `mcp` map with `type: "local"\|"remote"`. Pre-existing user keys preserved. |

## `targets`

When omitted: all 13 adapters above. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

### Interactive target selection

`agnostic-ai init` opens a multi-select prompt when stdin is a TTY so most projects only enable a handful of targets:

    agnostic-ai init

Use ↑/↓ to move, space to toggle, enter to confirm. The resulting `targets:` list contains only the chosen targets, in canonical order.

For scripted use (CI, integration tests), pipe a comma-separated line of target names instead:

    echo "claude,codex" | agnostic-ai init

Unknown names produce a clear error and leave the working tree untouched. To skip the picker entirely and enable every supported target, pass `--all` (`-a`). A non-TTY stdin with no piped data falls back to the full target list silently — CI invocations keep working without flags.

## `on-unsupported`

| Value | Behavior |
|-------|----------|
| `warn` | Log to stderr and continue. Default. |
| `error` | Fail the sync. |
| `silent` | Skip without logging. |

Fires when an adapter receives spec kinds it does not support, e.g. `hooks` for Codex or `skills` for Cursor.

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | When true, every `sync` rewrites a managed block in `.gitignore` listing every path the configured adapters would emit. |
| `path` | `.gitignore` | Override the file location. Useful for monorepos or local-only ignore files. |

The managed block is delimited by `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside the block are preserved as-is. Re-running `sync` with no spec changes is a no-op (file mtime unchanged).

Override per-run with `--gitignore on|off` on `sync`.

## `autoSync`

Persisted answer to the first-run auto-sync prompt. Three states:

| Value | Meaning |
|-------|---------|
| unset | Prompt has not fired. On a TTY, the next `sync` asks; non-TTY runs are silent. |
| `true` | Agents are expected to run `agnostic-ai sync` when specs change. The `auto-sync` rule spec under `sources.rules` carries the instruction. |
| `false` | User opted out. No prompt on subsequent syncs. |

Set non-interactively with `agnostic-ai sync --auto-sync=yes` or
`--auto-sync=no`. The prompt and persistence are skipped under `--dry-run`.

## Path semantics

- All `sources`/`outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.
- Add generated outputs to `.gitignore` to keep specs as the single source of truth (recommended).

## Output collisions

`codex`, `amp`, and `warp` all default to root `AGENTS.md` per the
community [agents.md](https://agents.md) spec. Only one adapter can own
the root file in a project.

`sync` and `sync --check` fail fast with an `output collision` error
listing every duplicated path and the targets that emit to it. Resolve
by either:

1. Dropping one of the colliding targets from `targets:` in `agnostic-ai.yaml`.
2. Overriding the colliding path on the loser via `outputs.<target>.file`
   (note: most CLIs only read their canonical filename, so this is
   rarely useful in practice).

The shipped defaults keep `codex` as the AGENTS.md owner. Users on Amp
or Warp instead must enable that target explicitly and drop `codex`
(and any other AGENTS.md owner) from `targets:`.

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml` (deep-merged over the base when present)
4. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Specs load from up to three layers, low- to high-precedence:

| Layer          | Root                                                | Loaded when         |
|----------------|-----------------------------------------------------|---------------------|
| `user-global`  | `$AGNOSTIC_AI_HOME` if set, else `~/.agnostic-ai`   | directory exists    |
| `project`      | `agnostic-ai.yaml` `sources` paths                  | always              |
| `project-user` | `<project>/.agnostic-ai.local`                      | directory exists    |

Higher layers override by spec name (per kind). New names append.

`user-global` and `project-user` use a fixed source layout: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/` under the layer root. Only the `project` layer honors custom `sources` paths.

Add `.agnostic-ai.local/` to your `.gitignore` so personal overrides stay local.

`agnostic-ai list` prints each spec's source layer for debugging.
