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

# Source directories, relative to this file.
sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks
  mcps: mcps
  commands: commands

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
    # exec-policies:              # opt-in. Renders .codex/rules/default.rules.
    #   - pattern: ["composer", "test"]
    #     decision: allow          # allow | forbidden | ask
    #     justification: Run tests
    #     match: ["composer test"]
    # exec-policies-file: ./agnostic-ai/codex.exec-policies.yaml  # alt: external YAML list
  gemini:
    commands-dir: .gemini/commands   # default. One .toml per agent (and per skill when opted in).
    emit-skills-as-commands: false   # default
    mcp-file: .gemini/settings.json  # default. Holds both mcpServers and hooks.
  cursor:
    rules-dir: .cursor/rules     # default
    mcp-file: .cursor/mcp.json   # default
    # commands-dir: .cursor/commands  # opt-in: also emit each agent as a Cursor Custom Command.
  copilot:
    instructions-dir: .github/instructions  # default. One .instructions.md per scoped rule, agent, skill.
    mcp-file: .vscode/mcp.json              # default
    # chatmodes-dir: .github/chatmodes      # opt-in: also emit each agent as a Copilot Custom Chat Mode.
  aider:
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
    commands-dir: .agents/commands  # default. One .md per agent.
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
    commands-dir: .opencode/commands     # default. One .md per agent (and per skill when opted in).
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
| `claude` | `rules-file` | _empty_ | When set, switches to the legacy concatenated single-file layout at that path (typically `CLAUDE.md`). `sync` skips the pointer-body write for `claude`. |
| `claude` | `mcp-file` | `.mcp.json` | Standard `mcpServers` schema. |
| `claude` | `settings` | _empty_ | First-class block mirroring `.claude/settings.json` keys. See [Claude settings](#claude-settings). |
| `codex` | `agents-dir` | `.codex/agents` | One TOML per agent (Codex subagent schema). Override to `.agents/agents` for the community shared layout. |
| `codex` | `skills-dir` | `.codex/skills` | One folder per skill (Codex skills layout). Override to `.agents/skills` for the community shared layout. |
| `codex` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `codex`. |
| `codex` | `mcp-file` | `.codex/config.toml` | Holds `[mcp_servers.<name>]` tables. Hooks moved out into `hooks-file`. |
| `codex` | `hooks-file` | `.codex/hooks.json` | Per-event hook arrays in Claude `settings.json`-shaped JSON. Preferred over the legacy TOML `[[hooks.<event>]]` schema: preserves per-hook `timeout` + `statusMessage` and dedupes overlapping matchers. |
| `codex` | `config` | _empty_ | First-class block for `.codex/config.toml` global keys. See [Codex config](#codex-config). |
| _any_ | `provenance-header` | `true` | When `false`, suppresses the `Generated by agnostic-ai` comment in this target's files. Useful for byte-stable round-trip from hand-authored sources. Loses the legacy-file detection that comment enables, so opt out per-target rather than project-wide. |
| `gemini` | `commands-dir` | `.gemini/commands` | One TOML per agent (one per skill when `emit-skills-as-commands: true`). |
| `gemini` | `emit-skills-as-commands` | `false` | When true, skills also emit as `.gemini/commands/skill-<name>.toml`. |
| `gemini` | `rules-file` | _empty_ | When set, writes a legacy concatenated rules document at that path. `sync` skips the pointer-body write for `gemini`. |
| `gemini` | `mcp-file` | `.gemini/settings.json` | Holds both `mcpServers` and `hooks`. HTTP MCP entries use `httpUrl`. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule, agent, and skill (`skill-<name>.mdc`). |
| `cursor` | `commands-dir` | _empty_ | When set, each agent also emits as a Cursor Custom Command at `<dir>/<name>.md`. The rule-form `.mdc` emission still happens. Opt-in. |
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

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when the key is absent; `agnostic-ai init` writes `true` | When true, every `sync` rewrites a managed block in `.gitignore` listing every path the configured adapters emit. |
| `path` | `.gitignore` | Override the file location. Useful for monorepos or local-only ignore files. |
| `allow` | empty | Re-allow patterns emitted as `!`-prefixed lines at the end of the managed block. Keeps a tracked file (e.g. a `testdata/AGENTS.md` fixture) from being ignored by a broader rule, without hand-editing. Patterns are gitignore globs, emitted verbatim. |

The managed block is delimited by `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside the block are preserved as-is. Re-running `sync` with no spec changes is a no-op (file mtime unchanged). Every generated entry is root-anchored (`/AGENTS.md`, not `AGENTS.md`), so a generated file never ignores a same-named file nested elsewhere. For cases a root-anchored ignore can't express, add the glob to `allow`; its `!` line is written last so it overrides the ignores above it.

Override per-run with `--gitignore on|off` on `sync`.

### Picking the default at `init`

`agnostic-ai init` writes `gitignore.enabled: true` by default, so a fresh project keeps its generated outputs out of git and `.agnostic-ai/` stays the single committed source. When stdin is a TTY the confirm prompt defaults to "Yes, ignore them"; pick "No" to commit emitted files instead (useful when teammates lack the CLI). Non-interactive runs (CI, piped stdin) take the default without prompting. Opt out with `--gitignore=false`:

    agnostic-ai init --all --gitignore=false

Note this only affects newly scaffolded configs. An existing `agnostic-ai.yaml` with no `gitignore` key keeps the absent-key default (`false`); add the block by hand to opt in.

## Claude settings

The `outputs.claude.settings` block declares first-class `.claude/settings.json` keys. agnostic-ai writes each non-empty field on top of the captured overlay (from `import claude`) and below the spec-derived `hooks` block. Keys you do not set fall through to the overlay, so partial adoption works.

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

Any setting not declared here round-trips through the overlay captured during `agnostic-ai import claude` (written to `.agnostic-ai/overlays/claude.settings.json`). When both the overlay and `outputs.claude.settings.*` declare the same key, the first-class config block wins.

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
