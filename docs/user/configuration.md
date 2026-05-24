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
# relevant to it. Defaults shown in comments. The project-root
# entry-point file (CLAUDE.md, AGENTS.md, GEMINI.md, CONVENTIONS.md,
# .github/copilot-instructions.md, .opencode/AGENTS.md,
# .agent/AGENTS.md) is written centrally by `sync` and is not
# adapter-configurable.
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
    # rules-file: CLAUDE.md      # opt-in: legacy concatenated rules layout.
  codex:
    agents-dir: .codex/agents    # default. One TOML per agent. Set to .agents/agents for the shared community layout.
    skills-dir: .codex/skills    # default. One folder per skill. Set to .agents/skills for the community shared layout.
    # shared-subagents: <bool>  # default: true (emit skills at skills-dir).
    #                            # Set false to skip codex skill emission entirely,
    #                            # useful when Codex CLI is configured to read
    #                            # claude's .claude/skills/ tree directly.
    commands-dir: .codex/prompts # default. One .md per command (slash prompt).
    mcp-file: .codex/config.toml # default. Holds both [[hooks.<event>]] and [mcp_servers.<name>].
  gemini:
    commands-dir: .gemini/commands   # default. One .toml per agent (and per skill when opted in).
    emit-skills-as-commands: false   # default
    mcp-file: .gemini/settings.json  # default. Holds both mcpServers and hooks.
  cursor:
    rules-dir: .cursor/rules     # default
    mcp-file: .cursor/mcp.json   # default
  copilot:
    instructions-dir: .github/instructions  # default. One .instructions.md per scoped rule, agent, skill.
    mcp-file: .vscode/mcp.json              # default
  aider:
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
    commands-dir: .agents/commands  # default. One .md per agent (and per skill when opted in).
    emit-skills-as-commands: false  # default
    mcp-file: .amp/settings.json    # default. amp.mcpServers (dotted key).
  zed:
    file: .rules                  # default
    mcp-file: .zed/settings.json  # default. context_servers (stdio native, HTTP via mcp-remote).
  warp:
    # workflows-dir: .warp/workflows  # opt-in: emit Warp Workflow YAMLs per agent.
    mcp-file: .warp/.mcp.json     # default. Standard mcpServers schema.
  opencode:
    commands-dir: .opencode/commands     # default. One .md per agent (and per skill when opted in).
    emit-skills-as-commands: false       # default
    mcp-file: opencode.json              # default. mcp map with type: local|remote.
  antigravity:
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
| `sync` | map | see below | Sync-level knobs. See [`sync`](#sync). |

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
| `claude` | `rules-dir` | `.claude/rules` | One `.md` per rule (default). |
| `claude` | `rules-file` | _empty_ | When set, switches back to the legacy concatenated single-file layout at that path (typically `CLAUDE.md`). `sync` skips the pointer-body write for `claude`. |
| `claude` | `mcp-file` | `.mcp.json` | Standard `mcpServers` schema. |
| `claude` | `settings` | _empty_ | First-class block mirroring `.claude/settings.json` keys. See [Claude settings](#claude-settings). |
| `codex` | `agents-dir` | `.codex/agents` | One TOML file per agent (Codex subagent schema). Override to `.agents/agents` for the community shared layout. |
| `codex` | `skills-dir` | `.codex/skills` | One folder per skill per the Codex skills layout. Override to `.agents/skills` for the community shared layout. |
| `codex` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `codex`. |
| `codex` | `mcp-file` | `.codex/config.toml` | Holds both `[[hooks.<event>]]` arrays and `[mcp_servers.<name>]` tables. |
| `codex` | `config` | _empty_ | First-class block for `.codex/config.toml` global keys. See [Codex config](#codex-config). |
| `gemini` | `commands-dir` | `.gemini/commands` | One TOML per agent (one per skill when `emit-skills-as-commands: true`). |
| `gemini` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.gemini/commands/skill-<name>.toml`. |
| `gemini` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `gemini`. |
| `gemini` | `mcp-file` | `.gemini/settings.json` | Holds both `mcpServers` and `hooks`. HTTP MCP entries use `httpUrl`. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule and per agent. |
| `cursor` | `mcp-file` | `.cursor/mcp.json` | Standard `mcpServers` schema. |
| `copilot` | `instructions-dir` | `.github/instructions` | One `.instructions.md` per scoped rule, agent, skill. `applyTo:` frontmatter derived from `globs` or scope. |
| `copilot` | `rules-file` | _empty_ | When set, writes always-on rules concatenated at that path (legacy layout). `sync` skips the pointer-body write for `copilot`. |
| `copilot` | `mcp-file` | `.vscode/mcp.json` | VS Code schema: top-level `servers` with `type` field per entry. |
| `aider` | `rules-file` | _empty_ | When set, writes a legacy merged document at that path (typically `CONVENTIONS.md`). `sync` skips the pointer-body write for `aider`. |
| `aider` | `conf-file` | _empty_ | When set, merges `.aider.conf.yml` so Aider auto-loads `CONVENTIONS.md`. Pre-existing keys preserved; `read:` list de-duplicates. Opt-in. |
| `aider` | `model` | _empty_ | Optional `model:` value written into the conf file. |
| `aider` | `weak-model` | _empty_ | Optional `weak-model:` value written into the conf file. |
| `cline` | `rules-dir` | `.clinerules` | One `.md` per rule and per agent. |
| `windsurf` | `rules-dir` | `.windsurf/rules` | One `.md` per rule and per agent. |
| `continue` | `rules-dir` | `.continue/rules` | One `.md` per rule and per agent. |
| `continue` | `mcp-dir` | `.continue/mcpServers` | One YAML per MCP server. |
| `amp` | `commands-dir` | `.agents/commands` | One `.md` per agent (one per skill when `emit-skills-as-commands: true`). |
| `amp` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.agents/commands/skill-<name>.md`. |
| `amp` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `amp`. |
| `amp` | `mcp-file` | `.amp/settings.json` | Writes `amp.mcpServers` (dotted key). Pre-existing keys preserved. |
| `zed` | `file` | `.rules` | Single merged document. |
| `zed` | `mcp-file` | `.zed/settings.json` | `context_servers` map. Stdio native; HTTP/SSE auto-bridges via `npx mcp-remote`. |
| `warp` | `workflows-dir` | _empty_ | When set, each agent emits as a Warp Workflow YAML at `<dir>/<name>.yaml`. |
| `warp` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `warp`. |
| `warp` | `mcp-file` | `.warp/.mcp.json` | Standard `mcpServers` schema. |
| `opencode` | `commands-dir` | `.opencode/commands` | One `.md` per agent (one per skill when `emit-skills-as-commands: true`). Frontmatter filtered to `description`, `agent`, `model`, `subtask`. |
| `opencode` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.opencode/commands/skill-<name>.md`. |
| `opencode` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `opencode`. |
| `opencode` | `mcp-file` | `opencode.json` | `mcp` map with `type: "local"\|"remote"`. Pre-existing user keys preserved. |
| `antigravity` | `rules-dir` | `.agent/rules` | One `.md` per rule and per agent. |
| `antigravity` | `rules-file` | _empty_ | When set, writes a legacy merged document at that path. `sync` skips the pointer-body write for `antigravity`. |

## `targets`

When omitted: all 13 adapters above. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

### Interactive target selection

`agnostic-ai init` opens a multi-select prompt when stdin is a TTY so most projects only enable a handful of targets:

    agnostic-ai init

Use ↑/↓ to move, space to toggle, enter to confirm. The resulting `targets:` list contains only the chosen targets, in canonical order.

For scripted use (CI, integration tests), pipe a comma-separated line of target names instead:

    echo "claude,codex" | agnostic-ai init

Unknown names produce a clear error and leave the working tree untouched. To skip the picker entirely and enable every supported target, pass `--all` (`-a`). A non-TTY stdin with no piped data falls back to the full target list silently — CI invocations keep working without flags.

## `sync`

Sync-level knobs applied globally. Per-target overrides live in `outputs.<target>`.

### `sync.collision-policy`

Controls what happens when two enabled targets would emit to the same output path.

| Value | Behavior |
|-------|----------|
| `prompt` | Error with a resolution hint. Default. On non-interactive stdin (CI), appends a hint to set a non-interactive policy. |
| `prefer-spec` | Skip the collision pre-flight. Let the last adapter win. Use this in CI when you intentionally enable overlapping targets and accept last-writer-wins. |
| `fail` | Hard error with no resolution hint. |

Per-target override: `outputs.<target>.collision-policy`.

```yaml
# In agnostic-ai.yaml
sync:
  collision-policy: prefer-spec   # CI-safe: skip collision check
```

## `import`

Per-source knobs for the `import` command. Empty blocks fall back to per-source defaults.

### `import.codex.shred`

Controls how `agnostic-ai import codex` treats `AGENTS.md`.

| Value | Behavior |
|-------|----------|
| `true` | Split each `AGENTS.md` into one rule spec per `##` heading. Default. |
| `false` | Keep each `AGENTS.md` as a single rule spec (full body verbatim). Use when codex `AGENTS.md` duplicates policy already authored as standalone rules and you want it as a reference doc, not a source of new rule files. |

```yaml
# In agnostic-ai.yaml
import:
  codex:
    shred: false   # one rule per AGENTS.md, no H2 sharding
```

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

### Picking the default at `init`

`agnostic-ai init` asks whether to enable the managed block when stdin is a TTY. Pick "No" (default) to commit emitted target files; pick "Yes" to treat them as build artifacts. Non-interactive runs (CI, piped stdin) skip the prompt — pass `--gitignore` to opt in:

    agnostic-ai init --all --gitignore

## Claude settings

The `outputs.claude.settings` block declares first-class
`.claude/settings.json` keys. agnostic-ai writes each non-empty field
on top of the captured overlay (from `import claude`) and below the
spec-derived `hooks` block. Keys you do not set fall through to the
overlay, so partial adoption is supported.

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

Any setting not declared here still round-trips through the overlay
captured during `agnostic-ai import claude` (written to
`.agnostic-ai/overlays/claude.settings.json`). When both the overlay
and `outputs.claude.settings.*` declare the same key, the
first-class config block wins.

## Codex config

The `outputs.codex.config` block declares first-class `.codex/config.toml`
global keys. agnostic-ai writes these into the project-tier config on each
sync. Keys not listed here belong in the user-global `~/.codex/config.toml`
which Codex merges last.

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
| `model-reasoning-effort` | string | Reasoning effort level for o-series models: `low`, `medium`, `high`. |
| `model-reasoning-summary` | string | Reasoning summary verbosity: `auto`, `concise`, `detailed`. |
| `history-persistence` | string | Conversation history scope: `project`, `global`, or `none`. |
| `notify` | string array | External program Codex invokes on session events. First element is the executable; rest are arguments. |
| `profiles` | map | Named `[profiles.<name>]` blocks. Each entry overrides top-level fields when Codex runs with `--profile <name>`. Supported keys: `model`, `sandbox`, `approval-policy`, `model-reasoning-effort`, `model-reasoning-summary`, `model-provider`. |
| `model-providers` | map | Named `[model_providers.<id>]` blocks declaring backends Codex can call. Supported keys: `name`, `base-url`, `wire-api`, `api-key-env`, `env-key`. Reference an `id` from `profiles.<name>.model-provider`. |

The codex emitter also reads `.agnostic-ai/overlays/codex.config.toml`
(captured by `agnostic-ai import codex`) and prepends its body before
the spec-derived `[[hooks.*]]` and `[mcp_servers.*]` sections. The
overlay carries every other `.codex/config.toml` key the user has
configured (`model`, `sandbox`, `approval_policy`, `notify`,
`[history]`, `[profiles.*]`, `[model_providers.*]`, …) so a wipe of
`.codex/` between `import` and `sync` no longer drops them. On
conflict with `outputs.codex.config.*` the overlay wins and the
first-class key is dropped to keep the TOML valid.

## Watched inputs

`sync --watch` re-emits whenever any of the following change on disk:

- `agnostic-ai.yaml` (and `agnostic-ai.local.yaml` when present)
- every directory listed under `sources` (agents, skills, rules, hooks, mcps, commands)
- `.agnostic-ai.local/` (the project-user spec layer)
- `.agnostic-ai/overlays/` — captured per-target settings (`claude.settings.json`, `codex.config.toml`). Hand-edit the overlay to change something the spec layer does not own (Claude `statusLine`, Codex `[profiles.*]`, …) and watch re-runs `sync` within the 50 ms debounce window.

See [`sync --watch`](cli-reference.md#sync) for the polling fallback and debounce details.

## Path semantics

- All `sources`/`outputs` paths are relative to the directory holding `agnostic-ai.yaml`.
- Output directories are created on demand. Existing files are overwritten.
- Add generated outputs to `.gitignore` to keep specs as the single source of truth (recommended).

## Entry-point files

`sync` writes `.agnostic-ai/AGNOSTIC_AI.md` plus a conventional root
entry-point file for every enabled target (`CLAUDE.md`, `AGENTS.md`,
`GEMINI.md`, `CONVENTIONS.md`, `.github/copilot-instructions.md`,
`.opencode/AGENTS.md`, `.agent/AGENTS.md`). All files share the same
canonical pointer body. Targets that share an entry-point path (codex,
amp, and warp all at `AGENTS.md`) write the file once; the dedup is
automatic.

To opt back into the legacy concatenated layout for a target, set
`outputs.<target>.rules-file: <path>`. The adapter then writes a
single merged document at `<path>` and `sync` skips the pointer-body
write for that target so the two do not collide.

Real collisions where two adapters write different content to the
same path (e.g. both `outputs.codex.rules-file: AGENTS.md` and
`outputs.amp.rules-file: AGENTS.md`) fail fast with an
`output collision` error by default. Set `sync.collision-policy: prefer-spec`
to skip the check and let the last adapter win, which is useful in CI.
See [`sync.collision-policy`](#synccollision-policy).

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
