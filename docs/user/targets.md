# Targets

Unsupported features skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs |
|-----------------|---------------------|--------|--------------------------|-------|------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `CLAUDE.md`   | `.claude/settings.json` | `.mcp.json` |
| **codex**       | `.codex/agents/*.toml` | `.agents/skills/<name>/SKILL.md` | `AGENTS.md` (nested per-dir by globs) | - | - |
| **gemini**      | merged in `GEMINI.md` | listed in `GEMINI.md` | `GEMINI.md`     | -     | - |
| **cursor**      | as `.mdc` (alwaysApply: false) | as `.mdc` (`skill-<name>.mdc`) | `.cursor/rules/*.mdc` | - | `.cursor/mcp.json` |
| **copilot**     | merged in instructions | listed in instructions | `.github/copilot-instructions.md` | - | `.vscode/mcp.json` |
| **aider**       | merged in `CONVENTIONS.md` | listed in `CONVENTIONS.md` | `CONVENTIONS.md` | - | - |
| **cline**       | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     | - |
| **windsurf**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     | - |
| **continue**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | - |
| **amp**         | merged in `AGENT.md` | listed in `AGENT.md` | `AGENT.md` | - | - |
| **zed**         | merged in `.rules` | listed in `.rules` | `.rules` | - | - |
| **warp**        | merged in `WARP.md` | listed in `WARP.md` | `WARP.md` | - | - |
| **opencode**    | merged in `.opencode/AGENTS.md` | listed in `.opencode/AGENTS.md` | `.opencode/AGENTS.md` | - | - |

Skills emitted to non-Claude targets are reference material. Only Claude
Code has native skill execution. For all other targets, the agent or
human reads the skill file and follows its instructions.

Hooks are Claude-specific. They run as shell commands on lifecycle
events (e.g. PreToolUse, PostToolUse, SessionStart). No other supported
target has an equivalent concept, so hooks emit only for Claude.

MCP servers emit for Claude Code (`.mcp.json`), Cursor (`.cursor/mcp.json`),
and GitHub Copilot in VS Code (`.vscode/mcp.json`). Other targets either
have no project-scoped MCP file (Codex, Gemini) or no MCP support at all,
and skip with a warning.

## Per-target output

### Claude Code (`claude`)

```
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
└── settings.json
CLAUDE.md
```

Config keys: `outputs.claude.dir` (default `.claude`), `outputs.claude.rules-file` (default `CLAUDE.md`), `outputs.claude.mcp-file` (default `.mcp.json`).

### Codex (`codex`)

```
AGENTS.md
src/AGENTS.md                                # if any rule has globs: src/**
docs/api/AGENTS.md                           # if any rule has globs: docs/api/**
.codex/agents/<name>.toml                    # one TOML per agent
.agents/skills/<name>/SKILL.md               # one folder per skill
.agents/skills/<name>/agents/openai.yaml     # optional, when x-codex provides UI/policy/deps
```

Config keys: `outputs.codex.file` (default `AGENTS.md`), `outputs.codex.agents-dir` (default `.codex/agents`), `outputs.codex.skills-dir` (default `.agents/skills`).

Codex emits a hierarchy of `AGENTS.md` files. Rules with a `globs` frontmatter field that names a fixed directory prefix (e.g. `src/**`, `docs/api/**`) route into that subdirectory. Unscoped rules and all agents go to the root file.

Skills follow the [Codex skills layout](https://developers.openai.com/codex/skills): one folder per skill under `.agents/skills/<name>/` with a required `SKILL.md` (frontmatter `name` + `description`, plus the body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an additional `agents/openai.yaml` is written in the skill folder for UI customization and policy declarations. The root `AGENTS.md` lists each skill with a pointer to its `SKILL.md`.

### Gemini CLI (`gemini`)

```
GEMINI.md
```

Config key: `outputs.gemini.file` (default `GEMINI.md`).

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
```

Config keys: `outputs.cursor.rules-dir` (default `.cursor/rules`), `outputs.cursor.mcp-file` (default `.cursor/mcp.json`).

Rules emit with `alwaysApply: true`; agents as rules with `alwaysApply: false`. Override in spec frontmatter.

### GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md
```

Config keys: `outputs.copilot.file` (default `.github/copilot-instructions.md`), `outputs.copilot.mcp-file` (default `.vscode/mcp.json`).

The Copilot MCP file uses the VS Code schema: a top-level `servers` key with each entry carrying a `type` field (`stdio`, `http`, or `sse`).

### Aider (`aider`)

```
CONVENTIONS.md
```

Config key: `outputs.aider.file` (default `CONVENTIONS.md`).

Pair with `aider --read CONVENTIONS.md` or add to `.aider.conf.yml`.

### Cline (`cline`)

```
.clinerules/<name>.md
```

Config key: `outputs.cline.rules-dir` (default `.clinerules`).

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
```

Config key: `outputs.windsurf.rules-dir` (default `.windsurf/rules`).

### Continue (`continue`)

```
.continue/rules/<name>.md
```

Config key: `outputs.continue.rules-dir` (default `.continue/rules`).

### Amp (`amp`)

```
AGENT.md
```

Config key: `outputs.amp.file` (default `AGENT.md`).

### Zed (`zed`)

```
.rules
```

Config key: `outputs.zed.file` (default `.rules`).

### Warp (`warp`)

```
WARP.md
```

Config key: `outputs.warp.file` (default `WARP.md`).

### OpenCode (`opencode`)

```
.opencode/AGENTS.md
```

Config key: `outputs.opencode.file` (default `.opencode/AGENTS.md`). Routed under `.opencode/` to avoid clashing with Codex's repo-root `AGENTS.md` so both can be enabled together.

## Selecting targets

Persistent (config):

```yaml
targets:
  - claude
  - cursor
  - copilot
```

Per-run (CLI):

```bash
agnostic-ai sync -t claude,cursor,copilot
```

CLI flag overrides config. Unknown targets log a warning and skip.

## New targets

See [adding-adapters](../internal/adding-adapters.md). ~50 lines plus one registry entry.
