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

- **Declared once.** Change a server in one file. Every tool picks it up on the next sync.
- **Local or remote.** A `stdio` server runs a command on your machine. `http`, `sse`, and `ws` servers connect to a URL.
- **Tool options kept.** Timeouts, tool filters, OAuth, and approval settings go only to the tools that read them.
- **Limited per agent.** An [agent's `mcpServers`](@/docs/spec-format/agents.md#mcpservers-support-by-target) limits which servers one subagent may reach.

## Write one

`agnostic-ai new mcp filesystem` creates `mcps/filesystem.yaml`: plain YAML, no markdown body.

```yaml
name: filesystem
description: Local filesystem access for the model.
type: stdio
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
env:
  ROOT: !literal /tmp
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

Real servers to copy are on the [MCP recipes](@/docs/spec-format/mcp-recipes.md) page.

## Fields

`name` is the server identifier, not the filename. It may contain package-style slashes, such as `npm:@modelcontextprotocol/server-sequential.thinking`. Such names are percent-encoded in filenames and written as they are in every generated config.

A server needs `command` (stdio) or `url` (remote). Only `agnostic-ai lint` reports a missing one (LINT008), so run it before `sync`. See [lint](@/docs/cli-reference/check.md#lint).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | none | Server identifier and key in the generated config. |
| `description` | no | empty | Free-form notes. Dropped for Junie and Warp. |
| `type` | no | `stdio` | `stdio`, `http`, `sse`, or `ws`. Remote types write an explicit `type`; `stdio` does not. Augment, Factory, and Qoder get no server for `ws`. |
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

## Secrets and plain settings {#plain-settings}

A value in `env` or `headers` is a reference by default: `${NAME}`, `${NAME:-default}`, or `$${NAME}` text. A header may put one word before it, as in `Bearer ${API_KEY}`. Mark a plain setting that is not a secret with `!literal`:

```yaml
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  NODE_ENV: !literal production
```

Sync writes `NODE_ENV: production` to every tool. An editor with the YAML language server reports an unknown tag until you add `!literal scalar` to its `yaml.customTags` setting.

`agnostic-ai lint` fails on any other value (LINT035), and `sync` refuses to write it unless the spec comes from a [pack](@/docs/packs.md). The message names the server, the field, and the key, never the value. A value with text around a reference, such as `postgres://u:pw@${HOST}/db`, fails too. Empty values, numbers, booleans, `x-<target>` blocks, `url`, and `args` are not checked.

[`agnostic-ai migrate --only secrets`](@/docs/cli-reference/maintain.md#migrate) rewrites existing specs. A value that looks like a [credential](#what-import-writes) becomes a reference, such as `GITHUB_TOKEN: ${GITHUB_TOKEN}`, and the command lists each variable to set, never the value. Every other value gets `!literal`. It leaves two cases to you: a key with a [credential name](#credential-names) whose value does not look like a credential, such as `API_KEY: sk-live-abc`, and a credential around a reference.

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
| [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | `${NAME}` | `${NAME}` |
| [Amp](@/docs/targets/amp.md) | Left out | `${NAME}` |
| [Cursor](@/docs/targets/cursor.md), [Windsurf](@/docs/targets/windsurf.md) | `${env:NAME}` | `${env:NAME}` |
| [OpenCode](@/docs/targets/opencode.md) | `{env:NAME}` | `{env:NAME}` |
| [Codex](@/docs/targets/codex.md) | `KEY: ${KEY}` adds `KEY` to `env_vars`. Any other reference is left out | `Authorization: Bearer ${NAME}` sets `bearer_token_env_var`. Another header of exactly `${NAME}` joins `env_http_headers`. Any other reference is left out |
| [Continue](@/docs/targets/continue.md) | `{% raw %}${{ secrets.NAME }}{% endraw %}` on stdio servers | `{% raw %}${{ secrets.NAME }}{% endraw %}` under `requestOptions.headers` |
| [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Left out | Left out |

Kiro expands only variables approved under **Mcp Approved Env Vars** in its settings. Factory fails the connection when a referenced variable is unset.

[`agnostic-ai doctor`](@/docs/cli-reference/check.md#doctor) lists each referenced variable that is unset in your shell, including in `x-<target>` blocks and disabled servers. A tool started from a desktop launcher may see a different environment, and Continue also reads `.env` files, so treat the list as a hint.

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
| [Codex](@/docs/targets/codex.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Antigravity](@/docs/targets/antigravity.md), [Augment](@/docs/targets/augment.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md), [Kilo Code](@/docs/targets/kilo.md), [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md), [Zed](@/docs/targets/zed.md) | Server left out | Server left out |

### Literal `${NAME}` text {#literal-text}

Some servers expand `${NAME}` themselves, such as `mcp-remote` in a header argument. Write `$${NAME}` to pass the text on unchanged. It works in `url`, `args`, `env`, and `headers`:

```yaml
command: npx
args: ["-y", "mcp-remote", "https://mcp.example.com", "--header", "x-chroma-token: $${X_CHROMA_TOKEN}"]
```

Sync writes `${X_CHROMA_TOKEN}` to every built-in tool, with no left-out server and no note. An external adapter gets the spec as written. Quote it inside a `[...]` list, as you would `${NAME}`.

A tool that expands `${NAME}` in that field fills in the value from its own environment before the server sees it: Claude Code, Crush, OpenHands, and Gemini in `url` and `args`, Amp in `url`, and the tools with `${NAME}` in the [`env` and `headers` table](#environment-references). None of them has a way to write a literal `${`, so sync still writes the server. The tool reads its own environment, not the server's `env` block. A value expanded in `args` shows in the process list. When the variable is unset and has no default, Claude Code keeps the `${NAME}` text and warns in `claude mcp list`.

### What import writes

`import` reads each tool's own form in `env`, `headers`, `url`, and `args` back as `${NAME}`, including a whole-value `$NAME` on Gemini and Crush and `%NAME%` on Gemini. A `${NAME}` in a field the tool never expands is plain text, so the spec gets [`$${NAME}`](#literal-text), as in Warp, Zed, and Codex `args`. Import also replaces every literal `env` and `headers` value with a reference, because it cannot tell a token from a plain setting and a spec is meant to be committed. A plain setting such as `NODE_ENV: production` becomes `NODE_ENV: ${NODE_ENV}`, a variable you must now set. Put a plain setting back by hand when it is not a secret.

- An `env` value reads the variable its key names.
- A header reads `<SERVER>_<HEADER>` in upper case, with any character other than a letter or digit as `_`, and keeps a `Bearer ` prefix outside the reference.
- When two different values would share a name, each one reads `<SERVER>_<KEY>` instead, then `_2`, `_3`. Codex forwards a variable only under its key's own name, so a renamed `env` value such as `API_KEY: ${GH_API_KEY}` is left out of `.codex/config.toml` with a note. Rename the variable to match the key when Codex needs that server.
- A `${NAME:-default}` loses its default, since a default is a value too.
- A value with text around its references, such as `postgres://u:pw@${HOST}/db` or `Bearer sk-1 ${EXTRA}`, is replaced whole, since that text may be the secret. So is a value with any other `${...}`, such as a VS Code `${input:id}` prompt.
- A Crush `$(command)` value becomes a reference.
- When a `url` or argument is exactly one URL, only its credentials become references: a `user:password@` password, a username that is a token by the prefixes below (`https://ghp_...@github.com`), and the value of a query or fragment parameter with a [credential name](#credential-names), such as `api_key`, `access_token`, or `clientSecret` (not `token_type`, `key_id`, or `sortKey`). A parameter named exactly `auth`, `sig`, `signature`, `code`, `session`, `jwt`, or `bearer` counts too, but `country_code` does not. Each reads `<SERVER>_<NAME>`, so `postgresql://admin:pw@db/app` in server `pg` becomes `postgresql://admin:${PG_PASSWORD}@db/app`. The rest of the URL stays as written, including a leading reference such as `${API_BASE}/mcp?token=...`. The output names the field and variable, never the URL.
- One URL means no spaces, quotes, or `$${`, and it starts with a scheme or a `${NAME}`. Import never rewrites any other value, such as a `sh -c` command or `--url=...`.
- In `args`, the value of a long flag with a [credential name](#credential-names) becomes a reference named after the flag, as `--api-key value` or `--api-key=value`. In server `gh`, `--api-key X` becomes `--api-key ${GH_API_KEY}`. A flag such as `--token-file`, `--key-id`, `--sort-key`, or `--no-token` is not a credential. These values stay as written: a `$NAME`, a file path (starting with `/`, `~`, `./`, `../`, or a drive such as `C:\`), a file name ending in `.json`, `.txt`, `.pem`, `.key`, `.env`, `.yaml`, `.yml`, `.toml`, `.crt`, `.p12`, or `.db`, a value starting with `@`, a number, a boolean word, and lower-case words joined by dashes such as `streamable-http`. After a flag whose name ends in `key`, `apikey`, `authentication`, or `authorization`, a value needs at least 8 characters, so `--require-api-key run` stays as written.
- An argument that is a whole token by its prefix becomes `${<SERVER>_TOKEN}`, then `_2`, `_3`. The prefixes are `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`, `glpat-`, `xoxb-`, `xoxp-`, `xoxa-`, `xapp-`, `sk_live_`, `sk_test_`, `rk_live_`, `sk-proj-`, `sk-ant-`, `npm_`, `AIza`, `sk-`, and `AKIA`, each with a plausible length. A JWT (`eyJ...` with three dot-separated parts) counts too.
- Import leaves the server out and names the server and field when a value it does not rewrite still holds a literal credential. A credential here is any of these:
  - a URL password or credential parameter (a quoted value such as `token='...'` counts, a quoted `'${NAME}'` does not);
  - a token by its prefix inside a longer value;
  - a `Bearer` with a literal token that has a digit, one of `-._~+/=`, or at least 20 characters;
  - a line `Name: value` or a JSON pair `"name": "value"` with a credential key and a literal value, as in a headers value `X-Api-Key: ...` or `{"apiKey": "..."}`;
  - a `--header` or `-H` argument such as `Authorization: ...`, `Cookie: ...`, or `X-Api-Key: ...` with a literal value;
  - a `user:password@host` without a scheme, as in `root:pw@db:3306`;
  - a `NAME=value` or `--name value` with a credential name inside a longer value, as in `sh -c "API_TOKEN=... exec srv"` or `sh -c "npx srv --token ..."`;
  - an all-digit password before `#`, or before `/` with no host, as in `redis://:1234#5@host`.
- Import never rewrites `command`, `cwd`, Continue `requestOptions` keys other than `headers`, or an `x-<target>` block. A credential in any of them, or a key there with a credential name and a literal value, leaves the server out.
- Import does not find these credentials, so check such servers by hand:
  - a short flag such as `-p secret` or `-psecret`, since `-p` may be a port or profile, or a single-dash flag such as `-token secret`;
  - a credential value that starts with `-` in its own argument after a flag;
  - a value shorter than 8 characters in its own argument after a flag whose name ends in `key`, `apikey`, `authentication`, or `authorization`;
  - a `Bearer` token shorter than 20 characters with only letters;
  - a token in a URL path, such as a Slack webhook `https://hooks.slack.com/services/...`, also under `import --global`;
  - a URL username that is a token without one of the prefixes above;
  - a token format not listed above.
- A query or fragment parameter named exactly `auth` or `code` always becomes a reference, even without a secret.

Import prints each replacement and the variable to set. `import --global` cannot change a server, so it leaves out each server with a literal credential and names the server and field, never the value. That is any credential above, and an `env` or header value with a [credential name](#credential-names) or that looks like a credential. A plain setting such as `NODE_ENV: production`, a path, a number, or `Bearer ${TOKEN}` still imports as written, marked [`!literal`](#plain-settings). A value under a [credential name](#credential-names) that does not look like a credential stays unmarked, so `lint` asks about it. Add a left-out server by hand under `local/mcps`, or write it with a `${NAME}` reference.

#### Credential names

A name is a credential name when its last word is `token`, `secret`, `password`, `passwd`, `pwd`, `pass`, `accesstoken`, `authtoken`, `credential`, `credentials`, `cookie`, `bearer`, `apikey`, `authentication`, or `authorization`. Words split at punctuation and camelCase, so `clientSecret` and `API_TOKEN` count and `token_type` does not. A one-word name ending in `password`, `passwd`, `secret`, or `token` counts too, as `PGPASSWORD` and `MYTOKEN` do.

A last word `key` counts unless the word before it is `sort`, `cache`, `public`, `partition`, `primary`, `foreign`, `idempotency`, `routing`, `object`, `row`, `hash`, `map`, `lookup`, `group`, `dedup`, `shard`, `index`, `unique`, `composite`, `natural`, `surrogate`, `ssh`, `gpg`, `pgp`, `s3`, or `id`. So `api_key`, `OPENAI_KEY`, and `--openai-key` count, and `sort_key`, `cache_key`, `--public-key`, and `--ssh-key` do not.

In an `env` or header key, an `x-<target>` or `requestOptions` key, and a `Name: value` line or JSON pair, a last word `cred` or `auth` counts too, as in `MY_CRED` and `X_AUTH`. A flag such as `--auth oauth` does not.

## Target-only fields

These fields apply only to the listed tools.

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

Claude Code's `alwaysLoad` and `bareElicitationCapability` keep an explicit `true` or `false`. Omit a flag to keep Claude Code's default. Set them at the top level or under `x-claude`, which wins. Other tools leave out both flags. See [Claude Code MCP behavior](@/docs/targets/claude.md#output).

```yaml
x-claude:
  alwaysLoad: false
  bareElicitationCapability: true
```

## `disabled` support by target

| Target | Behavior |
|--------|----------|
| [Antigravity](@/docs/targets/antigravity.md), [Crush](@/docs/targets/crush.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md), [Qoder](@/docs/targets/qoder.md), [Windsurf](@/docs/targets/windsurf.md) | Native `disabled` |
| [Codex](@/docs/targets/codex.md) | Mapped to `enabled = false` |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md), [Zed](@/docs/targets/zed.md) | Mapped to `"enabled": false` |
| [Copilot](@/docs/targets/copilot.md) | A `disabledMcpServers` entry in `.github/copilot/settings.json` for Copilot CLI. Removed from both MCP files with a note. Disable the server in VS Code for that half |
| [Claude Code](@/docs/targets/claude.md) | Mapped to project `disabledMcpjsonServers` in `.claude/settings.json` for servers written to `.mcp.json`. Import restores it |
| [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Junie](@/docs/targets/junie.md), [Trae](@/docs/targets/trae.md), [Warp](@/docs/targets/warp.md) | Removed with a note. Disable the server in the tool itself |
