+++
title = "Agents"
description = "agents/: subagents with their own prompt, tools, and model, written once for every tool."
weight = 10
aliases = ["/docs/spec-format/agent-specs/"]

[extra]
group = "Reference"
+++

# Agents

`agents/` defines subagents: specialists the main agent hands a task to, each with its own instructions, tools, and model. A reviewer that only reads, an architect on the strongest model, a test writer on a cheaper one.

Why a subagent instead of more instructions in the main session:

- **Clean context.** A subagent works in its own context and hands back a result, so its exploration stays out of the main conversation.
- **Least privilege.** `tools`, `readonly`, and `mcpServers` narrow what it may touch.
- **The right model per role.** `model` and `effort` set cost and depth per agent, and per tool.
- **One definition.** Every target with an agent surface gets its native file from the same spec; the rest get a coverage note.

## Write one

`agnostic-ai new agent code-reviewer` creates `agents/code-reviewer.md`. The frontmatter configures the agent. The body is its system prompt.

```markdown
---
name: code-reviewer
description: Reviews diffs for bugs, style, and architectural issues. Use after a change set is complete.
tools: [Read, Grep, Bash]
model: sonnet
---

You are a code reviewer. Report concise findings with `file:line` references.
```

Write `description` for the main agent: it reads it to decide when to delegate, so say what the agent does and when to use it.

A read-only auditor on a stronger model for Claude Code, with a fallback everywhere else and one MCP server:

```markdown
---
name: security-auditor
description: Checks a diff for injection, leaked secrets, and unsafe input handling. Use before merging auth or payment changes.
readonly: true
model: {claude: opus, default: gpt-5.5}
mcpServers: [github]
---

List each finding with its `file:line`, the attack it enables, and the smallest fix.
```

## Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | no | filename without `.md` | Agent identifier and output filename. |
| `description` | no | empty | When to delegate to the agent. Tools show it in listings and use it to pick an agent. |
| `tools` | no | unset | Tools the agent may invoke. See [`tools` support by target](#tools-support-by-target). |
| `model` | no | unset | A string for every target, a map per target, or a [tier](#model-tiers) name. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `effort` | no | unset | A string or integer for every target, or a map per target. See [per-target `model` and `effort`](#per-target-model-and-effort). |
| `color` | no | unset | Badge color. See [`color` support by target](#color-support-by-target). |
| `readonly` | no | unset | `true` restricts the agent to reading. Cursor: restricted. Claude: `disallowedTools: Write, Edit, NotebookEdit` (Bash stays allowed). Codex: writes `sandbox_mode = "read-only"`, which current Codex ignores (the agent keeps the session sandbox), with a coverage note. Factory: `tools: read-only` with `mcpServers: []` unless servers are listed (wins over a portable `tools` list). An explicit `x-claude.disallowedTools`, `x-codex.sandbox_mode`, or `x-factory.tools` wins. Other targets report a coverage note. `false` is a no-op. |
| `memory` | no | unset | Persistent memory scope: `user`, `project`, or `local`. |
| `mcpServers` | no | unset | MCP servers this agent may reach. See [`mcpServers` support by target](#mcpservers-support-by-target). |
| `permissionMode` | no | unset | Approval boundary for this agent. See [`permissionMode` and agent `hooks`](#agent-policy-support-by-target). |
| `hooks` | no | unset | Lifecycle hooks scoped to this agent. See [`permissionMode` and agent `hooks`](#agent-policy-support-by-target). |

Any other frontmatter field passes through unchanged.

`memory` gives the agent a directory that survives across sessions. Only [Claude Code](@/docs/targets/claude.md#agent-memory) is confirmed to act on it; [Qoder](@/docs/targets/qoder.md#subagent-memory) gets the key unconfirmed, Junie passes it through, and every other adapter drops it.

## Per-target `model` and `effort` {#per-target-model-and-effort}

`model:` and `effort:` each take a scalar or a map keyed by target name, with an optional `default`. Precedence: `x-<target>.<key>`, then `<key>.<target>`, then `<key>.default`, then the key is not written and the tool uses its own default. `x-<target>.<key>: null` deletes it. A non-scalar value under a target key falls through to `default`.

| Want | Write |
|------|-------|
| Same model everywhere | `model: gpt-5.5` |
| Per target, with a fallback | `model: {claude: sonnet, default: gpt-4o}` |
| Per target, tool default elsewhere | `model: {claude: sonnet}` |
| Different effort per target | `effort: {claude: xhigh, default: high}` |

```yaml
---
name: architect
description: Designs the change before anyone codes it.
model:
  claude: opus
  cursor: "claude-opus-5[effort=high]"
  default: gpt-5.5
effort:
  claude: xhigh
  qoder: 8000
  factory: max
  default: high
x-codex:
  model_reasoning_effort: xhigh
---
```

Result: Claude gets `opus` and `xhigh`; Qoder `gpt-5.5` and `8000`; Junie `gpt-5.5` and `high`; Cursor `claude-opus-5[effort=high]` (resolved effort discarded); Codex `gpt-5.5` with `x-codex` overriding effort to `xhigh`; Factory `gpt-5.5` with no `reasoningEffort` (`max` is outside its enum, coverage note); Trae drops both with notes.

**`effort` values by target.** Only the targets listed were checked. Omitting `effort` inherits the session's level.

| Target | Values | How it lands |
|--------|--------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent | Verbatim `effort`, not validated |
| [Qoder](@/docs/targets/qoder.md) | Same five names, or a positive integer | Verbatim `effort` |
| [Junie](@/docs/targets/junie.md) | Alias of `reasoningLevel` | Verbatim `effort` |
| [Factory](@/docs/targets/factory.md) | `low`, `medium`, `high` | `reasoningEffort`. Other values and integers raise a coverage note |
| [Codex](@/docs/targets/codex.md) | Any string | `model_reasoning_effort`; `x-codex.model_reasoning_effort` wins. An integer raises a note |
| [Cursor](@/docs/targets/cursor.md) | none | Coverage note. Put it in the model id: `model: {cursor: "claude-opus-5[effort=high]"}` |
| [Copilot](@/docs/targets/copilot.md) | none | Coverage note. `x-copilot.effort` still passes through |
| Every other target | none | Coverage note |

Cursor encodes effort in the `model` string, so it rides on the `model` map. Factory ignores `reasoningEffort` when `model` resolves to `inherit`.

**Claude model names on other targets.** A shared `model` (a scalar or `default`) set to a Claude model name raises a coverage note on a target that cannot load it, naming `model: {claude: <name>}`. `on-unsupported: error` fails the sync instead. A value under `model.<target>` or `x-<target>.model` passes. When the name comes from a tier's `default`, the note names the tier to fix. `import claude` writes these names as `model: {claude: <name>}`. `import codex` adds a Codex agent model to an existing spec as `model.codex`.

| Target | Claude names that raise the note |
|--------|----------------------------------|
| [Codex](@/docs/targets/codex.md), [Gemini](@/docs/targets/gemini.md), [OpenCode](@/docs/targets/opencode.md), [Kilo Code](@/docs/targets/kilo.md) | `sonnet`, `opus`, `haiku`, `inherit`, and `claude-*` ids |
| [Cursor](@/docs/targets/cursor.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | `sonnet`, `opus`, and `haiku` |

Only the targets listed were checked.

## Model tiers {#model-tiers}

Model ids belong to one vendor, so a per-target map repeats in every agent. Name the roles once under [`models`](@/docs/configuration.md#models) in `agnostic-ai.yaml` and write the tier name instead:

```yaml
# agnostic-ai.yaml
models:
  strong: {claude: opus, codex: gpt-5.5, effort: {claude: xhigh, codex: high}}
```

```yaml
---
name: architect
description: Designs the change before anyone codes it.
model: strong
---
```

Claude gets `opus` with `xhigh`, Codex `gpt-5.5` with `high`, and every other target its own default. Skills, commands, and settings specs name tiers the same way.

Precedence, high to low: `x-<target>.model`, then `model.<target>` in the spec, then the tier's entry for the target, then the tier's `default`, then the tool default. To override one target, write the tier as the map's `default`: `model: {codex: o4-mini, default: strong}`. The tier's `effort` applies only when the spec sets no `effort`; a spec `effort` replaces it whole. Values under `model.<target>` and `x-<target>.model` are always literal model ids.

`explain agents/architect.md` lists the model and effort each configured target gets. `lint` flags a tier an agent names with no entry and no `default` for one of the agent's targets (LINT023), and a Claude model name in a shared `model` or a tier `default` that reaches another vendor's target (LINT024). `import claude` suggests a tier when two or more agents set the same Claude model.

## `tools` support by target

Only the targets listed were checked. A target that cannot honor `tools` prints a coverage note at sync time, so `tools: [Read]` never silently becomes an unrestricted agent.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Passed through as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Passed through as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md) | Translated to native names |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md), [Kilo Code](@/docs/targets/kilo.md) | Dropped with a note |

Translation can widen access: on Kiro, `Edit` alone also permits `delete_file`. Most targets accept native names through `x-<target>.tools`, which bypasses translation.

## `mcpServers` support by target {#mcpservers-support-by-target}

Only the targets listed were checked. A top-level `mcpServers` list narrows which MCP servers one agent may reach. Omitting it inherits the session's full set.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Junie](@/docs/targets/junie.md), [Factory](@/docs/targets/factory.md) | Server names, written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Server names or inline objects, written to the agent file |
| [OpenHands](@/docs/targets/openhands.md) | Inline definitions only. Set `x-openhands.mcp_servers` |
| [Antigravity](@/docs/targets/antigravity.md) | Inline objects only. Set `x-antigravity.mcpServers` |
| [Kiro](@/docs/targets/kiro.md) | Inline definitions only. Set `x-kiro.mcpServers` |

Every other target drops the list with a coverage note; the three inline targets' notes name the `x-<target>` key to set.

**An empty list is not portable.** Junie treats `mcpServers: []` as keeping every configured server. Factory treats it as excluding every server. List the servers you want instead.

## `permissionMode` and agent `hooks` support by target {#agent-policy-support-by-target}

Only the targets listed were checked. `permissionMode` sets one delegated agent's approval boundary; `hooks` scopes lifecycle hooks to it. Omitting either inherits the parent session.

| Target | `permissionMode` | Agent `hooks` |
|--------|------------------|---------------|
| [Claude Code](@/docs/targets/claude.md) | Seven values, `manual` aliases `default` | Written to the agent file |
| [Qoder](@/docs/targets/qoder.md) | Six values, another one is reported | Seven events, a wider one is reported |
| [OpenHands](@/docs/targets/openhands.md) | Different names (`always_confirm`, `never_confirm`, `confirm_risky`). Set `x-openhands` | Six snake_case events, command handlers only. Set `x-openhands` |

On Qoder, `bypassPermissions` is demoted to `acceptEdits` when security policy disables it, and agent scope runs fewer hook events than project scope.

## `color` support by target

Only the targets listed were checked. `color` is written verbatim and not validated. An unrecognized value is cosmetic: the agent still runs.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token |
| [Qoder](@/docs/targets/qoder.md) | One of eight names |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |

`color: blue` is valid on Augment and Qoder but is neither hex nor a Kilo Code theme token. OpenHands shares its `.agents/agents/` tree with [Goose](@/docs/targets/goose.md), whose frontmatter has no `color`, so a portable `color` prints a coverage note there.

