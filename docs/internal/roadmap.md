# Roadmap

[Contributor docs](README.md)

Broad directions. Specific work is tracked in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration (shipped)

Project layers, from lowest to highest priority:

- **packs**: shared defaults installed into the project.
- **project** (`agnostic-ai.yaml` `sources`): checked-in specs.
- **project-user** (`<project>/.agnostic-ai/local/`, gitignored): per-developer overrides.

Native global configuration (`$AGNOSTIC_AI_HOME` or `~/.agnostic-ai/`) is a separate source for `sync --global` and does not participate in project layering.

When two entries share `(Kind, Name)`, the higher-priority layer wins. `spec.LoadLayered` combines the layers. See [configuration.md](../site/content/docs/configuration.md#layered-specs).

## Shipped

- Watch mode (`sync --watch`, fsnotify, `--watch-poll` fallback).
- Onboarding (`init --demo`, `init --preset <name>`).
- Interactive target selection (`init` terminal picker, `--all` to skip).
- Codex subagents + skills (`.codex/agents/<name>.toml`, `.agents/skills/<name>/SKILL.md`).
- Import with `import <source>` (multiple sources: `import claude codex`).
- `doctor --fix [--backup]`.
- MCP and hook support across tools: see the maintained [capability matrix](../site/content/docs/targets/_index.md#capability-matrix).

## Open

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)).
- Import more features from each supported source.
