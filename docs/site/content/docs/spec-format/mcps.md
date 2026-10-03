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
- **Tool options kept.** Timeouts, tool filters, OAuth, and approval settings that only some tools read are passed on where they apply.
- **Narrowed per agent.** An [agent's `mcpServers`](@/docs/spec-format/agents.md#mcpservers-support-by-target) limits which servers one subagent may reach.

## Write one

`agnostic-ai new mcp filesystem` creates `mcps/filesystem.yaml`. It is pure YAML with no markdown body, one file per server.

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

A remote server for two tools, defined but switched off:

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

A server needs `command` (stdio) or `url` (remote). `agnostic-ai lint` reports a missing one as LINT008. `validate` and `sync` do not, so some targets get a server that cannot start. See [lint](@/docs/cli-reference/check.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form documentation. Dropped where the target's MCP schema has no such key (Junie, Warp). |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote transports write an explicit `type`; `stdio` stays implicit. A `ws` entry emits no server on Augment, Factory, and Qoder. |
| `command` | stdio only | none | Executable to launch. |
| `args` | no | empty | Argument list for the command. An element can hold a [reference](#references-in-url-and-args). |
| `env` | no | empty | Environment variables for the server. Write a secret as a [reference](#environment-references). |
| `url` | http/sse/ws only | none | Endpoint URL. It can hold a [reference](#references-in-url-and-args). |
| `headers` | no | empty | HTTP headers for `http`/`sse`. Write a secret as a [reference](#environment-references). |
| `cwd` | no | empty | Working directory for a stdio server, where supported. |
| `timeout` | no | empty | Units vary by target: milliseconds on most. |
| `oauth` | no | empty | OAuth settings. The shape is target-specific; see the target page. |
| `disabled` | no | `false` | See [`disabled` support by target](#disabled-support-by-target). |
| `roots` | no | empty | List of `{uri, name}` objects, for targets that support MCP roots. |

## Environment references

Write a token or key as `${NAME}` in an `env` or `headers` value. The spec then names the variable and never holds the secret. The reference can be the whole value or part of it:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
headers:
  Authorization: Bearer ${API_KEY}
```

`${NAME:-default}` falls back to `default` when `NAME` is unset. Only Claude Code, Crush, OpenHands, and Gemini document that form. Any other `${...}`, such as `${env:NAME}` or `${input:id}`, is not a reference in a spec.

Sync writes each tool's own form. Where a tool cannot read a reference in that field, sync leaves the key out. It prints a note naming the server, the key, and the variable, and never writes the reference as plain text. This applies to a whole target with no form, Amp `env`, a `${NAME:-default}` outside the four tools above, and any other `${...}`. A literal value is written as it is.

| Target | `env` | `headers` |
|--------|-------|-----------|
| [Claude Code](@/docs/targets/claude.md), [Crush](@/docs/targets/crush.md), [OpenHands](@/docs/targets/openhands.md), [Gemini](@/docs/targets/gemini.md) | `${NAME}`, `${NAME:-default}` | `${NAME}`, `${NAME:-default}` |
| [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | `${NAME}` | `${NAME}` |
| [Amp](@/docs/targets/amp.md) | Left out | `${NAME}` |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Codex](@/docs/targets/codex.md) | `KEY: ${KEY}` adds `KEY` to `env_vars`. Any other reference is left out | `Authorization: Bearer ${NAME}` sets `bearer_token_env_var`. Another header of exactly `${NAME}` joins `env_http_headers`. Any other reference is left out |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}` on stdio servers | `{% raw %}${{ secrets.NAME }}{% endraw %}` under `requestOptions.headers` |
| [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Left out | Left out |

Kiro expands only the variables approved under **Mcp Approved Env Vars** in its settings. Factory fails the connection when a referenced variable is unset, and Claude Code passes the unexpanded `${NAME}` text to the server.

[`agnostic-ai doctor`](@/docs/cli-reference/check.md#doctor) lists each referenced variable that is unset in your shell, in top-level fields and in the `x-<target>` block of each selected target. It can list a variable that an override replaces. A tool started from a desktop launcher may see a different environment, and Continue also reads `.env` files, so treat the list as a hint.

Continue's IDE extensions read secrets from project `.env`, `.continue/.env`, or `~/.continue/.env` files. Its CLI also reads process environment variables. See [Continue's secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables). Keep `.env` files out of Git.

### References in `url` and `args`

A `${NAME}` can also sit in `url` or in an `args` element, such as a host that differs per machine or a token a server takes on its command line:

```yaml
url: https://${API_HOST}/mcp
args: [--token, "${GH_TOKEN}"]
```

Sync writes each tool's own form. A tool that reads no reference in that field gets no server at all, because dropping one argument would change the command. The note names the server, the field, and the variable. Sync checks only the field the tool writes for the server's transport, after any `x-<target>` override that tool applies. A `${NAME:-default}` follows the same rule outside Claude Code, Crush, OpenHands, and Gemini.

A tool's own form at the top level, such as `${env:NAME}`, is copied as text to every tool that does not read that form. `agnostic-ai lint` reports it as LINT028. Write `${NAME}`, or put the native text under `x-<target>:`.

`${workspaceFolder}`, `${workspaceFolderBasename}`, `${userHome}`, and `${pathSeparator}` are tool variables, not environment references, and stay as written. So does any other `${...}`, such as `${input:id}`, and every literal URL or argument.

| Target | `url` | `args` |
|--------|-------|--------|
| [Claude Code](@/docs/targets/claude.md), [Crush](@/docs/targets/crush.md), [OpenHands](@/docs/targets/openhands.md), [Gemini](@/docs/targets/gemini.md) | `${NAME}`, `${NAME:-default}` | `${NAME}`, `${NAME:-default}` |
| [Amp](@/docs/targets/amp.md) | `${NAME}` | Server left out |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}` | `{% raw %}${{ secrets.NAME }}{% endraw %}` |
| [Codex](@/docs/targets/codex.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Server left out | Server left out |

Import reads each tool's form in `url` and `args` back as `${NAME}`, including a whole-argument `$NAME` on Gemini and Crush. A tool reference to a variable named like one of the four tool variables, such as Cursor's `${env:workspaceFolder}`, is kept as written, so it never becomes the tool variable. Import never turns a literal URL or argument into a reference.

### Literal `${NAME}` text {#literal-text}

Some servers expand `${NAME}` themselves, such as `mcp-remote` in a header argument. Write `$${NAME}` to pass the text through unchanged. It works in `url`, `args`, `env`, and `headers`:

```yaml
command: npx
args: ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"]
```

Sync writes `${X_CHROMA_TOKEN}` to every built-in tool: no tool form, no left-out server, no note. An external adapter receives the spec as written, `$${NAME}` included. Quote it inside a `[...]` list, as you would `${NAME}`. Only `$${` is special. Any other `$$` is written as it is.

A tool that expands `${NAME}` in that field fills in the value from its own environment before the server sees it. These are Claude Code, Crush, OpenHands, and Gemini in `url` and `args`, Amp in `url`, and the tools with `${NAME}` in the [`env` and `headers` table](#environment-references). None of them documents an escape for a literal `${`, so sync still writes the server. The tool reads its own environment, not the server's `env` block. The server gets the same value unless its `env` sets that variable to something else. A value expanded in `args` shows in the process list, as it does for a `${NAME}` reference. When the variable is unset and has no default, Claude Code keeps the `${NAME}` text and warns in `claude mcp list`. In a remote server's `url` and `headers`, some credential variables read as empty instead ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp#environment-variable-expansion-in-mcp-json)).

When sync leaves out a reference a tool cannot read, the note names the `$${NAME}` spelling to use instead.

### What import writes

`import` reads each tool's own form back as `${NAME}`, including a whole-value `$NAME` on Gemini and Crush and `%NAME%` on Gemini. A `${NAME}` in a field the tool never expands is text, so the spec gets [`$${NAME}`](#literal-text) and the tool's file keeps `${NAME}`, as in Warp, Zed, and Codex `args`. A `$${NAME}` in a field whose form is `${NAME}`, such as Gemini `args`, imports as `$$${NAME}`, so sync writes the same text back. Gemini reads it as `$` plus the value. A `${NAME}` in a Codex `env` table is text too, so it no longer imports as a variable Codex forwards; `env_vars` still does. Import also replaces every literal `env` and `headers` value with a reference, because it cannot tell a token from a plain setting, and a spec is meant to be committed. A plain setting such as `NODE_ENV: production` becomes `NODE_ENV: ${NODE_ENV}`, a variable you must now set. Factory fails the connection without it. Put a plain setting back by hand when it is not a secret.

- An `env` value reads the variable its key names.
- A header reads `<SERVER>_<HEADER>` in upper case, with any character other than a letter or digit as `_`, and keeps a `Bearer ` prefix outside the reference.
- When two different values would share a name, or the import already references that name, each one reads `<SERVER>_<KEY>` instead, then `_2`, `_3` if that still clashes. Equal values share one name. Codex forwards a variable only under its key's own name. A renamed `env` value such as `API_KEY: ${GH_API_KEY}` is left out of `.codex/config.toml` with a note. Rename the variable to the key when Codex needs that server.
- A `${NAME:-default}` loses its default, since a default is a value too.
- A value with text around its references, such as `postgres://u:pw@${HOST}/db` or `Bearer sk-1 ${EXTRA}`, is replaced whole, since that text may be the secret. So is a value with any other `${...}`, such as a VS Code `${input:id}` prompt, which sync could not write. The output names the prompt.
- A Crush `$(command)` value becomes a reference, and the output names the command it ran.

Import prints each replacement and the variable to set. `import --global` keeps literal values, because the user files it adopts must render back unchanged.

## Target-only fields

These fields apply only to the listed targets. Other targets ignore them.

| Target | Extra fields |
|--------|--------------|
| [Codex](@/docs/targets/codex.md) | `env_vars`, `env_http_headers`, `http_headers_helper`, `auth`, `required`, `startup_timeout_sec`, `startup_timeout_ms`, `tool_timeout_sec`, `default_tools_approval_mode`, `scopes`, `oauth_resource`, `experimental_environment`, `enabled_tools`, `disabled_tools` |
| [Crush](@/docs/targets/crush.md) | `enabled_tools`, `disabled_tools`, `sessionless` |
| [Gemini](@/docs/targets/gemini.md) | `trust`, `includeTools`, `excludeTools` |
| [Qoder](@/docs/targets/qoder.md) | `trust`, `includeTools`, `excludeTools`, `alwaysAllow` |
| [Kiro](@/docs/targets/kiro.md) | `autoApprove`, `disabledTools`, `oauthScopes` |
| [Factory](@/docs/targets/factory.md) | `disabledTools`, `connectTimeout` |
| [Claude Code](@/docs/targets/claude.md) | `alwaysLoad`, `bareElicitationCapability`, `headersHelper` |
| [Cursor](@/docs/targets/cursor.md) | `envFile`, `auth` |
| [Copilot / VS Code](@/docs/targets/copilot.md) | `envFile`, `dev`, `sandboxEnabled` (VS Code file only), `tools` (Copilot CLI files only) |
| [Continue](@/docs/targets/continue.md) | `connectionTimeout`, `requestOptions` |
| [OpenHands](@/docs/targets/openhands.md) | `auth: oauth`, or a truthy `oauth`, which sets `auth: "oauth"` on a remote server in `~/.openhands/mcp.json` |

On Amp, set `x-amp.includeTools`. Use `x-factory`, `x-kilo`, or `x-continue` to override the matching top-level options for that target.

Claude Code's `alwaysLoad` and `bareElicitationCapability` preserve explicit `true` and `false` through import and sync. Omit a flag to keep Claude Code's default. Set them at the top level or under `x-claude`, where they override the matching top-level value. Other targets omit both flags. See [Claude Code MCP behavior](@/docs/targets/claude.md#output) for tool loading and connection compatibility.

```yaml
x-claude:
  alwaysLoad: false
  bareElicitationCapability: true
```

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
