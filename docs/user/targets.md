# Targets

Unsupported features skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs |
|-----------------|---------------------|--------|--------------------------|-------|------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `CLAUDE.md`   | `.claude/settings.json` | `.mcp.json` |
| **codex**       | `.codex/agents/*.toml` | `.agents/skills/<name>/SKILL.md` | `AGENTS.md` (nested per-dir by globs) | - | - |
| **gemini**      | `.gemini/commands/<name>.toml` | listed in `GEMINI.md` (or `.gemini/commands/skill-<name>.toml` w/ opt-in) | `GEMINI.md` (nested per-dir by scope/globs) | - | - |
| **cursor**      | as `.mdc` (alwaysApply: false) | as `.mdc` (`skill-<name>.mdc`) | `.cursor/rules/*.mdc` | - | `.cursor/mcp.json` |
| **copilot**     | `.github/instructions/agent-<name>.instructions.md` | `.github/instructions/skill-<name>.instructions.md` | `.github/instructions/<name>.instructions.md` + `.github/copilot-instructions.md` (always-on) | - | `.vscode/mcp.json` |
| **aider**       | merged in `CONVENTIONS.md` | listed in `CONVENTIONS.md` | `CONVENTIONS.md` | - | - |
| **cline**       | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     | - |
| **windsurf**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     | - |
| **continue**    | as `.md` rule       | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | `.continue/mcpServers/*.yaml` |
| **amp**         | `.agents/commands/<name>.md` | listed in `AGENTS.md` (or `.agents/commands/skill-<name>.md` w/ opt-in) | `AGENTS.md` (nested per-dir by scope/globs) | - | `.amp/settings.json` (`amp.mcpServers`) |
| **zed**         | merged in `.rules` | listed in `.rules` | `.rules` | - | - |
| **warp**        | inlined in `AGENTS.md` | listed in `AGENTS.md` | `AGENTS.md` (nested per-dir by scope/globs) | - | `.warp/.mcp.json` |
| **opencode**    | `.opencode/commands/<name>.md` | listed in `.opencode/AGENTS.md` (or `.opencode/commands/skill-<name>.md` w/ opt-in) | `.opencode/AGENTS.md` | - | `opencode.json` (`mcp`) |

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

Codex emits a hierarchy of `AGENTS.md` files. Rules with a `globs` frontmatter field that names a fixed directory prefix (e.g. `src/**`, `docs/api/**`) route into that subdirectory. Unscoped rules and all agents go to the root file. The `## Agents` section lists each agent with a pointer to its TOML rather than inlining the body, so each agent lives in exactly one place.

