+++
title = "Agents"
description = "agents/: subagents with their own prompt, tools, and model, written once for every tool."
weight = 10
aliases = ["/docs/spec-format/agent-specs/"]

[extra]
group = "Reference"

[extra.moved]
readonly-by-target = "@/docs/spec-format/agents-reference.md"
tools-support-by-target = "@/docs/spec-format/agents-reference.md"
mcpservers-support-by-target = "@/docs/spec-format/agents-reference.md"
maxturns-and-background-by-target = "@/docs/spec-format/agents-reference.md"
agent-policy-support-by-target = "@/docs/spec-format/agents-reference.md"
color-support-by-target = "@/docs/spec-format/agents-reference.md"
+++

# Agents

`agents/` defines subagents for the main agent, each with its own instructions, tools, and model. A reviewer can be read-only, and a test writer can use a cheaper model.

Give the agent one responsibility: the files or decisions it owns, the result it returns, and how to check that result. Use `description` to say when to delegate to it ("use proactively" makes Claude Code delegate more often). The body loads only when the agent runs; descriptions load every session.

Keep work in the main conversation when it needs frequent back-and-forth, when planning, implementation, and testing share one context, or when speed matters. An agent starts fresh, may spend time gathering context, and returns a summary.

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

Claude Code gets `tools: [Read, Bash]` and can run any command, because it ignores the pattern ([capabilities](#capabilities)). Codex drops `can` with a note ([tool support](@/docs/spec-format/agents-reference.md#tools-support-by-target)).

Run `agnostic-ai explain agents/code-reviewer.md` to inspect the translations, then `agnostic-ai sync` and `agnostic-ai sync --check`.

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty; the name for Claude Code, Qoder, Antigravity, and Gemini CLI | When to hand a task to the agent. Claude Code skips an agent without one, so sync writes the name there with a note. `lint` warns when it is missing (LINT048). |
| `can` | no | unset | What the agent may do, in tool-neutral names. See [capabilities](#capabilities). |
| `tools` | no | unset | The same list in Claude Code tool names. Set `can` or `tools`, not both. See [support by target](@/docs/spec-format/agents-reference.md#tools-support-by-target). |
| `model` | no | unset | A string, a map per target, or a [tier](#model-tiers) name. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `effort` | no | unset | A string or integer, or a map per target. Same section. |
| `color` | no | unset | Badge color. See [`color` support by target](@/docs/spec-format/agents-reference.md#color-support-by-target). |
| `readonly` | no | unset | `true` limits the agent to reading; `false` does nothing. See [`readonly` by target](@/docs/spec-format/agents-reference.md#readonly-by-target). |
| `memory` | no | unset | Persistent memory directory scope: `user`, `project`, or `local`. [Claude Code](@/docs/targets/claude-reference.md#agent-memory) uses it, [Qoder](@/docs/targets/qoder.md#subagent-memory) gets the key (unconfirmed), and Junie gets it as written. Every other tool drops it. |
| `mcpServers` | no | unset | MCP servers this agent may reach. See [support by target](@/docs/spec-format/agents-reference.md#mcpservers-support-by-target). |
| `maxTurns` | no | unset | The most turns before the agent stops, a whole number of 1 or more. See [support by target](@/docs/spec-format/agents-reference.md#maxturns-and-background-by-target). |
| `background` | no | unset | `true` runs the agent without making the main session wait. Same section. |
| `permissionMode` | no | unset | How much this agent may do without approval. See [support by target](@/docs/spec-format/agents-reference.md#agent-policy-support-by-target). |
| `hooks` | no | unset | Lifecycle hooks for this agent only. Same section. |

Any other frontmatter field is written as given.

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

<details>
<summary>Capability edge cases and Claude Code limits</summary>

- Each capability becomes the tool's own name. `delete` raises a sync note on tools with no delete tool. The names match the hook [`match:` tool kinds](@/docs/spec-format/hook-reference.md#portable-events).
- Claude Code names work as aliases, and a list can mix both: `can: [read, Grep]`. `Agent(<type>)` and `x-claude.tools` are written as given.
- Claude Code reads an agent's `tools` by name only, so sync drops a path or pattern there and prints a note. Example: `claude: .agnostic-ai/agents/code-reviewer.md: shell(git diff *) becomes Bash; Claude Code does not limit an agent's tools to a pattern`.
- `on-unsupported: error` fails on that note.
- To limit commands on Claude Code, add a `PreToolUse` hook to the agent's `hooks`, or deny commands in a [settings spec's permissions](@/docs/spec-format/settings.md#permission-rules), which apply to every agent. A skill's `allowed-tools` keeps the pattern.
- Claude Code refuses to launch an agent with an empty tool list, such as `can: []` or `can: [delete]`. Sync leaves it out of `.claude/agents/` with a note.
- For Claude Code, a server name that contains `__` or ends in `_`, such as `reports__v2`, becomes `mcp__<server>__*`, the only whole-server form Claude Code matches for it.
- `validate`, `lint` (LINT036), and `sync` stop on an unknown capability, on `can` beside `tools`, and on `can` under `x-<target>`.
- A capability is only as strong as the tool's own permission system. agnostic-ai adds no sandbox.

</details>

- `agnostic-ai migrate --only capabilities` rewrites `tools` as `can`. Names with no capability of their own stay as aliases.
- `import` writes capabilities when each tool name matches one.
- `lint --suggest-capabilities` suggests neutral names for aliases (LINT037). It is off by default.

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

Per-tool `effort` values, and which Claude model names other tools cannot load, are in the [agent reference](@/docs/spec-format/agents-reference.md#effort-by-target).

## Model tiers {#model-tiers}

A [tier](@/docs/configuration.md#models) names a model role once in `agnostic-ai.yaml`, such as `strong: {claude: opus, codex: sol, effort: high}`. Write `model: strong` in an agent, and each target the tier names gets its model; the others keep their default. A tier's [`description`](@/docs/configuration.md#models) tells the main agent when to pick it. For `model`, the first match wins:

1. `x-<target>.model`
2. `model.<target>` in the spec
3. The tier's entry for the target
4. The tier's `default`
5. The tool default

To override one target, write the tier as the map's `default`: `model: {codex: gpt-6-luna, default: strong}`.

<details>
<summary>Tier precedence details</summary>

- The tier's `effort` applies only when the spec sets no `effort`, and is skipped for a target whose model the spec sets itself.
- A value under `model.<target>` or `x-<target>.model` is never a tier name. `model.<target>` can be a [vendor alias](@/docs/configuration.md#models) such as `codex: sol`.
- `explain agents/<name>.md` lists the model and effort each configured tool gets. [`models`](@/docs/configuration.md#models) lists the tier `lint` checks (LINT025, LINT026).
- `import claude` suggests a tier when two or more agents set the same Claude model. `import claude` and `import codex` keep `model: strong` when the imported model and effort match what the tier gives that tool.

</details>

## Agents as skills {#agents-as-skills}

Sync drops agent specs for Amp, Crush, Warp, and Zed by default. Set `outputs.<target>.agents: skill` to write each agent as a skill, which loads only when the model needs it.

```yaml
outputs:
  amp:
    agents: skill
```

The generated skill:

- is `<skills-dir>/<agent>/SKILL.md`, with the agent's `name` and `description`;
- starts with a note that the model plays the role in the main session and says so (nothing enforces that separation);
- drops fields a skill cannot carry, such as `tools` and `model`, with a note;
- turns an [agent reference](@/docs/spec-format/shared-features.md#agent-and-skill-references) in skills, agents, and commands into that tool's skill phrase. A rule in a shared entry-point file such as `AGENTS.md` keeps the neutral phrase.

<details>
<summary>Limits and name clashes</summary>

- An agent and a skill with the same name would share a folder, so `validate` fails on them.
- These four tools write `.agents/skills/`. Codex, Copilot, Gemini, Cline, Cursor, OpenCode, Junie, and Qwen read it too, and several other tools with subagents write there.
- When one of those tools is enabled, sync keeps the agents out of that directory and names the tool. Set `outputs.<target>.skills-dir` to a separate directory to use this option beside them.
- The key cannot be combined with `rules-file` or, on Warp, `workflows-dir`. Other targets reject the key.

</details>

Per-tool tables for `can`, `tools`, `mcpServers`, `maxTurns`, `background`, `permissionMode`, `hooks`, `color`, and `readonly` are in the [agent reference](@/docs/spec-format/agents-reference.md).
