# Configuration

`agnostic.config.yaml` lives at the project root. Read from the current working directory at command time. Every section is optional; defaults below.

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

# Per-target output overrides. Each target accepts only the fields
# relevant to it. Defaults shown in comments.
outputs:
  claude:
    dir: .claude               # default
    rules-file: CLAUDE.md      # default
  codex:
    file: AGENTS.md            # default
    agents-dir: .codex/agents  # default. One TOML per agent (Codex subagent schema).
    skills-dir: .agents/skills # default. One folder per skill per the Codex skills layout.
  gemini:
    file: GEMINI.md            # default
  cursor:
    rules-dir: .cursor/rules   # default
  copilot:
    file: .github/copilot-instructions.md  # default
  aider:
    file: CONVENTIONS.md       # default
  cline:
    rules-dir: .clinerules     # default
  windsurf:
    rules-dir: .windsurf/rules # default
  continue:
    rules-dir: .continue/rules # default
  amp:
    file: AGENT.md             # default
  zed:
    file: .rules               # default
  warp:
    file: WARP.md              # default
  opencode:
    file: .opencode/AGENTS.md  # default. Routed under .opencode/ to coexist with Codex.

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
| `targets` | list | all 13 adapters | Adapter names to emit. Unknown targets log a warning and skip. |
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
| `codex` | `file` | `AGENTS.md` | Rules document; nested `<dir>/AGENTS.md` files share this base. Lists agents and skills with pointers to their per-target files. |
| `codex` | `agents-dir` | `.codex/agents` | One TOML file per agent (Codex subagent schema). |
| `codex` | `skills-dir` | `.agents/skills` | One folder per skill (`<name>/SKILL.md`, optional `<name>/agents/openai.yaml`) per the Codex skills layout. |
| `gemini` | `file` | `GEMINI.md` | Single merged document. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule and per agent. |
| `copilot` | `file` | `.github/copilot-instructions.md` | Single merged document. |
| `aider` | `file` | `CONVENTIONS.md` | Single merged document. |
| `cline` | `rules-dir` | `.clinerules` | One `.md` per rule and per agent. |
| `windsurf` | `rules-dir` | `.windsurf/rules` | One `.md` per rule and per agent. |
| `continue` | `rules-dir` | `.continue/rules` | One `.md` per rule and per agent. |
| `amp` | `file` | `AGENT.md` | Single merged document. |
| `zed` | `file` | `.rules` | Single merged document. |
| `warp` | `file` | `WARP.md` | Single merged document. |
| `opencode` | `file` | `.opencode/AGENTS.md` | Single merged document; routed under `.opencode/` to coexist with Codex. |

## `targets`

When omitted: all 13 adapters above. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

### Interactive target selection

By default, `agnostic-ai init` writes every supported target into the generated config. Most projects only use a handful. Pass `-i` (or `--interactive`) to pick:

    agnostic-ai init -i

Use ↑/↓ to move, space to toggle, enter to confirm. The resulting `targets:` list contains only the chosen targets, in canonical order.

For scripted use (CI, integration tests), pipe a comma-separated line of target names instead:

    echo "claude,codex" | agnostic-ai init -i

Unknown names, an empty selection, or a non-TTY stdin without piped data each produce a clear error and leave the working tree untouched.

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

- All `sources`/`outputs` paths are relative to the directory holding `agnostic.config.yaml`.
- Output directories are created on demand. Existing files are overwritten.
- Add generated outputs to `.gitignore` to keep specs as the single source of truth (recommended).

## Precedence

Last wins:

1. Built-in defaults
2. `agnostic.config.yaml`
3. CLI flags (e.g. `agnostic-ai sync -t claude`)

## Layered specs

Specs load from up to three layers, low- to high-precedence:

| Layer          | Root                                                | Loaded when         |
|----------------|-----------------------------------------------------|---------------------|
| `user-global`  | `$AGNOSTIC_AI_HOME` if set, else `~/.agnostic-ai`   | directory exists    |
| `project`      | `agnostic.config.yaml` `sources` paths              | always              |
| `project-user` | `<project>/.agnostic-ai.local`                      | directory exists    |

Higher layers override by spec name (per kind). New names append.

`user-global` and `project-user` use a fixed source layout: `agents/`, `skills/`, `rules/`, `hooks/`, `mcps/` under the layer root. Only the `project` layer honors custom `sources` paths.

Add `.agnostic-ai.local/` to your `.gitignore` so personal overrides stay local.

`agnostic-ai list` prints each spec's source layer for debugging.
