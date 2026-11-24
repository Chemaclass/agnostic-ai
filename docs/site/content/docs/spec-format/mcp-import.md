+++
title = "MCP import credentials"
description = "How import rewrites credential values and which servers it leaves out."
weight = 66

[extra]
nav_title = "MCP credentials"
group = "Reference"
+++

# MCP import credentials

`import` replaces MCP credentials with `${NAME}` references and skips some servers. The [Tools page's **Import** action](@/docs/cli-reference/serve.md#serve) applies the same rules to one server. This includes the user-level page.

## What import writes

Import cannot tell a token from a plain setting in a spec that will be committed. It replaces every literal `env` and `headers` value with a `${NAME}` reference and prints each replacement and the variable to set.

- `NODE_ENV: production` becomes `NODE_ENV: ${NODE_ENV}`, a variable you must now set. Mark it [`!literal`](@/docs/spec-format/mcps.md#plain-settings) by hand to put it back.
- A tool's own reference form in `env`, `headers`, `url`, and `args` is read back as `${NAME}`.
- A `${NAME}` in a field the tool never expands becomes [`$${NAME}`](@/docs/spec-format/mcps.md#literal-text).
- A `${NAME}` in a field the tool expands stays a reference. Sync writes a spec's `$${NAME}` there as `${NAME}`, since no tool can hold a literal `${`, so import puts the `$${NAME}` back when the spec it replaces held it. The [per-tool table](@/docs/spec-format/mcps-reference.md#environment-references-by-target) shows which fields each tool expands.
- A server that still holds a literal credential after the rewrite is left out. Import names the server and field.

{% <details summary="Details: native reference forms and fields a tool never expands"> %}
- Native forms read back include a whole-value `$NAME` on Gemini and Crush and `%NAME%` on Gemini.
- Fields a tool never expands: Warp, Zed, and Codex `args`. A `${NAME}` there is plain text.
{% </details> %}

<details>
<summary>How variables are named</summary>

- An `env` value reads the variable its key names.
- A header reads `<SERVER>_<HEADER>` in upper case, with any character other than a letter or digit as `_`. A `Bearer ` prefix stays outside the reference.
- When two different values would share a name, each reads `<SERVER>_<KEY>` instead, then `_2`, `_3`.
- Codex forwards a variable only under its key's own name. A renamed `env` value such as `API_KEY: ${GH_API_KEY}` is left out of `.codex/config.toml` with a note. Rename the variable to match the key when Codex needs that server.
- A `${NAME:-default}` loses its default, since a default is a value too.
- A value with text around its references, such as `postgres://u:pw@${HOST}/db` or `Bearer sk-1 ${EXTRA}`, is replaced whole, since the text may be the secret. So is a value with any other `${...}`, such as a VS Code `${input:id}` prompt.
- A Crush `$(command)` value becomes a reference.

</details>

<details>
<summary>URLs and arguments</summary>

A `url` or argument that is exactly one URL (no spaces, quotes, or `$${`; starts with a scheme or a `${NAME}`) gets only its credentials replaced, each named `<SERVER>_<NAME>`:

- a `user:password@` password;
- a username that is a token by the prefixes below (`https://ghp_...@github.com`);
- a query or fragment parameter with a [credential name](#credential-names), such as `api_key`, `access_token`, or `clientSecret` (not `token_type`, `key_id`, or `sortKey`);
- a parameter named exactly `auth`, `sig`, `signature`, `code`, `session`, `jwt`, or `bearer` (not `country_code`). `auth` and `code` always become references, even without a secret.

Example: `postgresql://admin:pw@db/app` in server `pg` becomes `postgresql://admin:${PG_PASSWORD}@db/app`. The rest of the URL stays, including a leading reference such as `${API_BASE}/mcp?token=...`. Output never prints the URL. Import never rewrites another value, such as a `sh -c` command or `--url=...`.

In `args`, the value of a long flag with a credential name (`--api-key value` or `--api-key=value`) becomes a reference named after the flag. In server `gh`, `--api-key X` becomes `--api-key ${GH_API_KEY}`.

- `--token-file`, `--key-id`, `--sort-key`, and `--no-token` are not credentials.
- These values stay as written: a `$NAME`; a file path (starting with `/`, `~`, `./`, `../`, or a drive such as `C:\`); a file name ending in `.json`, `.txt`, `.pem`, `.key`, `.env`, `.yaml`, `.yml`, `.toml`, `.crt`, `.p12`, or `.db`; a value starting with `@`; a number; a boolean word; lower-case words joined by dashes, such as `streamable-http`.
- After a flag whose name ends in `key`, `apikey`, `authentication`, or `authorization`, a value needs at least 8 characters. So `--require-api-key run` stays as written.

An argument that is a whole token by its prefix becomes `${<SERVER>_TOKEN}`, then `_2`, `_3`. Each prefix needs a plausible length:

`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`, `glpat-`, `xoxb-`, `xoxp-`, `xoxa-`, `xapp-`, `sk_live_`, `sk_test_`, `rk_live_`, `sk-proj-`, `sk-ant-`, `npm_`, `AIza`, `sk-`, `AKIA`. A JWT (`eyJ...` with three dot-separated parts) counts too.

</details>

<details>
<summary>What leaves a server out</summary>

When a value import does not rewrite still holds a literal credential, import leaves the server out. A credential is any of these:

- a URL password or credential parameter (a quoted `token='...'` counts; a quoted `'${NAME}'` does not);
- a token by its prefix inside a longer value;
- a `Bearer` with a literal token that has a digit, one of `-._~+/=`, or at least 20 characters;
- a line `Name: value` or a JSON pair `"name": "value"` with a credential key and a literal value, such as `X-Api-Key: ...` or `{"apiKey": "..."}`;
- a `--header` or `-H` argument such as `Authorization: ...`, `Cookie: ...`, or `X-Api-Key: ...` with a literal value;
- a `user:password@host` without a scheme, as in `root:pw@db:3306`;
- a `NAME=value` or `--name value` with a credential name inside a longer value, as in `sh -c "API_TOKEN=... exec srv"` or `sh -c "npx srv --token ..."`;
- an all-digit password before `#`, or before `/` with no host, as in `redis://:1234#5@host`.

Import never rewrites `command`, `cwd`, Continue `requestOptions` keys other than `headers`, or an `x-<target>` block. A credential in any of them, or a key there with a credential name and a literal value, leaves the server out.

</details>

<details>
<summary>What import cannot detect</summary>

Check these by hand:

- a short flag such as `-p secret` or `-psecret` (`-p` may be a port or profile), or a single-dash flag such as `-token secret`;
- a credential value that starts with `-` in its own argument after a flag;
- a value shorter than 8 characters in its own argument after a flag whose name ends in `key`, `apikey`, `authentication`, or `authorization`;
- a `Bearer` token shorter than 20 characters with only letters;
- a token in a URL path, such as a Slack webhook `https://hooks.slack.com/services/...`, also under `import --scope user`;
- a URL username that is a token without one of the prefixes above;
- a token format not listed above.

</details>

## User-level import {#global-import}

`import --scope user` cannot change a server, so it leaves out each server with a literal credential: any credential above, or an `env` or header value with a [credential name](#credential-names) or that looks like a credential. It names the server and field, never the value.

- A plain setting such as `NODE_ENV: production`, a path, a number, or `Bearer ${TOKEN}` imports as written, marked [`!literal`](@/docs/spec-format/mcps.md#plain-settings).
- A value under a credential name that does not look like a credential stays unmarked, so `lint` asks about it.
- Add a left-out server by hand under `local/mcps`, or with a `${NAME}` reference.

## Setting values

Other imports use one rule per value: Claude Code `env` values, hook headers, and `mcp_tool` inputs, Codex overlay values, and Grok hook and agent metadata. A value holds a credential when it has literal text and either looks like a credential by the rules above, or has a [credential name](#credential-names) as its key and is not a boolean (`true`, `false`, `yes`, `no`, `on`, `off`, `0`, `1`), a path, or a file name.

| Import | When a value holds a credential |
|--------|---------------------------------|
| Claude Code | Leaves out the `env` value with a warning, and the whole matcher group of a hook with such a header or input. Also leaves out any literal value of its fixed token variables, such as `ANTHROPIC_API_KEY`, and header variables, such as `OTEL_EXPORTER_OTLP_HEADERS` (see [overlays](@/docs/spec-format/overlays.md)). |
| Codex | Leaves out the value with a warning. Keys that start with `env_` and prose keys such as `instructions` use their own checks ([Codex reference](@/docs/targets/codex-reference.md)). |
| Grok | Stops before writing. |

<details>
<summary>Literal text and path rules, with examples</summary>

- Literal text is anything other than `${NAME}` and `$NAME` references, with an optional `Bearer`, `Basic`, or `Token` before them.
- Under a key that ends in `authorization`, such as `Proxy-Authorization`, any one word may come first, as in `SSWS ${OKTA_TOKEN}`.
- A `${NAME:-default}` with a default and an escaped `$${NAME}` count as literal.
- A path starts with `~/`, `./`, `../`, a drive such as `C:\`, or `/` with at least two segments, such as `/etc/x`.
- A value that reads as base64 with slashes, such as `/9j/4AAQSkZJRg...`, is not a path. It has a segment of 16 or more letters and digits with a digit, or 24 or more in both letter cases, or 20 or more letters and digits in all, in both letter cases, with no `.`, `-`, `_`, or `~`.
- A file name has no path and ends in an extension listed above. Neither a path nor a file name may hold a token.

| Holds a credential | Does not |
|---|---|
| `API_TOKEN: 12345678` | `API_TOKEN: ${API_TOKEN}` |
| `API_TOKEN: hunter $X` | `API_TOKEN: $A $B` |
| `API_TOKEN: ${API_TOKEN:-x}` | `Authorization: ApiKey ${ES_KEY}` |
| `PASSWORD: ~hunter2` | `CLAUDE_CODE_SKIP_BEDROCK_AUTH: 1` |
| `Authorization: Bearer abc` | `GOOGLE_APPLICATION_CREDENTIALS: /path/sa.json` |
| | `CLIENT_KEY: client.key` |

</details>

## Credential names

A name is a credential name when its last word is one of: `token`, `secret`, `password`, `passwd`, `passphrase`, `pwd`, `pass`, `accesstoken`, `authtoken`, `credential`, `credentials`, `cookie`, `bearer`, `apikey`, `authentication`, `authorization`.

{% <details summary="Details: how words split, and the key, cred and auth rules"> %}
- Words split at punctuation and camelCase, so `clientSecret` and `API_TOKEN` count and `token_type` does not.
- `pass_phrase` counts as `passphrase`.
- A one-word name ending in `password`, `passwd`, `passphrase`, `secret`, or `token` counts too, as `PGPASSWORD` and `MYTOKEN` do.
- A last word `key` counts unless the word before it is `sort`, `cache`, `public`, `partition`, `primary`, `foreign`, `idempotency`, `routing`, `object`, `row`, `hash`, `map`, `lookup`, `group`, `dedup`, `shard`, `index`, `unique`, `composite`, `natural`, `surrogate`, `ssh`, `gpg`, `pgp`, `s3`, or `id`.
- So `api_key`, `OPENAI_KEY`, and `--openai-key` count. `sort_key`, `cache_key`, `--public-key`, and `--ssh-key` do not.
- In an `env` or header key, an `x-<target>` or `requestOptions` key, and a `Name: value` line or JSON pair, a last word `cred` or `auth` counts too, as in `MY_CRED` and `X_AUTH`. A flag such as `--auth oauth` does not.

{% </details> %}
