# Roadmap

Tracked here at a high level. Concrete work happens in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration

Shipped. Three layers, low- to high-precedence:

- **user-global** (`$AGNOSTIC_AI_HOME` or `~/.agnostic-ai/`): one source of truth across every project on the machine.
- **project** (`agnostic.config.yaml` `sources`): the repo's checked-in specs.
- **project-user** (`<project>/.agnostic-ai.local/`, gitignored): per-developer overrides on top of project.

Higher layer wins on `(Kind, Name)` collision. New names append. Adapters stay layer-agnostic; merge happens in `spec.LoadLayered`. Optional layers load only when their root exists. See [docs/user/configuration.md](../user/configuration.md#layered-specs).

## Other directions

Filed as issues:

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)). Recently shipped: Amp, Zed, Warp, OpenCode.
- Richer `--from <source>` importers. Shipped: `claude`, `codex`, `cursor`, `cline`, `windsurf`, `continue`.
- ~~`doctor` improvements (drift fixes, not only detection).~~ Shipped: `doctor --fix [--backup]`.
- More MCP-aware targets. Shipped: Claude (`.mcp.json`), Cursor (`.cursor/mcp.json`), Copilot (`.vscode/mcp.json`).
