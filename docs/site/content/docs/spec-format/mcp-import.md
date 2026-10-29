+++
title = "MCP import credentials"
description = "How import rewrites credential values and which servers it leaves out."
weight = 66

[extra]
nav_title = "MCP credentials"
group = "Reference"
+++

# MCP import credentials

## What import writes

`import` replaces every literal `env` and `headers` value with a reference, because it cannot tell a token from a plain setting and a spec is meant to be committed. So `NODE_ENV: production` becomes `NODE_ENV: ${NODE_ENV}`, a variable you must now set; mark it [`!literal`](@/docs/spec-format/mcps.md#plain-settings) by hand to put it back. Import prints each replacement and the variable to set.

It reads each tool's own reference form in `env`, `headers`, `url`, and `args` back as `${NAME}`, including a whole-value `$NAME` on Gemini and Crush and `%NAME%` on Gemini. A `${NAME}` in a field the tool never expands, as in Warp, Zed, and Codex `args`, is plain text, so the spec gets [`$${NAME}`](@/docs/spec-format/mcps.md#literal-text).

- An `env` value reads the variable its key names.
- A header reads `<SERVER>_<HEADER>` in upper case, with any character other than a letter or digit as `_`, and keeps a `Bearer ` prefix outside the reference.
- When two different values would share a name, each reads `<SERVER>_<KEY>` instead, then `_2`, `_3`. Codex forwards a variable only under its key's own name, so a renamed `env` value such as `API_KEY: ${GH_API_KEY}` is left out of `.codex/config.toml` with a note; rename the variable to match the key when Codex needs that server.
- A `${NAME:-default}` loses its default, since a default is a value too.
- A value with text around its references, such as `postgres://u:pw@${HOST}/db` or `Bearer sk-1 ${EXTRA}`, is replaced whole, since that text may be the secret, and so is a value with any other `${...}`, such as a VS Code `${input:id}` prompt.
- A Crush `$(command)` value becomes a reference.
- When a `url` or argument is exactly one URL (no spaces, quotes, or `$${`, starting with a scheme or a `${NAME}`), only its credentials become references, each named `<SERVER>_<NAME>`: a `user:password@` password, a username that is a token by the prefixes below (`https://ghp_...@github.com`), and a query or fragment parameter with a [credential name](#credential-names), such as `api_key`, `access_token`, or `clientSecret` (not `token_type`, `key_id`, or `sortKey`). A parameter named exactly `auth`, `sig`, `signature`, `code`, `session`, `jwt`, or `bearer` counts too (not `country_code`), and one named `auth` or `code` always becomes a reference, even without a secret. So `postgresql://admin:pw@db/app` in server `pg` becomes `postgresql://admin:${PG_PASSWORD}@db/app`. The rest of the URL stays as written, including a leading reference such as `${API_BASE}/mcp?token=...`, and the output never prints the URL. Import never rewrites another value, such as a `sh -c` command or `--url=...`.
- In `args`, the value of a long flag with a [credential name](#credential-names), as `--api-key value` or `--api-key=value`, becomes a reference named after the flag: in server `gh`, `--api-key X` becomes `--api-key ${GH_API_KEY}`. `--token-file`, `--key-id`, `--sort-key`, and `--no-token` are not credentials. These values stay as written: a `$NAME`, a file path (starting with `/`, `~`, `./`, `../`, or a drive such as `C:\`), a file name ending in `.json`, `.txt`, `.pem`, `.key`, `.env`, `.yaml`, `.yml`, `.toml`, `.crt`, `.p12`, or `.db`, a value starting with `@`, a number, a boolean word, and lower-case words joined by dashes such as `streamable-http`. After a flag whose name ends in `key`, `apikey`, `authentication`, or `authorization`, a value needs at least 8 characters, so `--require-api-key run` stays as written.
- An argument that is a whole token by its prefix becomes `${<SERVER>_TOKEN}`, then `_2`, `_3`. The prefixes are `ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`, `glpat-`, `xoxb-`, `xoxp-`, `xoxa-`, `xapp-`, `sk_live_`, `sk_test_`, `rk_live_`, `sk-proj-`, `sk-ant-`, `npm_`, `AIza`, `sk-`, and `AKIA`, each with a plausible length. A JWT (`eyJ...` with three dot-separated parts) counts too.
- When a value it does not rewrite still holds a literal credential, import leaves the server out and names the server and field. A credential here is any of these:
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

`import --global` cannot change a server, so it leaves out each server with a literal credential: any credential above, or an `env` or header value with a [credential name](#credential-names) or that looks like a credential. It names the server and field, never the value. A plain setting such as `NODE_ENV: production`, a path, a number, or `Bearer ${TOKEN}` imports as written, marked [`!literal`](@/docs/spec-format/mcps.md#plain-settings). A value under a credential name that does not look like a credential stays unmarked, so `lint` asks about it. Add a left-out server by hand under `local/mcps`, or with a `${NAME}` reference.

## Setting values

Imports of other settings use one rule for each value: Claude Code `env` values, hook headers, and `mcp_tool` inputs, Codex overlay values, and Grok hook and agent metadata. A value holds a credential when it has literal text and either the text looks like a credential by the rules above, or its key has a [credential name](#credential-names) and the value is not a boolean (`true`, `false`, `yes`, `no`, `on`, `off`, `0`, `1`), a path, or a file name.

- Literal text is anything other than `${NAME}` and `$NAME` references, with an optional `Bearer`, `Basic`, or `Token` before them. Under a key that ends in `authorization`, such as `Proxy-Authorization`, any one word may come first, as in `SSWS ${OKTA_TOKEN}`. A `${NAME:-default}` with a default and an escaped `$${NAME}` count as literal.
- A path starts with `~/`, `./`, `../`, a drive such as `C:\`, or `/` with at least two segments, such as `/etc/x`. A value that reads as base64 with slashes, such as `/9j/4AAQSkZJRg...`, is not a path: a segment of 16 or more letters and digits with a digit, or 24 or more in both letter cases, or 20 or more letters and digits in all, in both letter cases, with no `.`, `-`, `_`, or `~`. A file name has no path and ends in an extension listed above. Neither may hold a token.

So `API_TOKEN: 12345678`, `API_TOKEN: hunter $X`, `API_TOKEN: ${API_TOKEN:-x}`, `PASSWORD: ~hunter2`, and `Authorization: Bearer abc` hold one. `API_TOKEN: ${API_TOKEN}`, `API_TOKEN: $A $B`, `Authorization: ApiKey ${ES_KEY}`, `CLAUDE_CODE_SKIP_BEDROCK_AUTH: 1`, `GOOGLE_APPLICATION_CREDENTIALS: /path/sa.json`, and `CLIENT_KEY: client.key` do not.

Claude Code import leaves out such an `env` value with a warning, and the whole matcher group of a hook with such a header or input. It also leaves out any literal value of its fixed token variables, such as `ANTHROPIC_API_KEY`, and header variables, such as `OTEL_EXPORTER_OTLP_HEADERS` (see [overlays](@/docs/spec-format/overlays.md)). Codex import leaves out such a value with a warning; keys that start with `env_` and prose keys such as `instructions` use their own checks, as the [Codex reference](@/docs/targets/codex-reference.md) says. Grok import stops before writing.

## Credential names

A name is a credential name when its last word is `token`, `secret`, `password`, `passwd`, `passphrase`, `pwd`, `pass`, `accesstoken`, `authtoken`, `credential`, `credentials`, `cookie`, `bearer`, `apikey`, `authentication`, or `authorization`. Words split at punctuation and camelCase, so `clientSecret` and `API_TOKEN` count and `token_type` does not; `pass_phrase` counts as `passphrase`. A one-word name ending in `password`, `passwd`, `passphrase`, `secret`, or `token` counts too, as `PGPASSWORD` and `MYTOKEN` do.

A last word `key` counts unless the word before it is `sort`, `cache`, `public`, `partition`, `primary`, `foreign`, `idempotency`, `routing`, `object`, `row`, `hash`, `map`, `lookup`, `group`, `dedup`, `shard`, `index`, `unique`, `composite`, `natural`, `surrogate`, `ssh`, `gpg`, `pgp`, `s3`, or `id`. So `api_key`, `OPENAI_KEY`, and `--openai-key` count, and `sort_key`, `cache_key`, `--public-key`, and `--ssh-key` do not.

In an `env` or header key, an `x-<target>` or `requestOptions` key, and a `Name: value` line or JSON pair, a last word `cred` or `auth` counts too, as in `MY_CRED` and `X_AUTH`. A flag such as `--auth oauth` does not.
