+++
title = "MCP servers"
description = "mcps/: MCP server connections, declared once and written into each tool's MCP config."
weight = 60
aliases = ["/docs/spec-format/mcp-servers/"]

[extra]
group = "Reference"

[extra.moved]
target-only-fields = "@/docs/spec-format/mcps-reference.md"
disabled-support-by-target = "@/docs/spec-format/mcps-reference.md"
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

A server needs `command` (stdio) or `url` (remote). Only [`agnostic-ai lint`](@/docs/cli-reference/check.md#lint) reports a missing one, or an unknown `type` (LINT008), so run it before `sync`.

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config, not the filename. |
| `description` | no | empty | Free-form notes. Dropped for Codex, Grok, Junie, and Warp. |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list. An element can hold a [reference](#references-in-url-and-args). |
| `env` | no | empty | Environment variables. Each value is a [reference](#environment-references) or a [plain setting marked `!literal`](#plain-settings). |
| `url` | http, sse, ws | none | Endpoint URL. It can hold a [reference](#references-in-url-and-args). |
| `headers` | no | empty | HTTP headers for `http` and `sse`. Same value rules as `env`. |
| `cwd` | no | empty | Working directory for a stdio server, where the tool supports it. |
| `timeout` | no | empty | Milliseconds. See the details below for tools that differ. |
| `oauth` | no | empty | OAuth settings. The format differs by tool. |
| `disabled` | no | `false` | See [`disabled` support by target](@/docs/spec-format/mcps-reference.md#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for tools with MCP roots. Dropped for Codex. |
| `agent-only` | no | `false` | Keep the server out of every MCP file. See [Agent-only servers](#agent-only). |

{% <details summary="Details: names, types, timeouts and roots"> %}
- `name`: package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`, are percent-encoded in filenames and kept in configs.
- `name` on Windows: import refuses a server whose spec file name would hold a colon, which a name with a colon and no slash does. See [path semantics](@/docs/configuration.md#path-semantics).
- `type`: `local` means `stdio`; `streamable-http` and `remote` mean `http`.
- `type`: remote types write an explicit `type`; `stdio` does not.
- `type`: Augment, Factory, and Qoder get no server for `ws`. Ellipsis gets none for `sse` or `ws`.
- `timeout`: milliseconds on most tools, including [Junie](@/docs/targets/junie-reference.md#mcp). [Kilo Code](@/docs/targets/kilo-reference.md#mcp) requires a positive whole number.
- `timeout`: [Kiro](@/docs/targets/kiro.md) agent files leave it out with a note. Set `x-kiro.timeout` or `x-kiro.requestTimeout` there.
- `timeout`: [Crush](@/docs/targets/crush-reference.md#mcp) and [Pi](@/docs/targets/pi.md#native-mappings) read seconds, so sync divides by 1000. Crush rounds up to whole seconds.
- `roots`: `uri` is required. Lint, `validate`, and sync refuse any other item, such as a plain string (LINT060).
{% </details> %}

## Agent-only servers {#agent-only}

Set `agent-only: true` to give a server only to the agents that name it, and keep its tools out of the main session.

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

Sync writes the full definition into each agent file that names it, on Claude Code, Qoder, Kiro, and OpenHands, and leaves it out of every MCP file. Other tools get no copy, with a note ([`mcpServers` support by target](@/docs/spec-format/agents-reference.md#mcpservers-support-by-target)).

<details>
<summary>Agent-only edge cases</summary>

- An agent file is committed, so a server written there is checked more strictly: a credential in `url`, `args`, an `x-<target>` block's `env` or `headers`, or a `${NAME:-default}` default is an error (LINT035), even from a [pack](@/docs/packs.md), instead of the [LINT049 warning](#plain-settings).
- Sync and `validate` refuse an `agent-only` value other than `true` or `false`, such as `yes`.
- A server no agent names reaches no tool. Sync prints a note.
- A `disabled: true` server is left out of the agent file.
- `sync --global` writes no agent-only server.
- Import does not create agent-only servers yet. `import claude` keeps an inline server in the agent's `mcpServers` list, and sync writes it back as it was.

</details>

## Secrets and plain settings {#plain-settings}

A value in `env` or `headers` is a reference by default: `${NAME}`, `${NAME:-default}`, or `$${NAME}` text. A header may put one word before it, as in `Bearer ${API_KEY}`. Mark a plain setting that is not a secret with `!literal`:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  NODE_ENV: !literal production
```

Sync writes `NODE_ENV: production` to every tool. Any other value, including text around a reference such as `postgres://u:pw@${HOST}/db`, is LINT035, and `sync` refuses it unless the spec comes from a [pack](@/docs/packs.md). The message names the server, field, and key, never the value. Empty values, numbers, and booleans are not checked.

An editor with the YAML language server needs `!literal scalar` in its `yaml.customTags` setting to accept the tag.

{% <details summary="Details: credentials in url, args and defaults, and the migrate command"> %}
- A credential in `url` or `args`, in an `x-<target>` block's `env` or `headers`, or in a `${NAME:-default}` default gets a LINT049 warning, and `sync` still runs.
- That check reads the value's shape, since those places take no `!literal` mark.
- [`agnostic-ai migrate --only secrets`](@/docs/cli-reference/upgrades.md#migrate) rewrites existing specs.
- A value that looks like a [credential](@/docs/spec-format/mcp-import.md#what-import-writes) becomes a reference, such as `GITHUB_TOKEN: ${GITHUB_TOKEN}`. The command lists each variable to set, never the value.
- Every other value gets `!literal`.
- It leaves two cases to you: a key with a [credential name](@/docs/spec-format/mcp-import.md#credential-names) whose value does not look like a credential, such as `API_KEY: sk-live-abc`, and a credential around a reference.
{% </details> %}

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

Sync writes each tool's own form and never writes a reference as plain text. Where a tool cannot read a reference in a field, the note names the server, the key, and the variable, and:

- in `env` or `headers`, sync leaves the key out;
- in `url` or `args`, sync leaves the whole server out, because dropping one argument would change the command.

[Each tool's form and fields](@/docs/spec-format/mcps-reference.md#environment-references-by-target) are in the reference. [`agnostic-ai doctor`](@/docs/cli-reference/doctor.md#doctor) lists each referenced variable unset in your shell, including in `x-<target>` blocks and disabled servers.

{% <details summary="Details: defaults, other forms and per-tool notes"> %}
- `${NAME:-default}` uses `default` when `NAME` is unset. Only Claude Code, Crush, OpenHands, and Gemini support it. On any other tool the note names the server, key, and variable, and the key or server is left out.
- Any other `${...}`, such as `${env:NAME}` or `${input:id}`, is not a reference.
- Kiro expands only variables approved under **Mcp Approved Env Vars** in its settings.
- Factory fails the connection when a referenced variable is unset.
- Continue's IDE extensions read secrets from project `.env`, `.continue/.env`, or `~/.continue/.env`. Its CLI also reads environment variables ([secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables)). Keep `.env` files out of Git.
- A tool started from a desktop launcher may see a different environment from your shell.
{% </details> %}

### References in `url` and `args`

`url` and `args` take the same references, with the stricter rule above: a tool that cannot read one there gets no server. `${workspaceFolder}`, `${userHome}`, and any other `${...}` that is not a reference stay as written.

{% <details summary="Details: a tool's own reference form, and variables left as written"> %}
- A tool's own form at the top level, such as `${env:NAME}`, is copied as text to every other tool, and `lint` reports it as LINT028. Write `${NAME}`, or put the tool's own text under `x-<target>:`.
- Left as written: `${workspaceFolder}`, `${workspaceFolderBasename}`, `${userHome}`, `${pathSeparator}`, and any other `${...}`, such as `${input:id}`.
{% </details> %}

### Literal `${NAME}` text {#literal-text}

Some servers expand `${NAME}` themselves, such as `mcp-remote` in a header argument. Write `$${NAME}` to pass the text on unchanged. It works in `url`, `args`, `env`, and `headers`:

```yaml
command: npx
args: ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"]
```

Sync writes `${X_CHROMA_TOKEN}` to every built-in tool, with no note; an external adapter gets the spec as written. Quote it inside a `[...]` list, as you would `${NAME}`.

{% <details summary="Details: how tools expand literal text"> %}
- A tool whose column in the [per-tool table](@/docs/spec-format/mcps-reference.md#environment-references-by-target) shows `${NAME}` for that field expands the text itself, before the server sees it.
- It expands from its own environment, not the server's `env` block.
- None of them can write a literal `${`, so sync still writes the server.
- A value expanded in `args` shows in the process list.
- When the variable is unset and has no default, Claude Code keeps the `${NAME}` text and warns in `claude mcp list`.
{% </details> %}

### What import writes

`import` turns native references and literal credentials into `${NAME}` references, and lists each variable to set. [MCP import credentials](@/docs/spec-format/mcp-import.md) describes the rewrite rules, [credential names](@/docs/spec-format/mcp-import.md#credential-names), and detection limits.

## Credential names {#credential-names .legacy-anchor}

The fields only some tools read, and how each tool handles `disabled`, are in the [MCP reference](@/docs/spec-format/mcps-reference.md).
