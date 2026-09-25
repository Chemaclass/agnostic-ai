+++
title = "Continue"
description = "How agnostic-ai emits Continue configuration: native paths, capability limits, and output options."
weight = 90

[extra]
group = "Reference"
target_id = "continue"
+++

# Continue (`continue`)

## Output

```
.continue/rules/<name>.md
.continue/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.continue/mcpServers/<name>.yaml       # one per MCP entry
.continue/assistants/<name>.yaml       # one per agent, only when assistants-dir is set
```

- **Rule activation**: scoped rules emit `globs` limited to the directory and omit `alwaysApply`, so file matching controls inclusion. Unscoped rules keep their `globs`, `alwaysApply`, and `description`. `x-continue.regex` works only without `scope`. See [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: one folder per skill at `.continue/skills/<name>/SKILL.md`, with bundled files copied byte-for-byte. Continue reads this tree and `.claude/skills/` in both the IDE extension and `cn`. It requires `name` and `description` frontmatter and offers every other file in the folder to the model on demand ([`loadMarkdownSkills.ts`](https://github.com/continuedev/continue/blob/main/core/config/markdown/loadMarkdownSkills.ts)). It does not merge duplicate names across the two trees, so with the `claude` target enabled each skill appears twice. Sync removes managed `.continue/rules/skill-<name>.md` files left by older versions.
- **MCP**: each file under `.continue/mcpServers/` is a Continue block: `name`, `version`, and `schema: v1`, with the server nested under an `mcpServers:` list (a flat single-server file does not load). Stdio emits `command`/`args`/`env`. Remote servers emit `type`/`url`/`requestOptions` with no `env`, since Continue accepts `env` only on stdio; a remote `env` is dropped with a coverage note.
  - `type: http` becomes `type: streamable-http`, the only Streamable HTTP value [Continue's schema](https://github.com/continuedev/continue/blob/main/packages/config-yaml/src/schemas/mcp/index.ts) accepts (`sse` and `streamable-http` pass unchanged). `headers` nests as `requestOptions.headers`. `import continue` reverses both, so specs stay portable.
  - Two kinds of entry emit no file, each with a coverage note: `type: ws` (Continue has no websocket transport), and an entry missing its required field (`command` on stdio, `url` on remote). Either would make Continue fail the whole file.
- **Assistants**: when `outputs.continue.assistants-dir` is set, each agent also emits `<dir>/<name>.yaml` with `name`, `version` (default `0.0.1`), and `schema: v1`, and the body as one `prompts: [{name, description, prompt}]` entry. This matches Continue's schema ([reference](https://docs.continue.dev/reference)).
  - Continue is not confirmed to scan this directory: its [config docs](https://docs.continue.dev/guides/understanding-configs) describe one global `~/.continue/config.yaml` and no project-level agent directory.
  - Each file is a valid standalone `config.yaml`, so load it explicitly with `cn --config <dir>/<name>.yaml` or the IDE's config picker. Models and rules are omitted so user defaults apply. The rule form (`.continue/rules/agent-<name>.md`) is written either way.

Stdio MCP servers keep `cwd`; every transport keeps `connectionTimeout` (milliseconds). Remote `requestOptions` keeps `timeout`, `verifySsl`, `caBundlePath`, `proxy`, `clientCertificate`, and `headers`. Portable `headers` fills `requestOptions.headers`, and an explicit native header wins a duplicate key. `x-continue` overrides each matching top-level option.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.continue.rules-dir` | `.continue/rules` | One `.md` per rule and agent. |
| `outputs.continue.skills-dir` | `.continue/skills` | |
| `outputs.continue.mcp-dir` | `.continue/mcpServers` | |
| `outputs.continue.assistants-dir` | empty | opt-in |

## Import

`agnostic-ai import continue` reads rules from `.continue/rules/` and reclassifies each file by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification). Native `globs` and `regex` strings or arrays survive through `x-continue`, including empty arrays and patterns with commas.

Source subdirectories still impose directory scope. Nested rules keep one or more array patterns when they stay inside that scope. Conflicting selectors, scoped `regex`, and scoped empty `globs` arrays are reported as unsupported rather than widened to the whole directory. Keep such rules in root-level source files.

Skills import from `.continue/skills/` with their bundled files. A `skill-<name>.md` rule from an older sync still imports as a skill.

MCP servers import from `.continue/mcpServers/`:

- `*.yaml` blocks with exactly one server.
- `.json` files parsed as JSONC, holding an `mcpServers` map or a bare server named after the file.

Duplicate or unsafe server names fail before any MCP spec is written. Connection options survive import and sync, and import reverses the Continue spellings described under **MCP**.

## Verify

1. Install the [Continue extension](https://marketplace.visualstudio.com/items?itemName=Continue.continue) in VS Code or JetBrains.
2. Check the tree: `ls .continue/rules/ .continue/skills/ .continue/mcpServers/`, `grep "Generated by agnostic-ai" .continue/rules/*.md .continue/mcpServers/*.yaml` for the provenance header, and `python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in __import__('glob').glob('.continue/mcpServers/*.yaml')]"`.
3. Open the project. Each `.continue/rules/*.md` appears in the rules picker with no "failed to parse" warning. A rule with `globs` is active only while a matching file is in context.
4. The MCP picker shows each `.continue/mcpServers/<name>.yaml` green.
5. If `outputs.continue.assistants-dir` is set, each `<dir>/<name>.yaml` parses (`python -c "import yaml; yaml.safe_load(open('<dir>/<name>.yaml'))"`). Load one with `cn --config <dir>/<name>.yaml` to confirm Continue accepts it.
