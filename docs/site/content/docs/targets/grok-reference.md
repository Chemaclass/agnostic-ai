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

Agents carry `name`, `description`, `model`, native `tools`, `disallowedTools`, `skills`, `effort`, `maxTurns`, `background`, `permissionMode`, `hooks`, and the supported native fields under `x-grok`. Effort accepts `low`, `medium`, `high`, `xhigh`, or `max`. Portable `can` and `tools` use verified native allowlist aliases and disable unrequested MCP bridge tools. Explicit bridge IDs and `x-grok.tools` retain their native choices; a capability without an exact mapping fails with the agent name. Unknown native tool entries fail because Grok restores its full toolset for an unresolved name. Empty restrictions are rejected because native empty tools inherit all tools. To preserve an existing Grok restriction, use verified native aliases or IDs in `tools` or `x-grok.tools`.

Skills keep `name`, `description`, `when-to-use`, `paths`, `allowed-tools`, `argument-hint`, `user-invocable`, `disable-model-invocation`, `metadata`, `license`, and `compatibility`. Bundled files are copied unchanged. `allowed-tools` describes use and does not enforce permission. `user-invocable: false` also hides a skill from model discovery. Skill model and effort overrides are not applied by the verified contract.

Grok converts skill and command names to lowercase `a-z`, digits and hyphens, with a 64-character limit. Skills can fall back to their folder name. Sync rejects names with no valid native form before writing files.

MCP servers become `[mcp_servers.<name>]`. Stdio keeps `command`, `args`, `env`, and `cwd`. Remote servers keep `url`, `headers`, and explicit HTTP or SSE transport. `disabled` becomes the inverse native `enabled`. Native `startup_timeout_sec` and `tool_timeout_sec` stay seconds. Environment references remain text for Grok to expand. Native extras under `x-grok` include the source-backed authentication and timeout fields. Portable `description` and `timeout` produce coverage notes because they have no exact native mapping.

Project settings write only `permissions.allow`, `permissions.ask`, and `permissions.deny` to `[permission]`; `x-grok.permission` and `x-grok.plugins` carry native project configuration. Existing MCP servers and unrelated configuration keys are retained. Project model, effort, permission default mode, and session retention are unsupported. Ignore, review, and environment specs have no native output.

At user scope, portable [voice settings](@/docs/spec-format/settings.md#voice-dictation) write `ui.voice_capture_mode` and `ui.voice_stt_language`; Grok has no dictation on/off key.

Hooks use native PascalCase `event` and optional `matcher`, command text, safely quoted `args`, timeout in seconds, and per-handler `env`. Sync adds `AGNOSTIC_AI_TARGET=grok` to generated handlers. Grok provides `CLAUDE_PROJECT_DIR` and `GROK_WORKSPACE_ROOT`. A matcher on `Stop` or `UserPromptSubmit` has no effect and produces a note. Hook errors fail open; native explicit denies block according to the event. Portable `on`/`match` hooks are not translated in this adapter. Use Grok-native events and its documented reply protocol. Project hook execution also requires Grok's hook trust approval.

On Windows, shared script paths that need shell-specific quoting, such as paths with spaces, require an explicit native wrapper command. Sync refuses an automatic rewrite it cannot preserve. Existing native invocation commands keep their quoting.

## Import

Run `agnostic-ai import grok` before the first sync of a native project. It reads root and scoped rule, agent, skill, and command directories, resources, native hook JSON files, MCP tables, compact permission lists, and plugins. Plugins are retained under `x-grok.plugins` from the configured settings destination. `x-grok.hooks` preserves each native hook file's groups and handler fields. Unsupported hook handler types fail import. Imported standalone MCP credentials become environment references through the shared credential filter.

Project and global imports leave MCP `description` out of the spec with a note because Grok ignores it. Global sync preserves the original native file. Importing the same server from Claude Code or Cursor keeps that tool's description in the shared spec.

Inline agent MCP and hook metadata keep their native form. Import refuses literal credentials there, and in standalone hooks, before writing the source file, by the [setting value rule](@/docs/spec-format/mcp-import.md#setting-values). Hook `env` values are passed through literally, so putting `${TOKEN}` there does not read the process environment. Inherit credentials from the process and reference them in hook `command` or `url`, where Grok expands variables. Import errors identify the native path without printing credential values.

Import uses the configured output directories. Importing emitted files into fresh source directories, syncing again, linting, and running `sync --check` are covered by the adapter tests.

## Global configuration

`agnostic-ai sync --global --only grok` writes `$GROK_HOME/AGENTS.md`, `skills/`, `agents/`, `hooks/`, `hook-scripts/`, and `config.toml`. `GROK_HOME` defaults to `~/.grok` and must be absolute when set. Global commands and permission lists are not emitted. The portable global model maps to `[models].default`; global effort is unsupported.

Global MCP servers and the default model share one planned config write. Sync owns individual servers and the model key, preserves other text and private file modes, checks for concurrent edits, and removes only unchanged owned values. `agnostic-ai import --global grok` reads the supported settings, MCP, and hook surfaces. Literal credentials are skipped by global import.
