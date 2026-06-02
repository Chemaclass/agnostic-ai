# Roadmap

High-level directions. Concrete work in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration (shipped)

Three layers, low to high precedence:

- **user-global** (`$AGNOSTIC_AI_HOME` or `~/.agnostic-ai/`): cross-project source of truth.
- **project** (`agnostic-ai.yaml` `sources`): checked-in specs.
- **project-user** (`<project>/.agnostic-ai.local/`, gitignored): per-developer overrides.

Higher layer wins on `(Kind, Name)` collision. Merge in `spec.LoadLayered`. See [configuration.md](../user/configuration.md#layered-specs).

## Shipped

- Watch mode (`sync --watch`, fsnotify, `--watch-poll` fallback).
- Onboarding (`init --demo`, `init --preset <name>`).
- Interactive target selection (`init` TTY picker, `--all` to skip).
- Codex subagents + skills (`.codex/agents/<name>.toml`, `.codex/skills/<name>/SKILL.md`).
- Top-level `import <source>` (multi-source: `import claude codex`).
- `doctor --fix [--backup]`.
- MCP for 10/14 targets. Aider/Cline/Windsurf lack project-scoped MCP.
- Hooks beyond Claude: Codex `.codex/config.toml`, Gemini `.gemini/settings.json`.

## Open

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)).
- Richer importers per source.
