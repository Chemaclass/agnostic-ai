+++
title = "MCP servers"
description = "mcps/: MCP server connections, declared once and written into each tool's MCP config."
weight = 60
aliases = ["/docs/spec-format/mcp-servers/"]

[extra]
group = "Reference"
+++

# MCP servers

`mcps/` connects agents to outside tools and data through the Model Context Protocol: a browser, a database, GitHub, an issue tracker, internal APIs. Each tool keeps servers in its own file and shape, such as `.mcp.json`, `.cursor/mcp.json`, or `[mcp_servers.*]` in `.codex/config.toml`. One YAML file per server feeds all of them.

- **Declared once.** Add or change a server in one file; every tool picks it up on the next sync.
- **Local or remote.** A `stdio` server runs a command on the machine; `http`, `sse`, and `ws` servers connect to a URL.
- **Tool options kept.** Timeouts, tool filters, OAuth, and approval settings that only some tools read ride along where they apply.
- **Narrowed per agent.** An [agent's `mcpServers`](@/docs/spec-format/agents.md#mcpservers-support-by-target) limits which servers one subagent may reach.

## Write one

`agnostic-ai new mcp filesystem` creates `mcps/filesystem.yaml`. Pure YAML, no markdown body, one file per server.

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
env:
  ROOT: /tmp
```

A remote server, kept defined but switched off, only for two tools:

```yaml
name: docs-search
description: Search the internal docs.
type: http
url: https://mcp.example.com/mcp
disabled: true
targets: [claude, codex]
```

## Fields

`name` is the server identifier, not the filename. It may contain package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`. Such names are percent-encoded in YAML filenames and kept as-is in every generated config. Other spec kinds need one safe path segment, because their names become output paths.

A server needs `command` (stdio) or `url` (remote). `agnostic-ai lint` reports a missing one as LINT008; `validate` and `sync` do not, and some targets write a server that cannot start. See [lint](@/docs/cli-reference/check.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form documentation. Dropped where the target's MCP schema has no such key (Junie, Warp). |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote transports write an explicit `type`; `stdio` stays implicit. A `ws` entry emits no server on Augment, Factory, and Qoder. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. |
| `env` | no | empty | Environment variables for the server. |
| `url` | http/sse/ws only | none | Endpoint URL. |
| `headers` | no | empty | HTTP headers for `http`/`sse`. |
| `cwd` | no | empty | Working directory for a stdio server, where supported. |
| `timeout` | no | empty | Units vary by target: milliseconds on most. |
| `oauth` | no | empty | OAuth settings. The shape is target-specific; see the target page. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for targets that support MCP roots. |

## Target-only fields

These fields apply only to the listed targets and are ignored elsewhere.

| Target | Extra fields |
|--------|--------------|
| [Codex](@/docs/targets/codex.md) | `env_vars`, `env_http_headers`, `http_headers_helper`, `auth`, `required`, `startup_timeout_sec`, `startup_timeout_ms`, `tool_timeout_sec`, `default_tools_approval_mode`, `scopes`, `oauth_resource`, `experimental_environment`, `enabled_tools`, `disabled_tools` |
| [Crush](@/docs/targets/crush.md) | `enabled_tools`, `disabled_tools`, `sessionless` |
| [Gemini](@/docs/targets/gemini.md) | `trust`, `includeTools`, `excludeTools` |
| [Qoder](@/docs/targets/qoder.md) | `trust`, `includeTools`, `excludeTools`, `alwaysAllow` |
| [Kiro](@/docs/targets/kiro.md) | `autoApprove`, `disabledTools`, `oauthScopes` |
| [Factory](@/docs/targets/factory.md) | `disabledTools`, `connectTimeout` |
| [Claude Code](@/docs/targets/claude.md) | `alwaysLoad`, `headersHelper` |
| [Cursor](@/docs/targets/cursor.md) | `envFile`, `auth` |
| [Copilot / VS Code](@/docs/targets/copilot.md) | `envFile`, `dev`, `sandboxEnabled` (VS Code file only), `tools` (Copilot CLI files only) |
| [Continue](@/docs/targets/continue.md) | `connectionTimeout`, `requestOptions` |
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in `~/.openhands/mcp.json` |

On Amp, set `x-amp.includeTools`. Use `x-factory`, `x-kilo`, or `x-continue` to override the matching top-level options for that target.

## `disabled` support by target

Only the targets listed were checked.

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. Stripped from both MCP files with a note; disable the server in VS Code for that half |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers emitted to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | Stripped with a note. Disable the server in the tool itself |

