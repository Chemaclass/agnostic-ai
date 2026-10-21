+++
title = "MCP servers"
description = "mcps/: MCP server connections, declared once and written into each tool's MCP config."
weight = 60
aliases = ["/docs/spec-format/mcp-servers/"]

[extra]
group = "Reference"
+++

# MCP servers

`mcps/` connects agents to outside tools and data through the Model Context Protocol: a browser, a database, GitHub, an issue tracker. Each tool keeps servers in its own file and format, such as `.mcp.json`, `.cursor/mcp.json`, or `[mcp_servers.*]` in `.codex/config.toml`. One YAML file per server feeds all of them.

Before you change tools, compare connection fields with [`agnostic-ai compare claude cursor`](@/docs/cli-reference/inspect.md#compare). It reports the fields each tool gets and the references it leaves out, without printing values or connecting to servers.

## Write one

`agnostic-ai new mcp filesystem` creates `mcps/filesystem.yaml`, plain YAML with no markdown body. Replace its placeholder with:

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
  - /path/to/allowed/dir
```

Replace `/path/to/allowed/dir` with a real directory. Sync, then check that the server connects in your coding tool. A remote server for two tools, defined but switched off:

```yaml
name: docs-search
description: Search the internal docs.
type: http
url: https://mcp.example.com/mcp
disabled: true
targets: [claude, codex]
```

Real servers to copy are on the [MCP recipes](@/docs/spec-format/mcp-recipes.md) page.

## Fields

`name` is the server identifier, not the filename. It may contain package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`. Such names are percent-encoded in filenames and written as they are in every generated config.

A server needs `command` (stdio) or `url` (remote). Only `agnostic-ai lint` reports a missing one (LINT008), so run it before `sync`. See [lint](@/docs/cli-reference/check.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form notes. Dropped for Junie and Warp. |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote types write an explicit `type`; `stdio` does not. Augment, Factory, and Qoder get no server for `ws`, and Ellipsis none for `sse` or `ws`. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. An element can hold a [reference](#references-in-url-and-args). |
| `env` | no | empty | Environment variables for the server. Each value is a [reference](#environment-references) or a [plain setting marked `!literal`](#plain-settings). |
| `url` | http/sse/ws only | none | Endpoint URL. It can hold a [reference](#references-in-url-and-args). |
| `headers` | no | empty | HTTP headers for `http`/`sse`. Each value is a [reference](#environment-references) or a [plain setting marked `!literal`](#plain-settings). |
| `cwd` | no | empty | Working directory for a stdio server, where the tool supports it. |
| `timeout` | no | empty | Milliseconds on most tools. |
| `oauth` | no | empty | OAuth settings. The format differs by tool. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for tools with MCP roots. |
| `agent-only` | no | `false` | Keep the server out of every MCP file. Only the agents that name it can reach it. See [Agent-only servers](#agent-only). |

## Agent-only servers {#agent-only}

Set `agent-only: true` to give a server to some agents and keep it away from the main session. Its tools then add nothing to the main session's context.

```yaml
# mcps/github.yaml
name: github
agent-only: true
command: npx
args: ["-y", "@modelcontextprotocol/server-github"]
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
```

```yaml
# agents/triager.md frontmatter
mcpServers: [github]
```

Sync leaves the server out of every project and user MCP file. It writes the server's full definition into the agent file of each agent that names it, on the tools whose agent file can hold one: [Claude Code](@/docs/targets/claude.md), [Qoder](@/docs/targets/qoder.md), [Kiro](@/docs/targets/kiro.md), and [OpenHands](@/docs/targets/openhands.md). Every other tool gets no copy of the server, and sync prints a note. See [`mcpServers` support by target](@/docs/spec-format/agents.md#mcpservers-support-by-target).

- The agent file is committed with every copy of the agent, so sync checks a server it writes there more strictly. Besides a literal `env` or `headers` value, it refuses a credential in `url` or `args`, in an `x-<target>` block's `env` or `headers`, and in a `${NAME:-default}` default, even for a server from a [pack](@/docs/packs.md). `validate` and `lint` report these as LINT035; outside an agent file they are a [LINT049 warning](#plain-settings).
- `agent-only` must be `true` or `false`. Sync and `validate` refuse any other value, such as `yes`.
- A server no agent names reaches no tool. Sync prints a note.
- A `disabled: true` server is left out of the agent file.
- `sync --global` writes no agent-only server.
- Import does not create agent-only servers yet. `import claude` keeps an inline server in the agent's `mcpServers` list, and sync writes it back as it was.

## Secrets and plain settings {#plain-settings}

A value in `env` or `headers` is a reference by default: `${NAME}`, `${NAME:-default}`, or `$${NAME}` text. A header may put one word before it, as in `Bearer ${API_KEY}`. Mark a plain setting that is not a secret with `!literal`:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  NODE_ENV: !literal production
```

Sync writes `NODE_ENV: production` to every tool. An editor with the YAML language server reports an unknown tag until you add `!literal scalar` to its `yaml.customTags` setting.

Any other `env` or `headers` value fails:

- `agnostic-ai lint` reports it as LINT035, and `sync` refuses to write it unless the spec comes from a [pack](@/docs/packs.md). The message names the server, the field, and the key, never the value.
- A value with text around a reference, such as `postgres://u:pw@${HOST}/db`, fails too.
- Empty values, numbers, and booleans are not checked.

A credential in `url` or `args`, in an `x-<target>` block's `env` or `headers`, or in a `${NAME:-default}` default gets a LINT049 warning instead, and `sync` still runs. The check reads the value's shape, and those places take no `!literal` mark. In an [agent file](#agent-only) it is an error.

[`agnostic-ai migrate --only secrets`](@/docs/cli-reference/upgrades.md#migrate) rewrites existing specs. A value that looks like a [credential](#what-import-writes) becomes a reference, such as `GITHUB_TOKEN: ${GITHUB_TOKEN}`, and the command lists each variable to set, never the value. Every other value gets `!literal`. It leaves two cases to you: a key with a [credential name](#credential-names) whose value does not look like a credential, such as `API_KEY: sk-live-abc`, and a credential around a reference.

## Environment references

Write a token or key as `${NAME}` in an `env` or `headers` value, whole or in part:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
headers:
  Authorization: Bearer ${API_KEY}
```

`${NAME:-default}` uses `default` when `NAME` is unset. Only Claude Code, Crush, OpenHands, and Gemini support that form. Any other `${...}`, such as `${env:NAME}` or `${input:id}`, is not a reference in a spec.

Sync writes each tool's own form. Where a tool cannot read a reference in that field, sync leaves the key out and prints a note naming the server, the key, and the variable. It never writes the reference as plain text. This applies to a tool with no reference form, Amp `env`, a `${NAME:-default}` outside the four tools above, and any other `${...}`.

| Target | `env` | `headers` |
|--------|-------|-----------|
| [Claude Code](@/docs/targets/claude.md), [Crush](@/docs/targets/crush.md), [OpenHands](@/docs/targets/openhands.md), [Gemini](@/docs/targets/gemini.md) | `${NAME}`, `${NAME:-default}` | `${NAME}`, `${NAME:-default}` |
| [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Ellipsis](@/docs/targets/ellipsis.md) | `${NAME}` | `${NAME}` |
| [Amp](@/docs/targets/amp.md) | Left out | `${NAME}` |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Codex](@/docs/targets/codex.md) | `KEY: ${KEY}` adds `KEY` to `env_vars`. Any other reference is left out | `Authorization: Bearer ${NAME}` sets `bearer_token_env_var`. Another header of exactly `${NAME}` joins `env_http_headers`. Any other reference is left out |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}` on stdio servers | `{% raw %}${{ secrets.NAME }}{% endraw %}` under `requestOptions.headers` |
| [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Left out | Left out |

Kiro expands only variables approved under **Mcp Approved Env Vars** in its settings. Factory fails the connection when a referenced variable is unset.

[`agnostic-ai doctor`](@/docs/cli-reference/check.md#doctor) lists each referenced variable that is unset in your shell, including in `x-<target>` blocks and disabled servers. Treat the list as a hint: a tool started from a desktop launcher may see a different environment, and Continue reads secrets from files too.

Continue's IDE extensions read secrets from project `.env`, `.continue/.env`, or `~/.continue/.env`. Its CLI also reads environment variables. See [Continue's secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables). Keep `.env` files out of Git.

### References in `url` and `args`

A `${NAME}` can also sit in `url` or an `args` element, such as a per-machine host or a token on the command line:

```yaml
url: https://${API_HOST}/mcp
args: [--token, "${GH_TOKEN}"]
```

Sync writes each tool's own form. A tool that reads no reference in that field gets no server at all, because dropping one argument would change the command. The note names the server, the field, and the variable. A `${NAME:-default}` follows the same rule outside Claude Code, Crush, OpenHands, and Gemini.

A tool's own form at the top level, such as `${env:NAME}`, is copied as text to every other tool, and `agnostic-ai lint` reports it as LINT028. Write `${NAME}`, or put the tool's own text under `x-<target>:`.

`${workspaceFolder}`, `${workspaceFolderBasename}`, `${userHome}`, and `${pathSeparator}` are tool variables, not references, and stay as written. So does any other `${...}`, such as `${input:id}`.

| Target | `url` | `args` |
|--------|-------|--------|
| [Claude Code](@/docs/targets/claude.md), [Crush](@/docs/targets/crush.md), [OpenHands](@/docs/targets/openhands.md), [Gemini](@/docs/targets/gemini.md) | `${NAME}`, `${NAME:-default}` | `${NAME}`, `${NAME:-default}` |
| [Amp](@/docs/targets/amp.md) | `${NAME}` | Server left out |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}` | `{% raw %}${{ secrets.NAME }}{% endraw %}` |
| [Codex](@/docs/targets/codex.md), [Ellipsis](@/docs/targets/ellipsis.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Server left out | Server left out |

### Literal `${NAME}` text {#literal-text}

Some servers expand `${NAME}` themselves, such as `mcp-remote` in a header argument. Write `$${NAME}` to pass the text on unchanged. It works in `url`, `args`, `env`, and `headers`:

```yaml
command: npx
args: ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"]
```

Sync writes `${X_CHROMA_TOKEN}` to every built-in tool, with no left-out server and no note. An external adapter gets the spec as written. Quote it inside a `[...]` list, as you would `${NAME}`.

A tool that expands `${NAME}` in that field fills in the value from its own environment before the server sees it: Claude Code, Crush, OpenHands, and Gemini in `url` and `args`, Amp in `url`, and the tools with `${NAME}` in the [`env` and `headers` table](#environment-references). None of them has a way to write a literal `${`, so sync still writes the server. The tool reads its own environment, not the server's `env` block. A value expanded in `args` shows in the process list. When the variable is unset and has no default, Claude Code keeps the `${NAME}` text and warns in `claude mcp list`.

### What import writes

Import converts native references and literal credentials before saving a server spec. Review the named variables before starting the tool. See [MCP import credentials](@/docs/spec-format/mcp-import.md) for rewrite rules, credential names, and detection limits.

#### Credential names

The [credential-name rules](@/docs/spec-format/mcp-import.md#credential-names) decide which fields import rewrites or leaves out.

## Target-only fields

| Target | Extra fields |
|--------|--------------|
| [Codex](@/docs/targets/codex.md) | `env_vars`, `env_http_headers`, `http_headers_helper`, `auth`, `required`, `startup_timeout_sec`, `startup_timeout_ms`, `tool_timeout_sec`, `default_tools_approval_mode`, `scopes`, `oauth_resource`, `experimental_environment`, `enabled_tools`, `disabled_tools` |
| [Crush](@/docs/targets/crush.md) | `enabled_tools`, `disabled_tools`, `sessionless`, `channel_enabled`, `channel_reply` |
| [Gemini](@/docs/targets/gemini.md) | `trust`, `includeTools`, `excludeTools` |
| [Qoder](@/docs/targets/qoder.md) | `trust`, `includeTools`, `excludeTools`, `alwaysAllow` |
| [Kiro](@/docs/targets/kiro.md) | `autoApprove`, `disabledTools`, `oauthScopes` |
| [Factory](@/docs/targets/factory.md) | `x-factory.enabledTools`, `disabledTools`, `connectTimeout` |
| [Claude Code](@/docs/targets/claude.md) | `alwaysLoad`, `bareElicitationCapability`, `headersHelper` |
| [Cursor](@/docs/targets/cursor.md) | `envFile`, `auth` |
| [Copilot / VS Code](@/docs/targets/copilot.md) | `envFile`, `dev`, `sandboxEnabled` and `sandbox` (VS Code file only), `tools` (Copilot CLI files only) |
| [Continue](@/docs/targets/continue.md) | `connectionTimeout`, `requestOptions` |
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in `~/.openhands/mcp.json` |

On Amp, set `x-amp.includeTools`. Use `x-factory`, `x-kilo`, or `x-continue` to override the matching top-level options for that target. Factory's tool list goes under `x-factory` ([filtering and import](@/docs/targets/factory.md#mcp)):

```yaml
x-factory:
  enabledTools: [read_file, shared_tool]
  disabledTools: [shared_tool]
```

Crush's `channel_enabled` keeps explicit `true` and `false`. `channel_reply` selects the direct and group reply tools, their argument names, and the tools that suppress duplicate replies. Set these fields at the top level or under `x-crush`, which wins. Import and sync preserve them. See [Crush MCP channels](@/docs/targets/crush.md#mcp).

Claude Code's `alwaysLoad` and `bareElicitationCapability` keep an explicit `true` or `false`. Omit a flag to keep Claude Code's default. Set them at the top level or under `x-claude`, which wins. Other tools leave out both flags. See [Claude Code MCP behavior](@/docs/targets/claude-reference.md#mcp-mappings).

```yaml
x-claude:
  alwaysLoad: false
  bareElicitationCapability: true
```

## `disabled` support by target

This table covers project files. For user files written by `sync --global`, see [global MCP servers](@/docs/global-configuration.md#global-mcp-servers).

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. The key is removed from both MCP files with a note. Disable the server in VS Code yourself |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers written to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | The key is removed with a note, so the server stays on. Disable the server in the tool itself |
| [Ellipsis](@/docs/targets/ellipsis.md) | Server left out, since a listed server is always available. Written into the environment file, not an MCP file |
