+++
title = "Continue"
description = "What agnostic-ai writes for Continue: file paths, what it supports, and output settings."
weight = 90

[extra]
group = "Reference"
target_id = "continue"

[extra.moved]
set-up-this-tool = "@/docs/targets/continue.md#first-rule"
+++

# Continue (`continue`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

agnostic-ai writes Continue rules, skills, and MCP servers under `.continue/`, plus optional assistant files.

## First rule

Install the client using the [official setup guide](https://marketplace.visualstudio.com/items?itemName=Continue.continue), then open your project in it.

Select `continue` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.continue/rules/conventional-commits.md`.

Agents become rule content by default. Optional assistant files are standalone configs, not automatically discovered subagents.

For existing files, check [what `import continue` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `.continue/rules/conventional-commits.md` for the commit-prefix rule. Open the project in Continue. Find the rule in the rules picker, then ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
.continue/rules/<name>.md
.continue/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>


- **Rule activation**: a scoped rule matches its scope directory plus its `globs` or `paths`, and gets no `alwaysApply`. A rule with no directory scope keeps `globs`, `alwaysApply`, and `description`. A comma-separated `globs` string becomes an array, because Continue reads a string as one pattern. `x-continue.regex` works only without `scope`. See [file-pattern limits for directory rules](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: `.continue/skills/<name>/SKILL.md` and bundled files are copied unchanged. The IDE extension and `cn` read this folder and `.claude/skills/`. Continue requires `name` and `description`, and offers the other files on demand ([`loadMarkdownSkills.ts`](https://github.com/continuedev/continue/blob/main/core/config/markdown/loadMarkdownSkills.ts)). It does not merge names across the two folders, so with the `claude` target each skill shows up twice.
  - A skill has only `name` and `description`, and every skill is offered to the model, so none can be kept user-only. `disable-model-invocation` is dropped with a note, and commands are skipped. `invokable` belongs to prompts, not skills.
- **MCP**: each file is a Continue block (`name`, `version`, `schema: v1`) with the server under an `mcpServers:` list. A flat single-server file does not load.
  - Stdio servers get `command`/`args`/`env`/`cwd`. Remote servers get `type`/`url`/`requestOptions`, and `env` (stdio only) is dropped with a sync note.
  - `type: http` becomes `type: streamable-http`, the only Streamable HTTP value [Continue's schema](https://github.com/continuedev/continue/blob/main/packages/config-yaml/src/schemas/mcp/index.ts) accepts. `sse` and `streamable-http` are unchanged. `headers` moves to `requestOptions.headers`. `import continue` reverses both.
  - Every transport keeps `connectionTimeout` (milliseconds). Remote `requestOptions` keeps `timeout`, `verifySsl`, `caBundlePath`, `proxy`, `clientCertificate`, and `headers`. An explicit native header wins a duplicate key. `x-continue` overrides each matching top-level option.
  - No file is written, with a sync note, for `type: ws` (Continue has no websocket transport) or for an entry missing `command` (stdio) or `url` (remote). Either would make Continue reject the whole file.
- **Assistants**: with `outputs.continue.assistants-dir` set, each agent also gets `<dir>/<name>.yaml` with `name`, `version` (default `0.0.1`), `schema: v1`, and the body as one `prompts: [{name, description, prompt}]` entry, per Continue's schema ([reference](https://docs.continue.dev/reference)).
  - Continue may not scan this directory. Its [config docs](https://docs.continue.dev/guides/understanding-configs) describe one global `~/.continue/config.yaml` and no project-level agent directory.
  - Each file is a standalone `config.yaml` for `cn --config <dir>/<name>.yaml` or the IDE's config picker. It has no models or rules, so your defaults apply.
  - The rule form (`.continue/rules/agent-<name>.md`) is written either way.

</details>

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.continue.rules-dir` | `.continue/rules` | One `.md` per rule and agent. |
| `outputs.continue.skills-dir` | `.continue/skills` | |
| `outputs.continue.mcp-dir` | `.continue/mcpServers` | |
| `outputs.continue.assistants-dir` | empty | opt-in |

## Import

`agnostic-ai import continue` reads `.continue/rules/` and sorts each file into a spec kind by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification). Native `globs` and `regex` values (strings or arrays, including empty arrays and patterns with commas) are kept under `x-continue`.

- **Scope**: a subfolder named after a project directory sets the rule's scope. The rule then matches the scope directory plus its patterns. A scoped `regex` or scoped empty `globs` array is reported as unsupported. To filter by file only, omit `scope` and keep the source file outside any folder that sets scope.
- **Native activation**: `x-continue.globs` and `x-continue.regex` keep the rule's activation exactly, as long as there is no portable `scope`. A rule in a subfolder stays in that folder, even when the folder names no project directory. An explicit `scope` switches to scope plus patterns, with its selector limits.
- **Skills**: from `.continue/skills/` with bundled files.
- **MCP**: from `.continue/mcpServers/`: `*.yaml` blocks with one server, and `.json` files (read as JSONC) with an `mcpServers` map or a single server named after the file. A duplicate or unsafe name fails before any spec is written. Connection options survive import and sync.

MCP import replaces literal `env` and header values with `${NAME}` references and tells you which variables to set. Continue's `{% raw %}${{ secrets.NAME }}{% endraw %}` comes back as `${NAME}`. Sync writes Continue's secret syntax in stdio `env` and `args` and in remote `url` and `requestOptions.headers`, including `x-continue` overrides. The IDE reads secrets from a project `.env`, `.continue/.env`, or `~/.continue/.env` file. The CLI also reads process environment variables. See [Continue's secrets guide](https://docs.continue.dev/guides/configuring-models-rules-tools#working-with-secrets) and [secret resolution](https://docs.continue.dev/faqs#managing-local-secrets-and-environment-variables). Keep those files out of Git.

<details>
<summary>Skill rules from older versions</summary>

Sync removes the `.continue/rules/skill-<name>.md` files it wrote in older versions. Import still reads a `skill-<name>.md` rule as a skill.

</details>

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
