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

# Per-target output overrides. Each target accepts only the fields
# relevant to it. Defaults shown in comments.
outputs:
  claude:
    dir: .claude               # default
    rules-file: CLAUDE.md      # default
  codex:
    file: AGENTS.md            # default
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
| `targets` | list | all 9 adapters | Adapter names to emit. Unknown targets log a warning and skip. |
| `outputs` | map | see below | Per-target output path overrides. |
| `on-unsupported` | string | `warn` | How to react when a kind is unsupported by a target. One of `warn`, `error`, `silent`. |
| `gitignore` | map | `enabled: false` | Auto-manage a block in `.gitignore` listing generated paths. See [`gitignore`](#gitignore). |

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
| `codex` | `file` | `AGENTS.md` | Single merged document (rules + agent listing + skill listing). Nested `<dir>/AGENTS.md` files share this base. |
| `codex` | `agents-dir` | `.codex/agents` | One TOML file per agent (Codex subagent schema). |
| `gemini` | `file` | `GEMINI.md` | Single merged document. |
| `cursor` | `rules-dir` | `.cursor/rules` | One `.mdc` per rule and per agent. |
| `copilot` | `file` | `.github/copilot-instructions.md` | Single merged document. |
| `aider` | `file` | `CONVENTIONS.md` | Single merged document. |
| `cline` | `rules-dir` | `.clinerules` | One `.md` per rule and per agent. |
| `windsurf` | `rules-dir` | `.windsurf/rules` | One `.md` per rule and per agent. |
| `continue` | `rules-dir` | `.continue/rules` | One `.md` per rule and per agent. |

## `targets`

When omitted: all 9 adapters above. Comment out entries to disable targets. CLI flag `-t/--target` overrides for a single run.

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
