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

- **Rule activation**: scoped rules emit `globs` constrained to the directory and omit `alwaysApply`, allowing file matching to control inclusion. Unscoped rules retain their configured `globs`, `alwaysApply`, and `description`; `x-continue.regex` is available only without `scope`. See [scoped selector limits](@/docs/scoped-context.md#narrow-a-rule-to-certain-files).
- **Skills**: one native folder per skill at `.continue/skills/<name>/SKILL.md`, with bundled sibling files copied byte-for-byte. Continue's skill loader reads that tree (and `.claude/skills/`) in both the IDE extension and `cn`, requires `name` and `description` frontmatter, and lists every other file in the folder to the model as a supporting file to read on demand. It does not merge duplicate names across the two trees, so with the `claude` target also enabled each skill appears twice. The docs site has no skills page yet; the source is [`loadMarkdownSkills.ts`](https://github.com/continuedev/continue/blob/main/core/config/markdown/loadMarkdownSkills.ts). A prior version flattened each skill into `.continue/rules/skill-<name>.md`, which dropped bundled files and broke every relative link to them (#1043); sync removes a managed file of that shape.
- **MCP**: each YAML under `.continue/mcpServers/` is a Continue block file: a `name` + `version` + `schema: v1` wrapper with the server nested under an `mcpServers:` list (a flat single-server file does not load). Stdio emits `command`/`args`/`env`. Remote servers emit `type`/`url`/`requestOptions`, with no `env`: Continue declares that field on its stdio server only, so a remote entry that sets one drops it and raises a coverage note rather than writing a key the schema strips.
  - The spelling is Continue's rather than the spec's: `type: http` lands as `type: streamable-http`, the only Streamable HTTP literal [Continue's schema](https://github.com/continuedev/continue/blob/main/packages/config-yaml/src/schemas/mcp/index.ts) accepts (`sse` and `streamable-http` carry over unchanged). A `headers` map nests as `requestOptions.headers`, the only place that branch reads headers from. `import continue` undoes both, so a spec that round-trips through Continue stays portable.
  - Two kinds of entry emit no file at all, each with a coverage note: `type: ws`, because Continue documents no websocket transport, and an entry missing the field its branch requires (`command` on stdio, `url` on a remote server), which trae, warp, antigravity and windsurf decline too. Both would match neither branch of the union and make `blockSchema.parse` throw, failing the whole file rather than skipping the entry.
- **Assistants**: when `outputs.continue.assistants-dir` is set, each agent also emits as a YAML file at `<dir>/<name>.yaml`: `name`, `version` (`0.0.1` by default), and `schema: v1` at the top level, with the agent body wrapped as a single `prompts: [{name, description, prompt}]` entry.
  - That shape matches Continue's own schema exactly (`promptSchema` and `configYamlSchema` in `continuedev/continue`'s `packages/config-yaml/src/schemas/index.ts`; [docs.continue.dev/reference](https://docs.continue.dev/reference) documents the same `name`/`version`/`schema: v1` top level). Confirmed 2026-08-08, after the prior citation, `/hub/assistants/intro`, 404d and the whole `/hub/` doc namespace turned out to be gone (#563).
  - What is not confirmed: that Continue itself scans `outputs.continue.assistants-dir` as a directory of assistants. [`understanding-configs.mdx`](https://docs.continue.dev/guides/understanding-configs) describes Local Configuration as one global `~/.continue/config.yaml`, with no project-scoped per-agent directory anywhere in the current docs.
  - Since the emitted file is a self-contained, valid `config.yaml`, point Continue at it explicitly instead: `cn --config <dir>/<name>.yaml`, or the IDE's config picker. Models and rules are omitted so user defaults apply. The rule-form emission (`.continue/rules/agent-<name>.md`) still happens either way.

Stdio MCP servers preserve `cwd`; every transport preserves `connectionTimeout` (milliseconds). Remote `requestOptions` preserves `timeout`, `verifySsl`, `caBundlePath`, `proxy`, `clientCertificate`, and `headers`. Portable `headers` fills `requestOptions.headers`; an explicit native header wins a duplicate key. `x-continue` overrides each corresponding top-level option.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.continue.rules-dir` | `.continue/rules` | One `.md` per rule and agent. |
| `outputs.continue.skills-dir` | `.continue/skills` | |
| `outputs.continue.mcp-dir` | `.continue/mcpServers` | |
| `outputs.continue.assistants-dir` | empty | opt-in |

## Import

Native `globs` and `regex` strings or arrays survive import through `x-continue`, including empty arrays and patterns containing commas.

Canonical source subdirectories still impose directory scope. Nested rules retain one or several array patterns when those patterns stay inside that scope. Conflicting selectors, scoped `regex`, and scoped empty `globs` arrays report unsupported instead of replacing their meaning with a directory-wide filter. Keep these native conditions in root-level source files when they cannot express a directory scope.

`agnostic-ai import continue` reads rules from `.continue/rules/` and reclassifies each file by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification). Skills import from `.continue/skills/` with their bundled files; a `skill-<name>.md` rule left by an older sync still imports as a skill.

MCP servers import from `.continue/mcpServers/`:

- `*.yaml` blocks containing exactly one server.
- `.json` files parsed as JSONC. A JSON file can hold an `mcpServers` map or a bare server named after its filename.

Duplicate or unsafe server names across files fail before MCP specs are written. Connection options survive import and sync, and import undoes the Continue spellings described under **MCP**.

## Verify

1. Install the [Continue extension](https://marketplace.visualstudio.com/items?itemName=Continue.continue) in VS Code (or JetBrains).
2. Check the tree: `ls .continue/rules/ .continue/skills/ .continue/mcpServers/`, `grep "Generated by agnostic-ai" .continue/rules/*.md .continue/mcpServers/*.yaml` for the provenance header, `python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in __import__('glob').glob('.continue/mcpServers/*.yaml')]"`.
3. Open the project. Continue loads every `.continue/rules/*.md`; each appears in the rules picker with no "failed to parse" warnings. A rule with `globs` shows as active only while a matching file is in context.
4. The MCP picker shows each `.continue/mcpServers/<name>.yaml` green.
5. If `outputs.continue.assistants-dir` is set, each `<dir>/<name>.yaml` parses as a valid `config.yaml` (`python -c "import yaml; yaml.safe_load(open('<dir>/<name>.yaml'))"`). Native directory discovery is unconfirmed, so load one explicitly with `cn --config <dir>/<name>.yaml` to confirm Continue accepts it.
