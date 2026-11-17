+++
title = "Continue native reference"
description = "Native field mappings, secrets, and import coverage for continue."
weight = 91

[extra]
nav_title = "Continue reference"
group = "Reference"
+++

# Continue native reference

Start with the [Continue setup](@/docs/targets/continue.md). Output keys are listed under [output settings](@/docs/targets/continue.md#output).

## Native mappings

### Rule activation

- A scoped rule matches its scope directory plus its `globs` or `paths`, and gets no `alwaysApply`. See [file-pattern limits for directory rules](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- A rule with no directory scope keeps `globs`, `alwaysApply`, and `description`.
- A comma-separated `globs` string becomes an array, because Continue reads a string as one pattern.
- `x-continue.regex` works only without `scope`.

### Skills

- Written to `.continue/skills/<name>/SKILL.md`. Bundled files are copied unchanged.
- The IDE extension and `cn` read this folder and `.claude/skills/`. They do not merge names across the two, so with the `claude` target each skill shows up twice.
- Continue requires `name` and `description` and offers the other files on demand ([`loadMarkdownSkills.ts`](https://github.com/continuedev/continue/blob/main/core/config/markdown/loadMarkdownSkills.ts)).
- A skill has only `name` and `description`, and every skill is offered to the model. None can be kept user-only, so `disable-model-invocation` is dropped with a note and commands are skipped. `invokable` belongs to prompts, not skills.

### MCP

- Each file is a Continue block (`name`, `version`, `schema: v1`) with the server in an `mcpServers:` list. A flat single-server file does not load.
- Stdio gets `command`, `args`, `env`, `cwd`. Remote gets `type`, `url`, `requestOptions`. Remote `env` is dropped with a note.
- `type: http` becomes `type: streamable-http`, the only Streamable HTTP value [Continue's schema](https://github.com/continuedev/continue/blob/main/packages/config-yaml/src/schemas/mcp/index.ts) accepts. `sse` and `streamable-http` stay as they are.
- `headers` moves to `requestOptions.headers`. `import continue` reverses both changes.
- Every transport keeps `connectionTimeout` (milliseconds).
- Remote `requestOptions` keeps `timeout`, `verifySsl`, `caBundlePath`, `proxy`, `clientCertificate`, `headers`.
- An explicit native header wins a duplicate key. `x-continue` overrides each matching top-level option.
- No file is written, with a note, for `type: ws` (Continue has no websocket transport) or for an entry missing `command` (stdio) or `url` (remote). Either would make Continue reject the whole file.

### Assistants

- With `outputs.continue.assistants-dir` set, each agent also gets `<dir>/<name>.yaml`: `name`, `version` (default `0.0.1`), `schema: v1`, and the body as one `prompts: [{name, description, prompt}]` entry ([Continue reference](https://docs.continue.dev/reference)).
- Continue's source loads YAML from `.continue/assistants`, `.continue/agents`, and `.continue/configs` at the workspace root as local profiles ([`ConfigHandler.ts`](https://github.com/continuedev/continue/blob/5522c6f/core/config/ConfigHandler.ts)). A folder set here with one of those paths loads as a local profile. Continue's docs do not describe a workspace folder, so this rests on the source only.
- Each file is also a standalone `config.yaml` for `cn --config <dir>/<name>.yaml` or the IDE's config picker. It has no models or rules, so your defaults apply.
- The rule form (`.continue/rules/agent-<name>.md`) is written either way.

## Import

`agnostic-ai import continue` reads `.continue/rules/` and sorts each file into a spec kind by [filename prefix](@/docs/cli-reference/import.md#filename-prefix-reclassification).

- Native `globs` and `regex` values (strings or arrays, empty arrays and patterns with commas included) are kept under `x-continue`.
- Scope: a subfolder named after a project directory sets the rule's scope, so the rule matches that directory plus its patterns.
- A scoped `regex` or scoped empty `globs` array is reported as unsupported. To filter by file only, omit `scope` and keep the source file outside any folder that sets scope.
- Native activation: `x-continue.globs` and `x-continue.regex` keep activation exactly, as long as there is no portable `scope`. A rule in a subfolder stays there, even when the folder names no project directory. An explicit `scope` switches to scope plus patterns, with its selector limits.
- Skills: read from `.continue/skills/` with bundled files.
- MCP: read from `.continue/mcpServers/`. That means `*.yaml` blocks with one server, and `.json` files (read as JSONC) with an `mcpServers` map or one server named after the file.
- A duplicate or unsafe MCP name fails before any spec is written. Connection options survive import and sync.
- Sync removes `.continue/rules/skill-<name>.md` files that older versions wrote. Import still reads such a rule as a skill.

### Secrets

- MCP import replaces literal `env` and header values with `${NAME}` references and tells you which variables to set.
- Continue's `{% raw %}${{ secrets.NAME }}{% endraw %}` comes back as `${NAME}`.
- Sync writes Continue's secret syntax in stdio `env` and `args` and in remote `url` and `requestOptions.headers`, `x-continue` overrides included.
- The IDE reads secrets from a project `.env`, `.continue/.env`, or `~/.continue/.env`. The CLI also reads process environment variables. See [Continue's secrets guide](https://docs.continue.dev/guides/configuring-models-rules-tools#working-with-secrets) and [secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables).
- Keep those files out of Git.
