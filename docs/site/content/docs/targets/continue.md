+++
title = "Continue"
description = "How agnostic-ai emits Continue configuration: native paths, capability limits, and output options."
weight = 90

[extra]
group = "Reference"
target_id = "continue"
+++

# Continue (`continue`)

agnostic-ai writes Continue rules, skills, and MCP servers under `.continue/`, plus optional assistant configs.

## Output

```
.continue/rules/<name>.md
.continue/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

- **Rule activation**: scoped rules emit the union of the scope directory and their `globs` or `paths`, without `alwaysApply`. Unscoped rules keep `globs`, `alwaysApply`, and `description`. A comma-separated `globs` string becomes an array, since Continue reads a string as one pattern. `x-continue.regex` works only without `scope`. See [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: `.continue/skills/<name>/SKILL.md` and bundled files copy byte-for-byte. Continue reads this tree and `.claude/skills/` in the IDE extension and `cn`. It requires `name` and `description` and offers the other files on demand ([`loadMarkdownSkills.ts`](https://github.com/continuedev/continue/blob/main/core/config/markdown/loadMarkdownSkills.ts)). It does not merge names across the two trees, so with the `claude` target each skill appears twice.
- **MCP**: each file is a Continue block (`name`, `version`, `schema: v1`) nesting the server under an `mcpServers:` list. A flat single-server file does not load.
  - Stdio emits `command`/`args`/`env`/`cwd`. Remote emits `type`/`url`/`requestOptions` and drops `env` (stdio only) with a coverage note.
  - `type: http` becomes `type: streamable-http`, the only Streamable HTTP value [Continue's schema](https://github.com/continuedev/continue/blob/main/packages/config-yaml/src/schemas/mcp/index.ts) accepts. `sse` and `streamable-http` pass unchanged. `headers` nests as `requestOptions.headers`. `import continue` reverses both.
  - Every transport keeps `connectionTimeout` (milliseconds). Remote `requestOptions` keeps `timeout`, `verifySsl`, `caBundlePath`, `proxy`, `clientCertificate`, and `headers`. An explicit native header wins a duplicate key. `x-continue` overrides each matching top-level option.
  - `type: ws` (no websocket transport) and an entry missing `command` (stdio) or `url` (remote) emit no file, with a coverage note. Either would make Continue fail the whole file.
- **Assistants**: with `outputs.continue.assistants-dir` set, each agent also emits `<dir>/<name>.yaml`: `name`, `version` (default `0.0.1`), `schema: v1`, and the body as one `prompts: [{name, description, prompt}]` entry, per Continue's schema ([reference](https://docs.continue.dev/reference)).
  - Continue is not confirmed to scan this directory. Its [config docs](https://docs.continue.dev/guides/understanding-configs) describe one global `~/.continue/config.yaml` and no project-level agent directory.
  - Each file is a standalone `config.yaml` for `cn --config <dir>/<name>.yaml` or the IDE's config picker. It omits models and rules, so user defaults apply.
  - The rule form (`.continue/rules/agent-<name>.md`) is written either way.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.continue.rules-dir` | `.continue/rules` | One `.md` per rule and agent. |
| `outputs.continue.skills-dir` | `.continue/skills` | |
| `outputs.continue.mcp-dir` | `.continue/mcpServers` | |
| `outputs.continue.assistants-dir` | empty | opt-in |

## Import

`agnostic-ai import continue` reads `.continue/rules/` and reclassifies each file by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification). Native `globs` and `regex` strings or arrays survive through `x-continue`, including empty arrays and patterns with commas.

- **Scope**: source subdirectories that name project directories imply scope. Scope and patterns form a union. Scoped `regex` and scoped empty `globs` arrays are reported as unsupported. For a narrow file filter only, omit `scope` and keep the source file outside a folder that implies scope.
- **Native activation**: `x-continue.globs` and `x-continue.regex` keep exact activation without a portable `scope`. A nested native rule keeps its folder for placement, even one naming no project directory. An explicit `scope` opts into the union and its selector limits.
- **Skills**: from `.continue/skills/` with bundled files.
- **MCP**: from `.continue/mcpServers/`: `*.yaml` blocks with one server, and `.json` files parsed as JSONC with an `mcpServers` map or a bare server named after the file. Duplicate or unsafe names fail before any spec is written. Connection options round-trip.

MCP import replaces literal `env` and header values with `${NAME}` references and names the variables to set. Continue's `{% raw %}${{ secrets.NAME }}{% endraw %}` comes back as `${NAME}`. Sync writes Continue's secret syntax in stdio `env` and `args`, remote `url` and `requestOptions.headers`, including `x-continue` overrides. The IDE reads project `.env`, `.continue/.env`, or `~/.continue/.env` files; the CLI also reads process environment variables. See [Continue's secrets guide](https://docs.continue.dev/guides/configuring-models-rules-tools#working-with-secrets) and [secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables). Keep those files out of Git.

{% <details summary="Skill rules from older versions"> %}
Sync removes managed `.continue/rules/skill-<name>.md` files from older versions. Import still reads a `skill-<name>.md` rule as a skill.
{% </details> %}

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the [Continue extension](https://marketplace.visualstudio.com/items?itemName=Continue.continue) in VS Code or JetBrains.
2. Check the tree: `ls .continue/rules/ .continue/skills/ .continue/mcpServers/`, the provenance header with `grep "Generated by agnostic-ai" .continue/rules/*.md .continue/mcpServers/*.yaml`, and `python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in __import__('glob').glob('.continue/mcpServers/*.yaml')]"`.
3. Open the project. Each `.continue/rules/*.md` shows in the rules picker without a "failed to parse" warning. A rule with `globs` is active only while a matching file is in context.
4. The MCP picker shows each `.continue/mcpServers/<name>.yaml` green.
5. With `outputs.continue.assistants-dir` set, each `<dir>/<name>.yaml` parses (`python -c "import yaml; yaml.safe_load(open('<dir>/<name>.yaml'))"`), and `cn --config <dir>/<name>.yaml` accepts it.