Skills follow the [Codex skills layout](https://developers.openai.com/codex/skills): one folder per skill under `.agents/skills/<name>/` with a required `SKILL.md` (frontmatter `name` + `description`, plus the body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an additional `agents/openai.yaml` is written in the skill folder for UI customization and policy declarations. The root `AGENTS.md` lists each skill with a pointer to its `SKILL.md`.

### Gemini CLI (`gemini`)

```
GEMINI.md                              # root rules + agent/skill references
src/GEMINI.md                          # rules scoped to src/ (source layout or globs)
docs/api/GEMINI.md                     # rules scoped to docs/api/
.gemini/commands/<name>.toml           # one per agent
.gemini/commands/skill-<name>.toml     # one per skill, only when emit-skills-as-commands: true
```

Config keys: `outputs.gemini.file` (default `GEMINI.md`), `outputs.gemini.commands-dir` (default `.gemini/commands`), `outputs.gemini.emit-skills-as-commands` (default `false`).

Gemini CLI loads `GEMINI.md` hierarchically: each subdirectory adds context for its subtree. This adapter routes rules by their source-layout scope (e.g. `rules/backend/auth.md` → `backend/GEMINI.md`) or by a leading literal prefix of their `globs` frontmatter (e.g. `docs/api/**` → `docs/api/GEMINI.md`). Source layout wins when both are present.

Agents emit as TOML slash commands under `.gemini/commands/` per [Gemini CLI custom commands](https://geminicli.com/docs/cli/custom-commands/). The root `GEMINI.md` lists each agent with a reference pointer rather than inlining the body, so each agent lives in exactly one place. Skills default to reference-only listings in the root `GEMINI.md`; flip `outputs.gemini.emit-skills-as-commands: true` to also emit one `.gemini/commands/skill-<name>.toml` per skill.

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
```

Config keys: `outputs.cursor.rules-dir` (default `.cursor/rules`), `outputs.cursor.mcp-file` (default `.cursor/mcp.json`).

Rules emit with `alwaysApply: true`; agents as rules with `alwaysApply: false`. Override in spec frontmatter.

### GitHub Copilot (`copilot`)

```
.github/copilot-instructions.md                            # always-on rules
.github/instructions/<name>.instructions.md                # scoped rule per file
.github/instructions/agent-<name>.instructions.md          # one per agent
.github/instructions/skill-<name>.instructions.md          # one per skill
.vscode/mcp.json                                           # when MCP entries exist
```

Config keys: `outputs.copilot.file` (default `.github/copilot-instructions.md`), `outputs.copilot.instructions-dir` (default `.github/instructions`), `outputs.copilot.mcp-file` (default `.vscode/mcp.json`).

Copilot natively supports path-scoped instructions via `applyTo:` frontmatter. Rules with a `globs` field (or a source-layout scope like `rules/backend/auth.md`) emit as a separate `.instructions.md` file with `applyTo` derived from the globs (explicit `globs` wins, else `<scope>/**`). Rules with `alwaysApply: true` or no scope merge into the always-on `.github/copilot-instructions.md`. Agents and skills always emit as catch-all (`applyTo: "**"`) so they remain discoverable across the repo.

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
.continue/mcpServers/<name>.yaml       # one per MCP entry
```

Config keys: `outputs.continue.rules-dir` (default `.continue/rules`), `outputs.continue.mcp-dir` (default `.continue/mcpServers`).

Continue picks up each YAML under `.continue/mcpServers/` as a single MCP server config. Stdio servers emit `command`/`args`/`env`; HTTP / SSE / streamable-http variants emit `type`/`url`/`headers`.

### Amp (`amp`)

```
AGENTS.md                              # root rules + agent/skill references
src/AGENTS.md                          # rules scoped to src/ (source layout or globs)
.agents/commands/<name>.md             # one per agent
.agents/commands/skill-<name>.md       # one per skill, only when emit-skills-as-commands: true
.amp/settings.json                     # when MCP entries exist (merged with any existing user config)
```

Config keys: `outputs.amp.file` (default `AGENTS.md`), `outputs.amp.commands-dir` (default `.agents/commands`), `outputs.amp.mcp-file` (default `.amp/settings.json`), `outputs.amp.emit-skills-as-commands` (default `false`).

When MCP entries are present, the adapter writes (or updates) `.amp/settings.json` with the `amp.mcpServers` map (note: a single dotted key, not a nested object). Stdio servers emit `command`/`args`/`env`; HTTP/SSE emit `url`/`headers`. Pre-existing non-managed keys in `.amp/settings.json` (theme, editor settings, etc.) are preserved across syncs; only `amp.mcpServers` is overwritten. Workspace MCPs require explicit approval inside Amp the first time the project is opened — Amp's safety model is intentional.

**Breaking change in this release:** the adapter previously wrote `AGENT.md` (singular). Amp's owner's manual specifies `AGENTS.md` (plural); the misnamed file is now retired. On first sync after upgrading, any agnostic-generated `AGENT.md` at the configured root is renamed to `AGENT.md.bak`. Run `agnostic-ai revert` to roll back, or `git rm AGENT.md.bak` once you have verified the new layout. A user-authored `AGENT.md` (no `Generated by agnostic-ai` marker) is left untouched.

Agents emit as slash commands under `.agents/commands/` per the Amp owner's manual. The root `AGENTS.md` references each agent via a pointer section rather than inlining the body. Skills default to reference-only listings; flip `outputs.amp.emit-skills-as-commands: true` to also emit `.agents/commands/skill-<name>.md` per skill.

Amp's hierarchical `AGENTS.md` shares the same open standard as Codex and Warp. When two or more of those targets are enabled, they may write the same root or scoped `AGENTS.md` — last writer wins.

### Zed (`zed`)

```
.rules
```

Config key: `outputs.zed.file` (default `.rules`).

### Warp (`warp`)

```
AGENTS.md                              # root rules + agent bodies + skill references
src/AGENTS.md                          # rules scoped to src/ (source layout or globs)
.warp/.mcp.json                        # when MCP entries exist
```

Config keys: `outputs.warp.file` (default `AGENTS.md`), `outputs.warp.mcp-file` (default `.warp/.mcp.json`).

**Breaking change in this release:** the adapter previously wrote `WARP.md`. Warp's current Rules docs specify `AGENTS.md` per the open AGENTS.md standard; the legacy name is now retired. On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak`. Run `agnostic-ai revert` to roll back, or `git rm WARP.md.bak` once you have verified the new layout. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched.

Warp has no separate slash-command surface, so agents inline their bodies into the root `AGENTS.md`. Skills list as reference pointers to their source spec (no execution model in Warp).

Warp's `AGENTS.md` shares the same open standard as Codex and Amp. When two or more of those targets are enabled, they may write the same root or scoped `AGENTS.md` — last writer wins.

### OpenCode (`opencode`)

```
.opencode/AGENTS.md                       # rules + agent/skill references
.opencode/commands/<name>.md              # one per agent
.opencode/commands/skill-<name>.md        # one per skill, only when emit-skills-as-commands: true
opencode.json                             # when MCP entries exist (merged with any existing user config)
```

Config keys: `outputs.opencode.file` (default `.opencode/AGENTS.md`), `outputs.opencode.commands-dir` (default `.opencode/commands`), `outputs.opencode.mcp-file` (default `opencode.json`), `outputs.opencode.emit-skills-as-commands` (default `false`).

When MCP entries are present, the adapter writes (or updates) `opencode.json` at the project root with a `$schema` link and the `mcp` map. Stdio servers map to `{type: "local", command: [...]}`; HTTP/SSE/remote servers map to `{type: "remote", url, headers}`. Any pre-existing non-managed keys (`theme`, `model`, etc.) are preserved across syncs; only `$schema` and `mcp` are overwritten. Note: drift checks (`sync --check`) read in capture mode and skip the read step, so unrelated user keys may be reported as drift until the next non-check sync — this is by design.

Routed under `.opencode/` rather than the repo root to avoid clashing with Codex's `AGENTS.md`, so both can be enabled together. Each command file carries frontmatter filtered to the OpenCode-supported keys (`description`, `agent`, `model`, `subtask`) — pass `agent`, `model`, or `subtask` through the `x-opencode` namespace in your spec frontmatter to route them into the command file without polluting other targets.

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
