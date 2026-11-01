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

Give the agent one responsibility, the files or decisions it owns, a clear return format, and how it should check its result. Write `description` as the trigger: say when to hand a task to the agent ("use proactively" makes Claude Code delegate more often). Keep detail in the body, which loads only when the agent runs; every description loads in every session.

## When not to write one

- Keep work in the main conversation when it needs frequent back-and-forth, or when planning, implementation, and testing share one context. An agent starts fresh and returns a summary.
- Keep a quick change, or a task where speed matters, in the main conversation. An agent may need time to gather context first.

## Write one

```bash
agnostic-ai new agent code-reviewer
```

Edit `.agnostic-ai/agents/code-reviewer.md` (or its configured source path). The YAML frontmatter configures the agent; the body is its prompt.

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues. Use after a change set is complete.
can: [read, shell(git diff *)]
model: {claude: sonnet, codex: sol}
---

You are a code reviewer. Report concise findings with `file:line` references.
```

Claude Code gets `tools: [Read, Bash]` and can run any command, because it ignores the pattern ([capabilities](#capabilities)). Codex drops `can` with a note ([tool support](#tools-support-by-target)).

Run `agnostic-ai explain agents/code-reviewer.md` to inspect the translations, then `agnostic-ai sync` and `agnostic-ai sync --check`.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty; the name for Claude Code, Qoder, Antigravity, and Gemini CLI | When to hand a task to the agent. Claude Code skips an agent without one, so sync writes the name there with a note. `lint` warns when it is missing (LINT048). |
| `can` | no | unset | What the agent may do, in tool-neutral names. See [capabilities](#capabilities). |
| `tools` | no | unset | The same list in Claude Code tool names. Set `can` or `tools`, not both. See [support by target](#tools-support-by-target). |
| `model` | no | unset | A string, a map per target, or a [tier](#model-tiers) name. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `effort` | no | unset | A string or integer, or a map per target. Same section. |
| `color` | no | unset | Badge color. See [`color` support by target](#color-support-by-target). |
| `readonly` | no | unset | `true` limits the agent to reading; `false` does nothing. See [`readonly` by target](#readonly-by-target). |
| `memory` | no | unset | Persistent memory directory scope: `user`, `project`, or `local`. [Claude Code](@/docs/targets/claude-reference.md#agent-memory) uses it, [Qoder](@/docs/targets/qoder.md#subagent-memory) gets the key (unconfirmed), and Junie gets it as written. Every other tool drops it. |
| `mcpServers` | no | unset | MCP servers this agent may reach. See [support by target](#mcpservers-support-by-target). |
| `maxTurns` | no | unset | The most turns before the agent stops, a whole number of 1 or more. See [support by target](#maxturns-and-background-by-target). |
| `background` | no | unset | `true` runs the agent without making the main session wait. Same section. |
| `permissionMode` | no | unset | How much this agent may do without approval. See [support by target](#agent-policy-support-by-target). |
| `hooks` | no | unset | Lifecycle hooks for this agent only. Same section. |

Any other frontmatter field is written as given.

### `readonly` by target {#readonly-by-target}

| Target | Effect |
|--------|--------|
| Cursor | Restricted |
| Claude Code | `disallowedTools: Write, Edit, NotebookEdit` (Bash stays allowed) |
| Codex | `sandbox_mode = "read-only"`, which current Codex ignores (the agent keeps the session sandbox), and a sync note |
| Factory | `tools: read-only` with `mcpServers: []` unless servers are listed. Wins over a portable `tools` list |
| Other targets | Sync note |

An explicit `x-claude.disallowedTools`, `x-codex.sandbox_mode`, or `x-factory.tools` wins.

## Capabilities {#capabilities}

`can` lists what an agent may do, in names that belong to no one tool:

```yaml
can: [read(src/**), edit(src/**), shell(go test *), mcp:github]
```

| `can` value | Grants | Claude Code name |
|-------------|--------|------------------|
| `read`, `read(<path>)` | Read files, or matching files | `Read`; the path is dropped |
| `write` | Create or overwrite files | `Write` |
| `edit`, `edit(<path>)` | Edit files, or matching files | `Edit`; the path is dropped |
| `delete` | Delete files | No native tool |
| `shell` | Run any command | `Bash` |
| `shell(<pattern>)` | Run the commands that match the pattern | `Bash`; the pattern is dropped |
| `web` | Fetch pages and search the web | `WebFetch`, `WebSearch` |
| `mcp:<server>` | Use every tool of one MCP server | `mcp__<server>` |
| `mcp:<server>/<tool>` | Use one MCP tool | `mcp__<server>__<tool>` |

- Each capability becomes the tool's own name. `delete` raises a sync note on tools with no delete tool. The names match the hook [`match:` tool kinds](@/docs/spec-format/hook-reference.md#portable-events).
- Claude Code names work as aliases, and a list can mix both: `can: [read, Grep]`. `Agent(<type>)` and `x-claude.tools` are written as given.
- Claude Code reads an agent's `tools` by name only, so sync drops a path or pattern there and prints a note such as `claude: .agnostic-ai/agents/code-reviewer.md: shell(git diff *) becomes Bash; Claude Code does not limit an agent's tools to a pattern`. `on-unsupported: error` fails on it. To limit commands there, add a `PreToolUse` hook to the agent's `hooks`, or deny commands in a [settings spec's permissions](@/docs/spec-format/settings.md#permission-rules), which apply to every agent. A skill's `allowed-tools` keeps the pattern.
- Claude Code refuses to launch an agent with an empty tool list, such as `can: []` or `can: [delete]`. Sync leaves it out of `.claude/agents/` with a note.
- For Claude Code, a server name that contains `__` or ends in `_`, such as `reports__v2`, becomes `mcp__<server>__*`, the only whole-server form Claude Code matches for it.
- `validate`, `lint` (LINT036), and `sync` stop on an unknown capability, on `can` beside `tools`, and on `can` under `x-<target>`.
- A capability is only as strong as the tool's own permission system. agnostic-ai adds no sandbox.

`agnostic-ai migrate --only capabilities` rewrites `tools` as `can`; names with no capability of their own stay as aliases. `import` writes capabilities when each tool name matches one. `lint --suggest-capabilities` suggests neutral names for aliases (LINT037), off by default.

## Per-target `model` and `effort` {#per-target-model-and-effort}

`model:` and `effort:` each take a single value or a map keyed by target name, with an optional `default`. The first match wins:

1. `x-<target>.<key>` (`null` deletes the key)
2. `<key>.<target>`
3. `<key>.default`
4. Nothing is written, and the tool uses its own default.

A target value that is not a single value falls back to `default`.

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
  default: high
x-codex:
  model_reasoning_effort: xhigh
---
```

Claude gets `opus` and `xhigh`. Qoder gets `gpt-6.1-sol` and `8000`. Cursor gets its model id and no effort. Codex gets `gpt-6.1-sol` with effort `xhigh` from `x-codex`.

**`effort` values by target.** Without `effort`, the agent uses the session's level.

| Target | Values | How it lands |
|--------|--------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent | `effort`, not checked |
| [Qoder](@/docs/targets/qoder.md) | Same five names, or a positive integer | `effort` |
| [Junie](@/docs/targets/junie.md) | Alias of `reasoningLevel` | `effort` |
| [Factory](@/docs/targets/factory.md) | `low`, `medium`, `high` | `reasoningEffort`; other values raise a sync note |
| [Codex](@/docs/targets/codex.md) | Any string | `model_reasoning_effort`; an integer raises a note |
| [Cursor](@/docs/targets/cursor.md) | none | Sync note. Put it in the model id, as above |
| [Copilot](@/docs/targets/copilot.md) | none | Sync note. `x-copilot.effort` still passes through |
| Every other target | none | Sync note |

**Claude model names on other targets.** A shared `model` (a single value or `default`) set to a Claude model name is left out, with a sync note, on a tool that cannot load it, so that tool uses its own default. The note suggests `model: {claude: <name>}`, or names the tier to fix when the name sits in a tier's `default`. `on-unsupported: error` fails the sync instead. A value under `model.<target>` or `x-<target>.model` is written as given. `import claude` writes these names as `model: {claude: <name>}`, and `import codex` adds a Codex agent model as `model.codex`.

| Target | Claude names that raise the note |
|--------|----------------------------------|
| [Codex](@/docs/targets/codex.md), [Gemini](@/docs/targets/gemini.md), [OpenCode](@/docs/targets/opencode.md), [Kilo Code](@/docs/targets/kilo.md) | The aliases, `inherit`, and `claude-*` ids |
| [Cursor](@/docs/targets/cursor.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | The aliases |

Claude Code's [aliases](https://code.claude.com/docs/en/model-config) are `sonnet`, `opus`, `haiku`, `fable`, `best`, `opusplan`, `sonnet[1m]`, and `opus[1m]`. The value `default` resets Claude's model instead of naming one, so it raises no note.

## Model tiers {#model-tiers}

A [tier](@/docs/configuration.md#models) names a model role once in `agnostic-ai.yaml`, such as `strong: {claude: opus, codex: sol, effort: high}`. Write `model: strong` in an agent, and each target the tier names gets its model; the others keep their default. For `model`, the first match wins:

1. `x-<target>.model`
2. `model.<target>` in the spec
3. The tier's entry for the target
4. The tier's `default`
5. The tool default

To override one target, write the tier as the map's `default`: `model: {codex: gpt-6-luna, default: strong}`.

- The tier's `effort` applies only when the spec sets no `effort`, and is skipped for a target whose model the spec sets itself.
- A value under `model.<target>` or `x-<target>.model` is never a tier name. `model.<target>` can be a [vendor alias](@/docs/configuration.md#models) such as `codex: sol`.
- `explain agents/<name>.md` lists the model and effort each configured tool gets. [`models`](@/docs/configuration.md#models) lists the tier `lint` checks (LINT025, LINT026).
- `import claude` suggests a tier when two or more agents set the same Claude model. `import claude` and `import codex` keep `model: strong` when the imported model and effort match what the tier gives that tool.

## Agents as skills {#agents-as-skills}

Amp, Crush, Warp, and Zed have no subagents, so sync drops agents there by default. Set `outputs.<target>.agents: skill` to write each agent as a skill, which loads only when the model needs it.

```yaml
outputs:
  amp:
    agents: skill
```

- The skill is `<skills-dir>/<agent>/SKILL.md`, with the agent's `name` and `description`. Its body starts with a note: the model plays the role in the main session and says so. Nothing enforces that separation.
- Fields a skill cannot carry, such as `tools` and `model`, are dropped with a note.
- An [agent reference](@/docs/spec-format/_index.md#agent-and-skill-references) in skills, agents, and commands becomes that tool's skill phrase. A rule in a shared entry-point file such as `AGENTS.md` keeps the neutral phrase.
- An agent and a skill with the same name would share a folder, so `validate` fails on them.
- These four tools write `.agents/skills/`, which Codex, Copilot, Gemini, Cline, Cursor, OpenCode, Junie, and Qwen read too, and several other tools with subagents write there. When one of those is enabled, sync keeps the agents out of that directory and names the tool. Set `outputs.<target>.skills-dir` to a separate directory to use this option beside them.
- The key cannot be combined with `rules-file` or, on Warp, `workflows-dir`. Other targets reject the key.

## `can` and `tools` support by target {#tools-support-by-target}

A tool that cannot honor the list prints a sync note, so `can: [read]` never silently becomes an unrestricted agent.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Written as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Written as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md), [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md) | Changed to the tool's own names or a permission map |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md) | Dropped with a note |

On Trae, an explicitly empty list disables all tools, and an unset field loads every tool. Use `x-trae: {tools: ""}` to restrict only Trae.

The change can widen access: on Kiro, `edit` also allows `delete_file`. Sync prints a note naming the extra access, and `on-unsupported: error` fails. `explain agents/<name>.md` lists each tool's names and any widening. Most tools accept their own names through `x-<target>.tools`, which skips the change.

## `mcpServers` support by target {#mcpservers-support-by-target}

`mcpServers` names the servers, usually from `mcps/`, that one agent may reach. Without it, the agent gets the session's full set. An [agent-only server](@/docs/spec-format/mcps.md#agent-only) reaches only the agents that name it.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Qoder](@/docs/targets/qoder.md) | Server names in the agent file. An agent-only server is written in full instead |
| [Kiro](@/docs/targets/kiro.md) | Every named server written in full, under `mcpServers` |
| [OpenHands](@/docs/targets/openhands.md) | Every named server written in full, under `mcp_servers`. Not when Goose shares `.agents/agents/`: set `outputs.openhands.agents-dir` |
| [Junie](@/docs/targets/junie.md), [Factory](@/docs/targets/factory.md) | Server names in the agent file. An agent-only server's name is left out, except on Junie when it is the only name |
| [Antigravity](@/docs/targets/antigravity.md) | Dropped; its docs give no shape for an inline server. Set `x-antigravity.mcpServers` |
| [Gemini](@/docs/targets/gemini.md), [Copilot](@/docs/targets/copilot.md) | Dropped. Set `x-gemini.mcp_servers` or `x-copilot.mcp-servers` |
| Every other target | Dropped, and agent-only servers left out, with a sync note |

An `x-<target>` key for the list, such as `x-kiro.mcpServers`, replaces what sync would write. On Kiro and OpenHands, a name with no MCP spec is dropped with a note.

An empty list keeps every server on Junie and excludes every server on Factory, so list the servers you want.

## `maxTurns` and `background` by target {#maxturns-and-background-by-target}

A native key under `x-<target>`, such as `x-gemini.max_turns`, `x-kilo.steps`, or `x-cursor.is_background`, wins. An `x-<target>` override with the wrong type raises a sync note, and the shared value is used. `validate`, `lint` (LINT050), and `sync` stop on a `maxTurns` that is not a whole number of 1 or more, or a `background` that is not `true` or `false`.

| Target | `maxTurns` | `background` |
|--------|------------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `maxTurns` | `background` |
| [Qoder](@/docs/targets/qoder.md) | `maxTurns` | `background`, when background subagent sessions are on |
| [Junie](@/docs/targets/junie.md) | `maxTurns` | Sync note |
| [Gemini CLI](@/docs/targets/gemini.md) | `max_turns` | Sync note |
| [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md) | `steps` | Sync note |
| [Cursor](@/docs/targets/cursor.md) | Sync note | `is_background` |
| Every other target | Sync note | Sync note |

`import` reads each native key back into `maxTurns` or `background`. When the spec already sets the value under `x-<target>`, import writes it back there and keeps the shared value.

## `permissionMode` and agent `hooks` support by target {#agent-policy-support-by-target}

Without either field, the agent uses the session's setting. On Claude Code, a session in `bypassPermissions`, `acceptEdits`, or `auto` mode runs the agent in that mode and ignores its value, and an agent's own `bypassPermissions` never applies.

| Target | `permissionMode` | Agent `hooks` |
|--------|------------------|---------------|
| [Claude Code](@/docs/targets/claude.md) | Seven values; `manual` is an alias of `default` | Written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Six values. Any other value is reported | Seven events. A wider setting is reported |
| [OpenHands](@/docs/targets/openhands.md) | Different names (`always_confirm`, `never_confirm`, `confirm_risky`). Set `x-openhands` | Six snake_case events, command handlers only. Set `x-openhands` |

On Qoder, `bypassPermissions` becomes `acceptEdits` when security policy disables it, and an agent runs fewer hook events than the project does.

## `color` support by target

`color` is written as given and not checked. An unrecognized value only affects looks.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token, so `color: blue` may not show as expected |
| [Claude Code](@/docs/targets/claude.md), [Qoder](@/docs/targets/qoder.md) | One of eight names: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note, since it shares `.agents/agents/` with [Goose](@/docs/targets/goose.md), which has no `color`. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |
