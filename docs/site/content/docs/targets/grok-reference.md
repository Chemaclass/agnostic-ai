+++
title = "Grok Build native reference"
description = "Native field mappings, import coverage, and global configuration for grok."
weight = 266

[extra]
nav_title = "Grok Build reference"
group = "Reference"
+++

# Grok Build native reference

Start with the [Grok setup](@/docs/targets/grok.md).

## Native mappings

### Agents

- Kept: `name`, `description`, `model`, native `tools`, `disallowedTools`, `skills`, `effort`, `maxTurns`, `background`, `permissionMode`, `hooks`, and supported fields under `x-grok`.
- `effort`: `low`, `medium`, `high`, `xhigh`, or `max`.
- Portable `can` and `tools` map to verified native allowlist aliases and disable unrequested MCP bridge tools.
- Explicit bridge IDs and `x-grok.tools` keep their native choice.
- A capability with no exact mapping fails and names the agent.
- An unknown native tool fails, because Grok restores its full toolset for an unresolved name.
- An empty restriction is rejected, because native empty `tools` inherits every tool.
- To keep an existing restriction, put verified aliases or IDs in `tools` or `x-grok.tools`.

### Skills and names

- Kept: `name`, `description`, `when-to-use`, `paths`, `allowed-tools`, `argument-hint`, `user-invocable`, `disable-model-invocation`, `metadata`, `license`, `compatibility`. Bundled files are copied unchanged.
- `allowed-tools` describes use; it enforces nothing.
- `user-invocable: false` also hides the skill from model discovery.
- Skill model and effort overrides are not applied.
- Skill and command names become lowercase `a-z`, digits, and hyphens, up to 64 characters. A skill can fall back to its folder name.
- A name with no valid native form is rejected before any file is written.

### MCP servers

- Written as `[mcp_servers.<name>]`.
- Stdio keeps `command`, `args`, `env`, `cwd`. Remote keeps `url`, `headers`, and explicit HTTP or SSE transport.
- `disabled` becomes the inverse `enabled`.
- `startup_timeout_sec` and `tool_timeout_sec` stay in seconds.
- Environment references stay as text for Grok to expand.
- `x-grok` carries native extras, such as authentication and timeouts.
- Portable `description` and `timeout` give coverage notes (no exact mapping).

### Project settings

- Only `permissions.allow`, `permissions.ask`, and `permissions.deny` are written, to `[permission]`.
- `x-grok.permission` and `x-grok.plugins` carry native project configuration.
- Existing MCP servers and unrelated keys are kept.
- Unsupported: project model, effort, permission default mode, session retention.
- Ignore, review, and environment specs have no native output.
- User scope: portable [voice settings](@/docs/spec-format/settings-reference.md#voice-dictation) write `ui.voice_capture_mode` and `ui.voice_stt_language`. Grok has no dictation on/off key.

### Hooks

- Fields: native PascalCase `event`, optional `matcher`, command, safely quoted `args`, timeout in seconds, per-handler `env`.
- Sync adds `AGNOSTIC_AI_TARGET=grok`. Grok provides `CLAUDE_PROJECT_DIR` and `GROK_WORKSPACE_ROOT`.
- A matcher on `Stop` or `UserPromptSubmit` does nothing and gives a note.
- Errors fail open. Native explicit denies block, depending on the event.
- Portable `on` and `match` hooks are not translated. Use Grok-native events and its reply protocol.
- Project hooks need Grok's hook trust approval.
- Windows: a shared script path that needs shell-specific quoting (such as a path with spaces) needs an explicit native wrapper command. Sync refuses a rewrite it cannot preserve. Existing native invocation commands keep their quoting.

## Import

Run `agnostic-ai import grok` before the first sync of a native project. It uses the configured output directories.

- Reads: root and scoped rule, agent, skill, and command directories, resources, native hook JSON, MCP tables, compact permission lists, plugins.
- Plugins stay under `x-grok.plugins`. `x-grok.hooks` keeps each hook file's groups and handler fields.
- An unsupported hook handler type fails import.
- Standalone MCP credentials become environment references through the shared credential filter.
- MCP `description` is left out with a note, because Grok ignores it. The same server imported from Claude Code or Cursor keeps its description.
- Global sync preserves the original native file.
- Inline agent MCP and hook metadata keep their native form. Literal credentials there and in standalone hooks are refused before any write ([setting value rule](@/docs/spec-format/mcp-import.md#setting-values)). Errors name the path, never the value.
- Hook `env` values pass through literally, so `${TOKEN}` there reads nothing. Use it in the hook `command` or `url`, where Grok expands variables.
- Adapter tests cover the round trip: import, sync, lint, `sync --check`.

## Global configuration

- `agnostic-ai sync --global --only grok` writes `$GROK_HOME/AGENTS.md`, `skills/`, `agents/`, `hooks/`, `hook-scripts/`, and `config.toml`.
- `GROK_HOME` defaults to `~/.grok` and must be absolute when set.
- Global commands and permission lists are not written. Global effort is unsupported.
- The global model maps to `[models].default`.
- MCP servers and the model share one planned write of the config file. Sync owns individual servers and the model key, keeps other text and private file modes, checks for concurrent edits, and removes only unchanged owned values.
- `agnostic-ai import --global grok` reads supported settings, MCP, and hooks. It skips literal credentials.
