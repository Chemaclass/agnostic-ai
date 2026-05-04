# Roadmap

Tracked here at a high level. Concrete work happens in [issues](https://github.com/Chemaclass/agnostic-ai/issues).

## Layered configuration

Today: a single project layer (specs in the repo).

Planned:

- **User-global layer** (`~/.agnostic-ai/`): one source of truth across every project on the machine. Useful for personal rules a developer wants in every repo (e.g. preferred commit style).
- **Project-user layer** (`.agnostic-ai/`, gitignored): per-developer overrides on top of the project layer. Useful for personal tweaks without polluting the team config.

**Precedence:** project-user > project > user-global.

Each layer can add new specs by name or override an existing one. The transpiler resolves the merged set before any adapter runs, so adapters stay layer-agnostic.

## Other directions

Filed as issues:

- More adapters (open a PR; see [adding-adapters.md](adding-adapters.md)). Recently shipped: Amp, Zed, Warp, OpenCode.
- Richer `--from <source>` importers. Shipped: `claude`, `codex`, `cursor`. Wanted: `cline`, `windsurf`, `continue`.
- `doctor` improvements (drift fixes, not only detection).
- More MCP-aware targets. Shipped: Claude (`.mcp.json`), Cursor (`.cursor/mcp.json`), Copilot (`.vscode/mcp.json`).
