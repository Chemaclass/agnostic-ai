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

[`agnostic-ai compare claude cursor`](@/docs/cli-reference/inspect.md#compare) reports the connection fields each tool gets and the references it leaves out, without printing values or connecting to servers.

## Write one

`agnostic-ai new mcp filesystem` creates `mcps/filesystem.yaml`, plain YAML with no body. Replace its placeholder with:

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

Replace the path with a real directory, sync, and check that the server connects in your coding tool. A remote server for two tools, defined but switched off:

```yaml
name: docs-search
description: Search the internal docs.
type: http
url: https://mcp.example.com/mcp
disabled: true
targets: [claude, codex]
```

[MCP recipes](@/docs/spec-format/mcp-recipes.md) has real servers to copy.

## Fields

A server needs `command` (stdio) or `url` (remote). Only [`agnostic-ai lint`](@/docs/cli-reference/check.md#lint) reports a missing one (LINT008), so run it before `sync`.

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config, not the filename. Package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`, are percent-encoded in filenames and kept in configs. On Windows, import refuses a name with a colon and no slash, which would keep the colon in the filename; see [path semantics](@/docs/configuration.md#path-semantics). |
| `description` | no | empty | Free-form notes. Dropped for Junie and Warp. |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote types write an explicit `type`; `stdio` does not. Augment, Factory, and Qoder get no server for `ws`, and Ellipsis none for `sse` or `ws`. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. An element can hold a [reference](#references-in-url-and-args). |
| `env` | no | empty | Environment variables for the server. Each value is a [reference](#environment-references) or a [plain setting marked `!literal`](#plain-settings). |
| `url` | http/sse/ws only | none | Endpoint URL. It can hold a [reference](#references-in-url-and-args). |
| `headers` | no | empty | HTTP headers for `http`/`sse`. Each value is a [reference](#environment-references) or a [plain setting marked `!literal`](#plain-settings). |
| `cwd` | no | empty | Working directory for a stdio server, where the tool supports it. |
| `timeout` | no | empty | Milliseconds on most tools, including [Junie](@/docs/targets/junie.md#mcp). |
| `oauth` | no | empty | OAuth settings. The format differs by tool. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for tools with MCP roots. |
| `agent-only` | no | `false` | Keep the server out of every MCP file. Only the agents that name it can reach it. See [Agent-only servers](#agent-only). |

## Agent-only servers {#agent-only}

Set `agent-only: true` to give a server to some agents and keep its tools out of the main session's context.

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

Sync leaves the server out of every project and user MCP file and writes its full definition into each agent file that names it, on Claude Code, Qoder, Kiro, and OpenHands. Every other tool gets no copy, with a note ([`mcpServers` support by target](@/docs/spec-format/agents.md#mcpservers-support-by-target)).

- An agent file is committed, so a server written there is checked more strictly: a credential in `url`, `args`, an `x-<target>` block's `env` or `headers`, or a `${NAME:-default}` default is an error (LINT035), even from a [pack](@/docs/packs.md), instead of the [LINT049 warning](#plain-settings).
- Sync and `validate` refuse an `agent-only` value other than `true` or `false`, such as `yes`.
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

Sync writes `NODE_ENV: production` to every tool. An editor with the YAML language server needs `!literal scalar` in its `yaml.customTags` setting to accept the tag.

Any other `env` or `headers` value, including text around a reference such as `postgres://u:pw@${HOST}/db`, is LINT035: `sync` refuses it unless the spec comes from a [pack](@/docs/packs.md). The message names the server, field, and key, never the value. Empty values, numbers, and booleans are not checked.

A credential in `url` or `args`, in an `x-<target>` block's `env` or `headers`, or in a `${NAME:-default}` default gets a LINT049 warning, and `sync` still runs. The check reads the value's shape, since those places take no `!literal` mark.

[`agnostic-ai migrate --only secrets`](@/docs/cli-reference/upgrades.md#migrate) rewrites existing specs. A value that looks like a [credential](@/docs/spec-format/mcp-import.md#what-import-writes) becomes a reference, such as `GITHUB_TOKEN: ${GITHUB_TOKEN}`, and the command lists each variable to set, never the value. Every other value gets `!literal`. It leaves two cases to you: a key with a [credential name](@/docs/spec-format/mcp-import.md#credential-names) whose value does not look like a credential, such as `API_KEY: sk-live-abc`, and a credential around a reference.

## Environment references

Write a token or key as `${NAME}`, whole or in part, in an `env` or `headers` value, a `url`, or an `args` element:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
headers:
  Authorization: Bearer ${API_KEY}
url: https://${API_HOST}/mcp
args: [--token, "${GH_TOKEN}"]
```

`${NAME:-default}` uses `default` when `NAME` is unset. Only Claude Code, Crush, OpenHands, and Gemini support that form. Any other `${...}`, such as `${env:NAME}` or `${input:id}`, is not a reference.

Sync writes each tool's own form, and never writes a reference as plain text. Where a tool cannot read a reference in that field, including a `${NAME:-default}` outside the four tools above, the note names the server, the key, and the variable, and:

- in `env` or `headers`, sync leaves the key out, as it does for any other `${...}` there;
- in `url` or `args`, sync leaves the whole server out, because dropping one argument would change the command.

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

Kiro expands only variables approved under **Mcp Approved Env Vars** in its settings. Factory fails the connection when a referenced variable is unset. Continue's IDE extensions read secrets from project `.env`, `.continue/.env`, or `~/.continue/.env`, and its CLI also reads environment variables ([secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables)). Keep `.env` files out of Git.

[`agnostic-ai doctor`](@/docs/cli-reference/check.md#doctor) lists each referenced variable unset in your shell, including in `x-<target>` blocks and disabled servers. A tool started from a desktop launcher may see a different environment.

### References in `url` and `args`

`url` and `args` take the same references, with the stricter rule above: a tool that cannot read one there gets no server.

- A tool's own form at the top level, such as `${env:NAME}`, is copied as text to every other tool, and `lint` reports it as LINT028. Write `${NAME}`, or put the tool's own text under `x-<target>:`.
- `${workspaceFolder}`, `${workspaceFolderBasename}`, `${userHome}`, `${pathSeparator}`, and any other `${...}`, such as `${input:id}`, are not references and stay as written.

### Literal `${NAME}` text {#literal-text}

Some servers expand `${NAME}` themselves, such as `mcp-remote` in a header argument. Write `$${NAME}` to pass the text on unchanged. It works in `url`, `args`, `env`, and `headers`:

```yaml
command: npx
args: ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"]
```

Sync writes `${X_CHROMA_TOKEN}` to every built-in tool, with no note; an external adapter gets the spec as written. Quote it inside a `[...]` list, as you would `${NAME}`.

A tool whose column in the [table above](#environment-references) shows `${NAME}` for that field expands the text itself, from its own environment rather than the server's `env` block, before the server sees it. None of them can write a literal `${`, so sync still writes the server. A value expanded in `args` shows in the process list. When the variable is unset and has no default, Claude Code keeps the `${NAME}` text and warns in `claude mcp list`.

### What import writes

`import` turns native references and literal credentials into `${NAME}` references, and lists each variable to set. [MCP import credentials](@/docs/spec-format/mcp-import.md) owns the rewrite rules, [credential names](@/docs/spec-format/mcp-import.md#credential-names), and detection limits.

## Credential names {#credential-names .legacy-anchor}

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
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in an agent's `mcp_servers` |

The Crush and Claude Code fields also work under `x-crush` or `x-claude`, which wins, and other tools leave them out. On Amp, set `x-amp.includeTools`. `x-factory`, `x-kilo`, and `x-continue` override the matching top-level options for that target. Each target page gives the details: [Factory tool lists](@/docs/targets/factory.md#mcp), [Crush channels](@/docs/targets/crush.md#mcp), and [Claude Code flags](@/docs/targets/claude-reference.md#mcp-mappings).

## `disabled` support by target

This table covers project files. For user files written by `sync --global`, see [global MCP servers](@/docs/global-configuration.md#global-mcp-servers).

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. The key is removed from both MCP files with a note. Disable the server in VS Code yourself |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers written to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | The key is removed with a note, and the server stays in the file. Disable the server in the tool itself; Warp starts a project server only when you start it |
| [Ellipsis](@/docs/targets/ellipsis.md) | Server left out, since a listed server is always available. Written into the environment file, not an MCP file |
