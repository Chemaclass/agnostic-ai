# Roadmap

[Contributor docs](README.md)

High-level directions. Concrete work in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration (shipped)

Project layers, low to high precedence:

- **packs**: shared defaults installed into the project.
- **project** (`agnostic-ai.yaml` `sources`): checked-in specs.
- **project-user** (`<project>/.agnostic-ai.local/`, gitignored): per-developer overrides.

Native global configuration (`$AGNOSTIC_AI_HOME` or `~/.agnostic-ai/`) is a separate source for `sync --global` and does not participate in project layering.

Higher layer wins on `(Kind, Name)` collision. Merge in `spec.LoadLayered`. See [configuration.md](../user/configuration.md#layered-specs).

## Shipped

- Watch mode (`sync --watch`, fsnotify, `--watch-poll` fallback).
- Onboarding (`init --demo`, `init --preset <name>`).
- Interactive target selection (`init` TTY picker, `--all` to skip).
- Codex subagents + skills (`.codex/agents/<name>.toml`, `.agents/skills/<name>/SKILL.md`).
- Top-level `import <source>` (multi-source: `import claude codex`).
- `doctor --fix [--backup]`.
- MCP and hook support across tools: see the maintained [capability matrix](../user/targets.md#capability-matrix).

## Open

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)).
- Richer importers per source.
