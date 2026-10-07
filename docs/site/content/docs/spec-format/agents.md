+++
title = "Agents"
description = "agents/: subagents with their own prompt, tools, and model, written once for every tool."
weight = 10
aliases = ["/docs/spec-format/agent-specs/"]

[extra]
group = "Reference"
+++

# Agents

`agents/` defines subagents: specialists the main agent hands a task to. Each has its own instructions, tools, and model. Examples: a reviewer that only reads, an architect on the strongest model, a test writer on a cheaper one.

Why use a subagent instead of more instructions in the main session:

- **Clean context.** A subagent works in its own context and hands back a result, so its exploration stays out of the main conversation.
- **Least access.** `can`, `readonly`, and `mcpServers` limit what it may touch.
- **The right model per role.** `model` and `effort` set cost and depth per agent, and per tool.
- **One definition.** Every tool that supports agents gets its own file from the same spec. The rest get a coverage note.

## Write one

`agnostic-ai new agent code-reviewer` creates `agents/code-reviewer.md`. The frontmatter configures the agent. The body is its system prompt.

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues. Use after a change set is complete.
can: [read, shell(git diff *)]
model: sonnet
---

You are a code reviewer. Report concise findings with `file:line` references.
```

Write `description` for the main agent. It reads it to decide when to hand off a task, so say what the agent does and when to use it.

This read-only auditor uses a stronger model on Claude Code, another model everywhere else, and one MCP server:

```markdown
---
name: security-auditor
description: Checks a diff for injection, leaked secrets, and unsafe input handling. Use before merging auth or payment changes.
readonly: true
model: {claude: opus, default: gpt-6.1-sol}
mcpServers: [github]
---

