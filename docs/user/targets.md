# Targets

Each adapter emits in its tool's native format — separate files where the tool supports them, a merged document otherwise. Unsupported features (e.g. hooks for a non-hook-aware target) skip with a warning by default. Override via `on-unsupported` in [configuration](configuration.md).

## Capability matrix

| Target          | Agents              | Skills | Rules                    | Hooks | MCPs |
|-----------------|---------------------|--------|--------------------------|-------|------|
| **claude**      | `.claude/agents/`   | `.claude/skills/` | `.claude/rules/*.md`   | `.claude/settings.json` | `.mcp.json` |
| **codex**       | `.codex/agents/*.toml` | `.agents/skills/<name>/SKILL.md` | `AGENTS.md` (nested per-dir by globs) | `.codex/config.toml` (`[[hooks.<event>]]`) | `.codex/config.toml` (`[mcp_servers.<name>]`) |
| **gemini**      | `.gemini/commands/<name>.toml` | listed in `GEMINI.md` (or `.gemini/commands/skill-<name>.toml` w/ opt-in) | `GEMINI.md` (nested per-dir by scope/globs) | `.gemini/settings.json` (`hooks`) | `.gemini/settings.json` (`mcpServers`) |
| **cursor**      | as `.mdc` (alwaysApply: false) | as `.mdc` (`skill-<name>.mdc`) | `.cursor/rules/*.mdc` | - | `.cursor/mcp.json` |
| **copilot**     | `.github/instructions/agent-<name>.instructions.md` | `.github/instructions/skill-<name>.instructions.md` | `.github/instructions/<name>.instructions.md` + `.github/copilot-instructions.md` (always-on) | - | `.vscode/mcp.json` |
| **aider**       | merged in `CONVENTIONS.md` | listed in `CONVENTIONS.md` | `CONVENTIONS.md` (+ `.aider.conf.yml` w/ opt-in) | - | - |
| **cline**       | as `.md` rule (+ `.clinerules/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.clinerules/*.md`       | -     | - |
| **windsurf**    | as `.md` rule (+ `.windsurf/workflows/<name>.md` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.windsurf/rules/*.md`   | -     | - |
| **continue**    | as `.md` rule (+ `.continue/assistants/<name>.yaml` w/ opt-in) | as `.md` (`skill-<name>.md`) | `.continue/rules/*.md`   | -     | `.continue/mcpServers/*.yaml` |
| **amp**         | `.agents/commands/<name>.md` | listed in `AGENTS.md` (or `.agents/commands/skill-<name>.md` w/ opt-in) | `AGENTS.md` (nested per-dir by scope/globs) | - | `.amp/settings.json` (`amp.mcpServers`) |
| **zed**         | merged in `.rules` | listed in `.rules` | `.rules` | `.zed/tasks.json` w/ opt-in | `.zed/settings.json` (`context_servers`) |
| **warp**        | inlined in `AGENTS.md` (or `.warp/workflows/<name>.yaml` w/ opt-in) | listed in `AGENTS.md` | `AGENTS.md` (nested per-dir by scope/globs) | - | `.warp/.mcp.json` |
| **opencode**    | `.opencode/commands/<name>.md` | listed in `.opencode/AGENTS.md` (or `.opencode/commands/skill-<name>.md` w/ opt-in) | `.opencode/AGENTS.md` | - | `opencode.json` (`mcp`) |

Skills emitted to non-Claude targets are reference material. Only Claude
Code has native skill execution. For all other targets, the agent or
human reads the skill file and follows its instructions.

Hooks run as shell commands on lifecycle events (e.g. `PreToolUse`,
`PostToolUse`, `SessionStart`). Emitted natively by Claude Code, Codex,
and Gemini in each tool's own schema. Zed picks them up too via the
opt-in `outputs.zed.tasks-file`, but as on-demand tasks rather than
event-triggered hooks. Other targets have no equivalent concept and
skip with a warning.

MCP servers propagate to every target that has a project-scoped MCP file
(10 of 13 — see the matrix above). Aider, Cline, and Windsurf have no
project-scoped MCP surface and skip with a warning.

## Per-target output

### Claude Code (`claude`)

```
.claude/
├── agents/<name>.md
├── skills/<name>/SKILL.md
├── rules/<name>.md
└── settings.json
.mcp.json
```

Rules emit one file per spec under `.claude/rules/`. A hand-authored `CLAUDE.md` is never overwritten. Reference per-rule files from `CLAUDE.md` with `@.claude/rules/<name>.md` imports if you want Claude Code to load them.

Config keys: `outputs.claude.dir` (default `.claude`), `outputs.claude.rules-dir` (default `.claude/rules`), `outputs.claude.rules-file` (unset; setting it switches back to the legacy concatenated single-file layout, typically `CLAUDE.md`), `outputs.claude.mcp-file` (default `.mcp.json`).

### Codex (`codex`)

```
AGENTS.md
src/AGENTS.md                                # if any rule has globs: src/**
docs/api/AGENTS.md                           # if any rule has globs: docs/api/**
.codex/agents/<name>.toml                    # one TOML per agent
.agents/skills/<name>/SKILL.md               # one folder per skill
.agents/skills/<name>/agents/openai.yaml     # optional, when x-codex provides UI/policy/deps
.codex/config.toml                           # when hook and/or MCP entries exist
```

Config keys: `outputs.codex.file` (default `AGENTS.md`), `outputs.codex.agents-dir` (default `.codex/agents`), `outputs.codex.skills-dir` (default `.agents/skills`), `outputs.codex.mcp-file` (default `.codex/config.toml` — also holds hooks).

Codex emits a hierarchy of `AGENTS.md` files. Rules with a `globs` frontmatter field that names a fixed directory prefix (e.g. `src/**`, `docs/api/**`) route into that subdirectory. Unscoped rules and all agents go to the root file. The `## Agents` section lists each agent with a pointer to its TOML rather than inlining the body, so each agent lives in exactly one place.

Skills follow the [Codex skills layout](https://developers.openai.com/codex/skills): one folder per skill under `.agents/skills/<name>/` with a required `SKILL.md` (frontmatter `name` + `description`, plus the body). When the spec carries `x-codex.interface`, `x-codex.policy`, or `x-codex.dependencies`, an additional `agents/openai.yaml` is written in the skill folder for UI customization and policy declarations. The root `AGENTS.md` lists each skill with a pointer to its `SKILL.md`.

Hooks and MCP servers both land in `.codex/config.toml`. Hook specs route by their `event` frontmatter (e.g. `SessionStart`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, `PostCompact`) into `[[hooks.<event>]]` array-of-tables entries with `matcher` and `command`. MCP servers emit as `[mcp_servers.<name>]` tables — stdio uses `command`/`args`/`env`; HTTP/SSE uses `url`/`bearer_token_env_var`/`http_headers`. The project-tier config.toml is agnostic-ai-managed (overwritten on each sync); add unmanaged Codex config to `~/.codex/config.toml` user-global instead.

### Gemini CLI (`gemini`)

```
GEMINI.md                              # root rules + agent/skill references
src/GEMINI.md                          # rules scoped to src/ (source layout or globs)
docs/api/GEMINI.md                     # rules scoped to docs/api/
.gemini/commands/<name>.toml           # one per agent
.gemini/commands/skill-<name>.toml     # one per skill, only when emit-skills-as-commands: true
.gemini/settings.json                  # when MCP and/or hook entries exist (merged with any existing user config)
```

Config keys: `outputs.gemini.file` (default `GEMINI.md`), `outputs.gemini.commands-dir` (default `.gemini/commands`), `outputs.gemini.mcp-file` (default `.gemini/settings.json` — also holds hooks), `outputs.gemini.emit-skills-as-commands` (default `false`).

Gemini CLI loads `GEMINI.md` hierarchically: each subdirectory adds context for its subtree. This adapter routes rules by their source-layout scope (e.g. `rules/backend/auth.md` → `backend/GEMINI.md`) or by a leading literal prefix of their `globs` frontmatter (e.g. `docs/api/**` → `docs/api/GEMINI.md`). Source layout wins when both are present.

Agents emit as TOML slash commands under `.gemini/commands/` per [Gemini CLI custom commands](https://geminicli.com/docs/cli/custom-commands/). The root `GEMINI.md` lists each agent with a reference pointer rather than inlining the body, so each agent lives in exactly one place. Skills default to reference-only listings in the root `GEMINI.md`; flip `outputs.gemini.emit-skills-as-commands: true` to also emit one `.gemini/commands/skill-<name>.toml` per skill.

When MCP and/or hook entries are present, the adapter writes (or updates) `.gemini/settings.json` with the `mcpServers` map and the `hooks` map. Gemini uses `httpUrl` (not `url`) for HTTP MCP servers — the adapter handles the rename automatically. Hook specs route by their `event` frontmatter (e.g. `BeforeTool`, `AfterTool`, `SessionStart`); each event entry is `{matcher, command}`. Pre-existing user-managed keys in `.gemini/settings.json` survive across syncs.

### Cursor (`cursor`)

```
.cursor/rules/<name>.mdc
.cursor/commands/<name>.md           # one per agent, only when commands-dir is set
```

Config keys: `outputs.cursor.rules-dir` (default `.cursor/rules`), `outputs.cursor.commands-dir` (default empty — opt-in), `outputs.cursor.mcp-file` (default `.cursor/mcp.json`).

Rules emit with `alwaysApply: true`; agents as rules with `alwaysApply: false`. Override in spec frontmatter.

When `outputs.cursor.commands-dir` is set, each agent additionally emits as a [Cursor Custom Command](https://docs.cursor.com/agent/custom-commands): a Markdown file with optional `description` and `model` frontmatter. The rule-form emission (`.cursor/rules/<name>.mdc`) still happens, so existing setups keep working.

### GitHub Copilot (`copilot`)

> When `outputs.copilot.chatmodes-dir` is set, each agent additionally emits as a [Copilot Custom Chat Mode](https://docs.github.com/en/copilot/customizing-copilot/adding-custom-instructions-for-github-copilot#about-custom-chat-modes) at `<dir>/<name>.chatmode.md` with `description`/`model`/`tools` frontmatter. The catch-all `agent-<name>.instructions.md` emission keeps working.


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
.aider.conf.yml          # only when conf-file is set
```

Config keys: `outputs.aider.file` (default `CONVENTIONS.md`), `outputs.aider.conf-file` (default empty — opt-in), `outputs.aider.model`, `outputs.aider.weak-model`.

By default the adapter only emits the conventions document and you wire it in yourself via `aider --read CONVENTIONS.md`. Set `outputs.aider.conf-file: .aider.conf.yml` to have `sync` also merge a `read:` entry into Aider's [project config](https://aider.chat/docs/config/aider_conf.html) so the file auto-loads. `model` and `weak-model` propagate into the same file when set. Pre-existing keys in the conf file are preserved; the `read:` list de-duplicates.

### Cline (`cline`)

```
.clinerules/<name>.md
.clinerules/workflows/<name>.md      # one per agent, only when workflows-dir is set
```

Config keys: `outputs.cline.rules-dir` (default `.clinerules`), `outputs.cline.workflows-dir` (default empty — opt-in).

When `outputs.cline.workflows-dir` is set, each agent additionally emits as a [Cline Workflow](https://docs.cline.bot/features/workflows): a Markdown file invokable from chat as `/<name>.md`. The italic description prefixes the body when present. The rule-form emission (`.clinerules/agent-<name>.md`) still happens, so existing setups keep working.

### Windsurf (`windsurf`)

```
.windsurf/rules/<name>.md
.windsurf/workflows/<name>.md        # one per agent, only when workflows-dir is set
```

Config keys: `outputs.windsurf.rules-dir` (default `.windsurf/rules`), `outputs.windsurf.workflows-dir` (default empty — opt-in).

When `outputs.windsurf.workflows-dir` is set, each agent additionally emits as a [Windsurf Workflow](https://docs.windsurf.com/windsurf/cascade/workflows): a Markdown file with `description` frontmatter, invokable in Cascade as `/<name>`. The rule-form emission (`.windsurf/rules/agent-<name>.md`) still happens, so existing setups keep working.

### Continue (`continue`)

```
.continue/rules/<name>.md
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

Config keys: `outputs.continue.rules-dir` (default `.continue/rules`), `outputs.continue.mcp-dir` (default `.continue/mcpServers`), `outputs.continue.assistants-dir` (default empty — opt-in).

Continue picks up each YAML under `.continue/mcpServers/` as a single MCP server config. Stdio servers emit `command`/`args`/`env`; HTTP / SSE / streamable-http variants emit `type`/`url`/`headers`.

When `outputs.continue.assistants-dir` is set, each agent additionally emits as a [Continue local Assistant](https://docs.continue.dev/hub/assistants/intro) YAML at `<dir>/<name>.yaml` (`schema: v1`, `version: 0.0.1` by default). The agent body wraps as a single named prompt so Continue surfaces it in the assistant picker. Models and rules are intentionally omitted so the user's defaults apply. The rule-form emission (`.continue/rules/agent-<name>.md`) still happens, so existing setups keep working.

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
.zed/settings.json                     # when MCP entries exist (merged with any existing user config)
.zed/tasks.json                        # one task per hook, only when tasks-file is set
```

Config keys: `outputs.zed.file` (default `.rules`), `outputs.zed.mcp-file` (default `.zed/settings.json`), `outputs.zed.tasks-file` (default empty — opt-in).

When MCP entries are present, the adapter writes (or updates) `.zed/settings.json` with the `context_servers` map (Zed's key — not `mcpServers`). Each server emits with Zed's nested `command: {path, args, env}` shape plus an empty `settings: {}`. User-managed keys (theme, buffer_font_size, etc.) are preserved across syncs.

Zed only supports stdio transport natively. HTTP / SSE MCP entries auto-bridge through `npx mcp-remote <url>` so they still work — a one-line stderr warning fires when this fallback is taken.

When `outputs.zed.tasks-file` is set, every hook spec emits as a [Zed Task](https://zed.dev/docs/tasks) into that JSON file. Each task uses `sh -c "<hook command>"` so arbitrary one-liners work; the hook's name is the label (with the description appended after an em-dash when present). Zed has no lifecycle-hook surface, so these tasks run on demand from the command palette rather than firing on `PreToolUse` / `PostToolUse` events.

### Warp (`warp`)

```
AGENTS.md                              # root rules + agent bodies (or references) + skill references
src/AGENTS.md                          # rules scoped to src/ (source layout or globs)
.warp/workflows/<name>.yaml            # one per agent, only when workflows-dir is set
.warp/.mcp.json                        # when MCP entries exist
```

Config keys: `outputs.warp.file` (default `AGENTS.md`), `outputs.warp.workflows-dir` (default empty — opt-in), `outputs.warp.mcp-file` (default `.warp/.mcp.json`).

**Breaking change in this release:** the adapter previously wrote `WARP.md`. Warp's current Rules docs specify `AGENTS.md` per the open AGENTS.md standard; the legacy name is now retired. On first sync after upgrading, any agnostic-generated `WARP.md` at the configured root is renamed to `WARP.md.bak`. Run `agnostic-ai revert` to roll back, or `git rm WARP.md.bak` once you have verified the new layout. A user-authored `WARP.md` (no `Generated by agnostic-ai` marker) is left untouched.

By default Warp has no separate slash-command surface, so agents inline their bodies into the root `AGENTS.md`. When `outputs.warp.workflows-dir` is set, each agent additionally emits as a [Warp Workflow](https://docs.warp.dev/features/warp-drive/workflows) YAML at `<dir>/<name>.yaml` (`name`/`command`/`description`/`tags`), and the `## Agents` section in `AGENTS.md` downgrades to reference pointers so each agent body lives in exactly one place. The workflow `command:` is the agent body verbatim — tailor it to a Warp-friendly shell snippet from there. Skills always list as reference pointers to their source spec (no execution model in Warp).

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
