# Roadmap

Tracked here at a high level. Concrete work happens in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration

Shipped. Three layers, low- to high-precedence:

- **user-global** (`$AGNOSTIC_AI_HOME` or `~/.agnostic-ai/`): one source of truth across every project on the machine.
- **project** (`agnostic.config.yaml` `sources`): the repo's checked-in specs.
- **project-user** (`<project>/.agnostic-ai.local/`, gitignored): per-developer overrides on top of project.

Higher layer wins on `(Kind, Name)` collision. New names append. Adapters stay layer-agnostic; merge happens in `spec.LoadLayered`. Optional layers load only when their root exists. See [docs/user/configuration.md](../user/configuration.md#layered-specs).

## Shipped in v0.4

- ~~Watch mode for tight authoring loops.~~ Shipped: `sync --watch` (200 ms poll, Ctrl+C exits).
- ~~Agent-managed auto-sync.~~ Shipped: `sync --auto-sync=yes|no` plus a first-run TTY prompt; persists `autoSync` in `agnostic.config.yaml`.
- ~~Onboarding examples in fresh projects.~~ Shipped: `init --demo`.
- ~~Interactive target selection.~~ Shipped: `init -i` / `--interactive`.
- ~~Codex subagents and skills.~~ Shipped: `.codex/agents/<name>.toml` and `.agents/skills/<name>/SKILL.md`.
- ~~Promote import to a top-level command.~~ Shipped: `import <source>` (replaces `init --from`).

## Other directions

Filed as issues:

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)). Recently shipped: Amp, Zed, Warp, OpenCode.
- Richer `import <source>` importers. Shipped: `claude`, `codex`, `cursor`, `cline`, `windsurf`, `continue`.
- ~~`doctor` improvements (drift fixes, not only detection).~~ Shipped: `doctor --fix [--backup]`.
- More MCP-aware targets. Shipped: Claude (`.mcp.json`), Cursor (`.cursor/mcp.json`), Copilot (`.vscode/mcp.json`).
