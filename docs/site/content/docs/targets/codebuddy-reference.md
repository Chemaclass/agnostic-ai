+++
title = "CodeBuddy Code native reference"
description = "Import coverage, project MCP output, native fields and limits, and config keys for codebuddy."
weight = 128

[extra]
nav_title = "CodeBuddy Code reference"
group = "Reference"
+++

# CodeBuddy Code native reference

Start with the [CodeBuddy Code setup](@/docs/targets/codebuddy.md).

## Import an existing setup

```bash
agnostic-ai import codebuddy
agnostic-ai sync --plan
agnostic-ai sync
agnostic-ai sync --check
agnostic-ai lint
```

Import reads root and scoped instructions, modular rules, agents, skill folders and resources, commands, settings, command and prompt hooks, and root MCP files.

- Native fields with no portable equivalent stay under `x-codebuddy`.
- Top-level environment and credential settings stay in the native file.
- Import rejects nested credentials and literal credentials in inline agent MCP servers. The error names the field, not the value. Use native environment references in supported MCP fields.
- Native MCP import reads `.mcp.json` before deprecated `mcp.json`.
- Configure the [MCP opt-in](#enable-project-mcp-output) before syncing imported project servers.

Instruction frontmatter:

- Keeps native `enabled`, `alwaysApply`, and `paths` values.
- `enabled: false`, and `alwaysApply: false` without paths, stay inactive.
- The source-only hint `x-codebuddy.instruction-file` (`CODEBUDDY.md` or `.codebuddy/CODEBUDDY.md`), with a validated `scope`, restores the original location. It is left out of native output.
- If both root alternatives exist, pick one with the [entry-point file configuration](@/docs/configuration.md#entry-point-files) before import.

Sync reports a conflict before writing when a controlled root file shares its instruction policy with effective shared or local instructions, root rules, or the target overview. Scoped native files also reject mixed activation policies for one path.

## Enable project MCP output

```yaml
outputs:
  codebuddy:
    root-mcp-file: .mcp.json
```

- Sync writes an explicit `type`: `stdio`, `http`, or `sse`.
- Import accepts native JSONC and infers an omitted or empty type from `command` (stdio) or `url` (http).
- Stdio servers keep `command`, `args`, `env`, and `cwd`. Remote servers keep `url` and `headers`. Both keep `description`.
- Native `cwd` strings stay unchanged. CodeBuddy expands uppercase `${NAME}` and `${NAME:-default}` there. Sync does not expand environment references.
- `x-codebuddy.defer_loading` and `x-codebuddy.tools` keep native loading controls.
- Sync rejects disabled servers and remote-server `cwd`.
- Sync never approves project servers. CodeBuddy asks the user before the first connection.

At user scope, sync and import use the first existing file:

1. `~/.codebuddy/.mcp.json`
2. deprecated `~/.codebuddy/mcp.json`
3. legacy `~/.codebuddy.json`

New output uses the preferred file. Comments, account state, the global `projects` map, and unmanaged servers stay intact.

## Fields and limits

### Agents

- Names use lowercase letters and hyphens.
- Sync rejects scoped shell and file tool patterns. Keep those policies in permission rules.
- When an agent needs a policy CodeBuddy cannot represent, use per-spec target filters. `x-codebuddy.tools` gives an explicit native exposure list.
- Accepted fields: `name`, `description`, `tools`, `disallowedTools`, `model`, `effort`, `skills`, `mcpServers`, `permissionMode`, `maxTurns`, `background`, `initialPrompt`, `memory`, `hooks`, `disable-model-invocation` (keeps a manual-only agent out of model selection).
- Native additions go under `x-codebuddy`.
- Portable tool names map only to verified native tools. Sync fails when a restriction has no mapping, so access never widens.
- Native agent memory adds `Read`, `Write`, and `Edit` even to a restrictive tools list, including `x-codebuddy.tools: []`. Sync reports this through `on-unsupported`; `error` rejects it before writing.

Sub-agent MCP tools (CodeBuddy 2.181.0 or later, [docs](https://www.codebuddy.ai/docs/cli/sub-agents)):

- `mcp__docs` or `mcp__docs__*` exposes one named server. `mcp__docs__search` exposes one tool.
- Portable `can: [mcp:docs]` writes `mcp__docs`. These forms exclude other servers.
- The global wildcard `mcp__*` is unsupported in an agent allow list.
- Do not use `--agent` to isolate a main session's MCP tools. CodeBuddy 2.181.0 still exposed other configured servers.

Empty tool lists:

- A portable empty `tools` or `can` list fails sync, because CodeBuddy before 2.179.0 gives such an agent every tool.
- For no tools, set `x-codebuddy.tools: []`. Sync writes `tools: []` and notes that only CodeBuddy [2.179.0](https://www.codebuddy.ai/docs/cli/release-notes/v2.179.0) or later reads it as no tools.
- CodeBuddy still adds its messaging tools and the plan-exit tool to such an agent.
- Import keeps a native `tools: []` as `x-codebuddy.tools: []`.

### Skills and commands

- Skills keep native scoped allowed-tool strings such as `Bash(git:*)` and `Edit(src/**/*.ts)`, invocation controls, and native `context`, `agent`, `model`, and `hooks`.
- CodeBuddy applies agent, model, and hook overrides only with `context: fork`.
- Bundled files are copied unchanged.
- Commands keep description, argument hint, model, allowed tools, invocation control, and body placeholders.
- Global commands are not managed by agnostic-ai, although CodeBuddy has a user command directory.

### MCP permission rules

- Accepted: `mcp__server`, `mcp__server__*`, and exact `mcp__server__tool`.
- The all-server `mcp__*` form works in `ask` and `deny` only.
- Partial-segment wildcards are rejected.
- These follow the [native permission grammar](https://www.codebuddy.ai/docs/cli/permissions#mcp-tools).

### Settings

- `model` maps to `model`. `effort` maps to `reasoningEffort` and accepts `minimal`, `low`, `medium`, `high`, `xhigh`, `max`.
- Permission lists map to `permissions.allow/ask/deny`. `permissions.default-mode` maps to `permissions.defaultMode`.
- Session retention maps to `cleanupPeriodDays` at user scope.
- Voice has no documented native key.
- Project `autoMode` and `permissions.defaultMode: auto` are rejected, because CodeBuddy ignores them there.

### Hooks

- Command hooks keep native event names, matchers, and timeouts in seconds. Arguments are shell-quoted into the command.
- Prompt hooks (`type: prompt`) work on `Stop`, `UserPromptSubmit`, and `PreToolUse` in project settings. They write `prompt`, `timeout`, and `continueOnBlock`.
- CodeBuddy has no prompt `model` field, so sync drops it with a note.
- Sync rejects prompt hooks on other events, prompt hooks in user settings, and other handler types.
- Settings merge with existing native keys and hook entries.
- CodeBuddy reviews hook changes itself. Frontmatter hooks also need the user's `allowUntrustedFrontmatterHooks` choice. Sync never makes trust or approval decisions.

### Not generated

Ignore files, review configuration, environments, plugins, credentials, sessions, and automatic memories have no output in this adapter.

## Config keys

Shared root instruction file: [entry-point files](@/docs/configuration.md#entry-point-files).

| Output key | Default |
|---|---|
| `dir` under `outputs.codebuddy` | `.codebuddy` |
| `outputs.codebuddy.rules-dir` | `.codebuddy/rules` |
| `outputs.codebuddy.agents-dir` | `.codebuddy/agents` |
| `outputs.codebuddy.skills-dir` | `.codebuddy/skills` |
| `outputs.codebuddy.commands-dir` | `.codebuddy/commands` |
| `outputs.codebuddy.conf-file` | `.codebuddy/settings.json` |
| `outputs.codebuddy.root-mcp-file` | Unset |

`dir` moves the tool directory; individual keys win. Scoped instruction files keep their native names. Custom paths outside CodeBuddy's documented paths need its own discovery configuration.