List each finding with its `file:line`, the attack it enables, and the smallest fix.
```

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty | When to hand a task to the agent. Tools show it in lists and use it to pick an agent. |
| `can` | no | unset | What the agent may do, in tool-neutral names. See [capabilities](#capabilities). |
| `tools` | no | unset | The same list in Claude Code tool names. A spec sets `can` or `tools`, not both. See [`tools` support by target](#tools-support-by-target). |
| `model` | no | unset | A string for every target, a map per target, or a [tier](#model-tiers) name. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `effort` | no | unset | A string or integer for every target, or a map per target. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `color` | no | unset | Badge color. See [`color` support by target](#color-support-by-target). |
| `readonly` | no | unset | `true` limits the agent to reading; `false` does nothing. See [`readonly` by target](#readonly-by-target). |
| `memory` | no | unset | Persistent memory scope: `user`, `project`, or `local`. |
| `mcpServers` | no | unset | MCP servers this agent may reach. See [`mcpServers` support by target](#mcpservers-support-by-target). |
| `permissionMode` | no | unset | How much this agent may do without approval. See [`permissionMode` and agent `hooks`](#agent-policy-support-by-target). |
| `hooks` | no | unset | Lifecycle hooks for this agent only. See [`permissionMode` and agent `hooks`](#agent-policy-support-by-target). |

Any other frontmatter field is written as given.

`memory` gives the agent a directory that lasts across sessions. Only [Claude Code](@/docs/targets/claude.md#agent-memory) is confirmed to use it. [Qoder](@/docs/targets/qoder.md#subagent-memory) gets the key, unconfirmed. Junie gets it as written. Every other tool drops it.

### `readonly` by target {#readonly-by-target}

| Target | Effect |
|--------|--------|
| Cursor | Restricted |
| Claude Code | `disallowedTools: Write, Edit, NotebookEdit` (Bash stays allowed) |
| Codex | Writes `sandbox_mode = "read-only"`, which current Codex ignores (the agent keeps the session sandbox), and a coverage note |
| Factory | `tools: read-only` with `mcpServers: []` unless servers are listed. Wins over a portable `tools` list |
| Other targets | Coverage note |

An explicit `x-claude.disallowedTools`, `x-codex.sandbox_mode`, or `x-factory.tools` wins.

## Capabilities {#capabilities}

`can` lists what an agent may do, in names that belong to no one tool:

```yaml
can: [read(src/**), edit(src/**), shell(go test *), mcp:github]
```

| `can` value | Grants | Claude Code name |
|-------------|--------|------------------|
| `read`, `read(<path>)` | Read files, or matching files | `Read`, `Read(<path>)` |
| `write` | Create or overwrite files | `Write` |
| `edit`, `edit(<path>)` | Edit files, or matching files | `Edit`, `Edit(<path>)` |
| `delete` | Delete files | No native tool |
| `shell` | Run any command | `Bash` |
| `shell(<pattern>)` | Run the commands that match the pattern | `Bash(<pattern>)` |
| `web` | Fetch pages and search the web | `WebFetch`, `WebSearch` |
| `mcp:<server>` | Use every tool of one MCP server | `mcp__<server>` |
| `mcp:<server>/<tool>` | Use one MCP tool | `mcp__<server>__<tool>` |

- Each capability becomes the tool's own name for it. `delete` raises a coverage note on tools with no delete tool.
- Claude Code names still work as aliases, with no plan to remove them. A list can mix both: `can: [read, Grep]`.
- The names match the hook [`match:` tool kinds](@/docs/spec-format/hooks.md#portable-events).
- A tool that grants more than you asked for prints a note naming the extra access. On Kiro, `edit` also allows `delete_file`. `on-unsupported: error` fails on this, for capabilities and Claude Code aliases alike.
- `validate`, `lint` (LINT036), and `sync` stop on an unknown capability, on `can` beside `tools`, and on `can` under `x-<target>`. A typo never gives an agent every tool.
- A capability is only as strong as the tool's own permission system. agnostic-ai adds no sandbox.

`agnostic-ai migrate --only capabilities` rewrites `tools` as `can`. Each name that has a capability of its own becomes that capability. An adjacent `WebFetch, WebSearch` pair becomes `web`. The rest stay as aliases, and sync writes the same files. `import` writes capabilities when each tool name matches one capability. `lint --suggest-capabilities` suggests neutral names for aliases (LINT037). These suggestions are off by default.

## Per-target `model` and `effort` {#per-target-model-and-effort}

`model:` and `effort:` each take a single value or a map keyed by target name, with an optional `default`. The first match in this list wins:

1. `x-<target>.<key>`
2. `<key>.<target>`
3. `<key>.default`
4. The key is not written and the tool uses its own default.

`x-<target>.<key>: null` deletes the key. A value under a target key that is not a single value falls back to `default`.

| Want | Write |
|------|-------|
| Same model everywhere | `model: gpt-6.1-sol` |
| Per target, with a default | `model: {claude: sonnet, default: gpt-5.6-terra}` |
| Per target, tool default elsewhere | `model: {claude: sonnet}` |
| Different effort per target | `effort: {claude: xhigh, default: high}` |

```yaml
---
name: architect
description: Designs the change before anyone codes it.
model:
  claude: opus
  cursor: "claude-opus-5[effort=high]"
  default: gpt-6.1-sol
effort:
  claude: xhigh
  qoder: 8000
  factory: max
  default: high
x-codex:
  model_reasoning_effort: xhigh
---
```

Result:

- Claude gets `opus` and `xhigh`.
- Qoder gets `gpt-6.1-sol` and `8000`.
- Junie gets `gpt-6.1-sol` and `high`.
- Cursor gets `claude-opus-5[effort=high]`. The effort is not written.
- Codex gets `gpt-6.1-sol`, with `x-codex` overriding effort to `xhigh`.
- Factory gets `gpt-6.1-sol` and no `reasoningEffort`, because `max` is not one of its values (coverage note).
- Trae drops both, with notes.

**`effort` values by target.** Only the listed tools were checked. Without `effort`, the agent uses the session's level.

| Target | Values | How it lands |
|--------|--------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent | Written as `effort`, not checked |
| [Qoder](@/docs/targets/qoder.md) | Same five names, or a positive integer | Written as `effort` |
| [Junie](@/docs/targets/junie.md) | Alias of `reasoningLevel` | Written as `effort` |
| [Factory](@/docs/targets/factory.md) | `low`, `medium`, `high` | `reasoningEffort`. Other values and integers raise a coverage note |
| [Codex](@/docs/targets/codex.md) | Any string | `model_reasoning_effort`; `x-codex.model_reasoning_effort` wins. An integer raises a note |
| [Cursor](@/docs/targets/cursor.md) | none | Coverage note. Put it in the model id: `model: {cursor: "claude-opus-5[effort=high]"}` |
| [Copilot](@/docs/targets/copilot.md) | none | Coverage note. `x-copilot.effort` still passes through |
| Every other target | none | Coverage note |

Cursor takes effort in the `model` string, so set it through the `model` map. Factory ignores `reasoningEffort` when `model` is `inherit`.

**Claude model names on other targets.** A shared `model` (a single value or `default`) set to a Claude model name raises a coverage note on a tool that cannot load it. The note names `model: {claude: <name>}`. Sync leaves the value out, so that tool uses its own default.

- A map keeps its other entries.
- `on-unsupported: error` fails the sync instead.
- A Claude name in a later settings spec no longer hides the model from an earlier one.
- A value under `model.<target>` or `x-<target>.model` is written as given.
- When the name comes from a tier's `default`, the note names the tier to fix.
- `import claude` writes these names as `model: {claude: <name>}`.
- `import codex` adds a Codex agent model to an existing spec as `model.codex`.

Claude Code's [aliases](https://code.claude.com/docs/en/model-config) are `sonnet`, `opus`, `haiku`, `fable`, `best`, `opusplan`, `sonnet[1m]`, and `opus[1m]`. The model value `default` resets Claude's model instead of naming one, so it raises no note.

| Target | Claude names that raise the note |
|--------|----------------------------------|
| [Codex](@/docs/targets/codex.md), [Gemini](@/docs/targets/gemini.md), [OpenCode](@/docs/targets/opencode.md), [Kilo Code](@/docs/targets/kilo.md) | The aliases, `inherit`, and `claude-*` ids |
| [Cursor](@/docs/targets/cursor.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | The aliases |

Only the listed tools were checked.

## Model tiers {#model-tiers}

Model ids belong to one vendor, so the same per-target map repeats in every agent. Name the roles once under [`models`](@/docs/configuration.md#models) in `agnostic-ai.yaml`, then write the tier name in each agent:

```yaml
# agnostic-ai.yaml
models:
  strong: {claude: opus, codex: gpt-6.1-sol, effort: {claude: xhigh, codex: high}}
```

```yaml
---
name: architect
description: Designs the change before anyone codes it.
model: strong
---
```

Claude gets `opus` with `xhigh`. Codex gets `gpt-6.1-sol` with `high`. Every other target uses its own default. Skills, commands, and settings specs name tiers the same way.

For `model`, the first match in this list wins:

1. `x-<target>.model`
2. `model.<target>` in the spec
3. The tier's entry for the target
4. The tier's `default`
5. The tool default

To override one target, write the tier as the map's `default`: `model: {codex: gpt-6-luna, default: strong}`.

- The tier's `effort` applies only when the spec sets no `effort`. A spec `effort` replaces it.
- The tier's `effort` is also skipped for a target whose model the spec sets itself, since that effort was chosen for the tier's model.
- A value under `model.<target>` or `x-<target>.model` is never a tier name.
- A `model.<target>` value can be a [vendor alias](@/docs/configuration.md#models) such as `codex: sol`. `x-<target>.model` is written as given.

`explain agents/architect.md` lists the model and effort each configured tool gets.

{% <details summary="Lint and import with tiers"> %}
`lint` flags:

- a tier a spec names with no entry and no `default` for one of the spec's targets
- a tier named like a Claude model (LINT025)
- a Claude model name in a shared `model` or a tier `default` that reaches another vendor's target (LINT026)

`import claude` suggests a tier when two or more agents set the same Claude model. `import claude` and `import codex` keep `model: strong` when the imported model and effort match what the tier gives that tool. A sync followed by an import then does not write a fixed model.
{% </details> %}

## Agents as skills {#agents-as-skills}

Amp, Crush, Warp, and Zed have no subagents, so sync drops agents there by default. Set `outputs.<target>.agents: skill` to write each agent as a skill instead. A skill loads only when the model needs it, so the agent costs no context until then.

```yaml
outputs:
  amp:
    agents: skill
```

- The skill is `<skills-dir>/<agent>/SKILL.md`, with the agent's `name` and `description`.
- Its body starts with a short note: the tool has no subagents, so the model plays the role in the main session, follows only these instructions, and says in one line that it did. Nothing enforces that separation.
- Fields a skill cannot carry, such as `tools` and `model`, are dropped with a coverage note.
- In skills, agents, and commands, an [agent reference](@/docs/spec-format/_index.md#agent-and-skill-references) becomes that tool's skill phrase. A rule in a shared entry-point file such as `AGENTS.md` keeps the neutral phrase.
- An agent and a skill with the same name would share a folder, so `validate` fails on them.
- These four tools write `.agents/skills/`. Codex, Copilot, Gemini, Cline, Cursor, OpenCode, and Junie read it too, and several other tools with subagents write there. When one of them is enabled, sync keeps the agents out of that directory and names the tool, so it does not get the role twice. Set `outputs.<target>.skills-dir` to a separate directory to use this option beside them.
- The key cannot be combined with `rules-file` or, on Warp, `workflows-dir`, which already carry the agents.

Other targets reject the key.

## `can` and `tools` support by target {#tools-support-by-target}

Only the listed tools were checked. Sync reads `can` as the `tools` it stands for. A tool that cannot honor the list prints a coverage note at sync time, so `can: [read]` never becomes an unrestricted agent without warning.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Written as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Written as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md), [Kilo Code](@/docs/targets/kilo.md) | Changed to the tool's own names or a permission map |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md) | Dropped with a note |

The change can widen access: on Kiro, `edit` also permits `delete_file`. Sync prints a note naming the extra access, and `on-unsupported: error` fails. `explain agents/<name>.md` lists each configured tool's own names and any widening. Most tools accept their own names through `x-<target>.tools`, which skips the change.

## `mcpServers` support by target {#mcpservers-support-by-target}

Only the listed tools were checked. A top-level `mcpServers` list limits which MCP servers one agent may reach. Without it, the agent gets the session's full set.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Junie](@/docs/targets/junie.md), [Factory](@/docs/targets/factory.md) | Server names, written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Server names or inline objects, written to the agent file |
| [OpenHands](@/docs/targets/openhands.md) | Inline definitions only. Set `x-openhands.mcp_servers` |
| [Antigravity](@/docs/targets/antigravity.md) | Inline objects only. Set `x-antigravity.mcpServers` |
| [Kiro](@/docs/targets/kiro.md) | Inline definitions only. Set `x-kiro.mcpServers` |

Every other tool drops the list with a coverage note. On the three inline tools, the note names the `x-<target>` key to set.

**An empty list does not mean the same everywhere.** Junie treats `mcpServers: []` as keeping every configured server. Factory treats it as excluding every server. List the servers you want instead.

## `permissionMode` and agent `hooks` support by target {#agent-policy-support-by-target}

Only the listed tools were checked. `permissionMode` sets how much one agent may do without approval. `hooks` sets lifecycle hooks for that agent only. Without either, the agent uses the main session's setting.

| Target | `permissionMode` | Agent `hooks` |
|--------|------------------|---------------|
| [Claude Code](@/docs/targets/claude.md) | Seven values; `manual` is an alias of `default` | Written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Six values. Any other value is reported | Seven events. A wider setting is reported |
| [OpenHands](@/docs/targets/openhands.md) | Different names (`always_confirm`, `never_confirm`, `confirm_risky`). Set `x-openhands` | Six snake_case events, command handlers only. Set `x-openhands` |

On Qoder, `bypassPermissions` becomes `acceptEdits` when security policy disables it, and an agent runs fewer hook events than the project does.

## `color` support by target

Only the listed tools were checked. `color` is written as given and not checked. An unrecognized value only affects looks: the agent still runs.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token |
| [Qoder](@/docs/targets/qoder.md) | One of eight names |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |

`color: blue` is valid on Augment and Qoder, but it is neither hex nor a Kilo Code theme token. OpenHands shares its `.agents/agents/` folder with [Goose](@/docs/targets/goose.md), whose frontmatter has no `color`, so a portable `color` prints a coverage note there.

