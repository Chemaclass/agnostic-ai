+++
title = "MCP reference"
description = "Fields only some tools read for an MCP server, and how each tool handles disabled."
weight = 61

[extra]
group = "Reference"
+++

# MCP reference

Per-tool details for the fields on the [MCP servers page](@/docs/spec-format/mcps.md).

## Target-only fields {#target-only-fields}

| Target | Extra fields |
|--------|--------------|
| [Codex](@/docs/targets/codex.md) | `env_vars`, `env_http_headers`, `http_headers_helper`, `auth`, `required`, `startup_timeout_sec`, `startup_timeout_ms`, `tool_timeout_sec`, `default_tools_approval_mode`, `scopes`, `oauth_resource`, `environment_id`, `enabled_tools`, `disabled_tools` |
| [Crush](@/docs/targets/crush.md) | `enabled_tools`, `disabled_tools`, `sessionless`, `channel_enabled`, `channel_reply` |
| [Gemini](@/docs/targets/gemini.md) | `trust`, `includeTools`, `excludeTools` |
| [Qoder](@/docs/targets/qoder.md) | `trust`, `includeTools`, `excludeTools`, `alwaysAllow` |
| [Kiro](@/docs/targets/kiro.md) | `autoApprove`, `disabledTools`, `oauth`, `oauthScopes` |
| [Factory](@/docs/targets/factory.md) | `x-factory.enabledTools`, `disabledTools`, `connectTimeout` |
| [Windsurf / Devin Desktop](@/docs/targets/windsurf.md) | `disabledTools`, `oauthScopes` (remote servers) |
| [Claude Code](@/docs/targets/claude.md) | `alwaysLoad`, `bareElicitationCapability`, `headersHelper` |
| [Cursor](@/docs/targets/cursor.md) | `envFile`, `auth` |
| [Copilot / VS Code](@/docs/targets/copilot.md) | `envFile`, `dev`, `sandboxEnabled` and `sandbox` (VS Code file only), `tools` (Copilot CLI files only) |
| [Continue](@/docs/targets/continue.md) | `connectionTimeout`, `requestOptions` |
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in an agent's `mcp_servers` |

<details>
<summary>Codex environment ID and target overrides</summary>

Codex `environment_id` is the ID of a configured executor and defaults to `local`. The older `experimental_environment` still loads: sync writes `local` as `environment_id = "local"` and leaves `remote` out, since Codex no longer reads that key. `lint` and project `sync` say so. [`agnostic-ai migrate --only mcps`](@/docs/cli-reference/upgrades.md#migrate) rewrites `local`.

The Crush and Claude Code fields also work under `x-crush` or `x-claude`, which wins, and other tools leave them out. On Amp, set `x-amp.includeTools`. `x-factory`, `x-kilo`, and `x-continue` override the matching top-level options for that target. Each target page gives the details: [Factory tool lists](@/docs/targets/factory-reference.md#mcp), [Crush channels](@/docs/targets/crush-reference.md#mcp), and [Claude Code flags](@/docs/targets/claude-reference.md#mcp-mappings).

</details>

## `disabled` support by target {#disabled-support-by-target}

This table covers project files. For user files written by `sync --global`, see [global MCP servers](@/docs/global-configuration-reference.md#global-mcp-servers).

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. The key is removed from both MCP files with a note. Disable the server in VS Code yourself |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers written to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | The key is removed with a note, and the server stays in the file. Disable the server in the tool itself; Warp starts a project server only when you start it |
| [Ellipsis](@/docs/targets/ellipsis.md) | Server left out, since a listed server is always available. Written into the environment file, not an MCP file |

## Environment references by target {#environment-references-by-target}

How sync writes a `${NAME}` reference for each tool. Where the cell says "left out", the note names the server, key, and variable. See [environment references](@/docs/spec-format/mcps.md#environment-references).

| Target | `env` and `headers` | `url` and `args` |
|--------|---------------------|------------------|
| [Claude Code](@/docs/targets/claude.md), [Crush](@/docs/targets/crush.md), [OpenHands](@/docs/targets/openhands.md), [Gemini](@/docs/targets/gemini.md) | `${NAME}`, `${NAME:-default}` | `${NAME}`, `${NAME:-default}` |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}`, in `env` on stdio servers and under `requestOptions.headers` | `{% raw %}${{ secrets.NAME }}{% endraw %}` |
| [Amp](@/docs/targets/amp.md) | `headers`: `${NAME}`. `env`: key left out | `url`: `${NAME}`. `args`: server left out |
| [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Ellipsis](@/docs/targets/ellipsis.md) | `${NAME}` | Server left out |
| [Codex](@/docs/targets/codex.md) | `env`: `KEY: ${KEY}` adds `KEY` to `env_vars`. `headers`: `Authorization: Bearer ${NAME}` sets `bearer_token_env_var`, and another header of exactly `${NAME}` joins `env_http_headers`. Any other reference: key left out | Server left out |
| [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Key left out | Server left out |
