+++
title = "Agent reference"
description = "How each tool handles an agent's can, tools, mcpServers, maxTurns, background, permissionMode, hooks, color, and readonly fields."
weight = 11

[extra]
group = "Reference"
+++

# Agent reference

Per-tool behavior for the fields on the [agents page](@/docs/spec-format/agents.md). Where a tool cannot honor a field, sync prints a note.

## `effort` values by target {#effort-by-target}

Without `effort`, the agent uses the session's level.

| Target | Values | Output |
|--------|--------|--------------|
| [Claude Code](@/docs/targets/claude.md) | `low`, `medium`, `high`, `xhigh`, `max`, model dependent | `effort`, not checked |
| [Qoder](@/docs/targets/qoder.md) | Same five names, or a positive integer | `effort` |
| [Junie](@/docs/targets/junie.md) | Alias of `reasoningLevel` | `effort` |
| [Factory](@/docs/targets/factory.md) | `low`, `medium`, `high` | `reasoningEffort`; other values raise a sync note |
| [Codex](@/docs/targets/codex.md) | Any string | `model_reasoning_effort`; an integer raises a note |
| [Cursor](@/docs/targets/cursor.md) | none | Sync note. Put it in the model id, as above |
| [Copilot](@/docs/targets/copilot.md) | none | Sync note. `x-copilot.effort` still passes through |
| [Augment](@/docs/targets/augment.md) | `off`, `minimal`, `low`, `medium`, `high`, `xhigh` | `thinking` (Auggie 1.x); other values raise a sync note, and `x-augment.thinking` wins |
| Every other target | none | Sync note |

## Claude model names on other targets {#claude-model-names}

A shared `model` (a single value or `default`) set to a Claude model name is left out, with a sync note, on a tool that cannot load it. That tool uses its own default.

- The note suggests `model: {claude: <name>}`, or names the tier to fix when the name sits in a tier's `default`.
- `on-unsupported: error` fails the sync instead.
- A value under `model.<target>` or `x-<target>.model` is written as given.
- `import claude` writes these names as `model: {claude: <name>}`. `import codex` adds a Codex agent model as `model.codex`.

| Target | Claude names that raise the note |
|--------|----------------------------------|
| [Codex](@/docs/targets/codex.md), [Gemini](@/docs/targets/gemini.md), [OpenCode](@/docs/targets/opencode.md), [Kilo Code](@/docs/targets/kilo.md) | The aliases, `inherit`, and `claude-*` ids |
| [Cursor](@/docs/targets/cursor.md), [Factory](@/docs/targets/factory.md), [Kiro](@/docs/targets/kiro.md) | The aliases |

Claude Code's [aliases](https://code.claude.com/docs/en/model-config) are `sonnet`, `opus`, `haiku`, `fable`, `best`, `opusplan`, `sonnet[1m]`, and `opus[1m]`. The value `default` resets Claude's model instead of naming one, so it raises no note.

## `can` and `tools` support by target {#tools-support-by-target}

A tool that cannot enforce the list prints a sync note. Check these notes before relying on `can: [read]` to restrict an agent.

| Target | Behavior |
|--------|----------|
| [Claude Code](@/docs/targets/claude.md), [Copilot](@/docs/targets/copilot.md), [Junie](@/docs/targets/junie.md) | Written as a YAML list |
| [Qoder](@/docs/targets/qoder.md), [Trae](@/docs/targets/trae.md) | Written as a comma-separated string (`tools: Read, Bash`) |
| [Windsurf](@/docs/targets/windsurf.md), [Kiro](@/docs/targets/kiro.md), [Factory](@/docs/targets/factory.md), [Gemini](@/docs/targets/gemini.md), [Kilo Code](@/docs/targets/kilo.md), [OpenCode](@/docs/targets/opencode.md) | Changed to the tool's own names or a permission map |
| [Antigravity](@/docs/targets/antigravity.md), [OpenHands](@/docs/targets/openhands.md), [Goose](@/docs/targets/goose.md), [Codex](@/docs/targets/codex.md), [Cursor](@/docs/targets/cursor.md), [Augment](@/docs/targets/augment.md) | Dropped with a note |

On Trae, an explicitly empty list disables all tools, and an unset field loads every tool. Use `x-trae: {tools: ""}` to restrict only Trae.

Translation can grant extra access: on Kiro, `edit` also allows `delete_file`. Sync prints a note naming the extra access, and `on-unsupported: error` fails. `explain agents/<name>.md` lists each tool's names and any widening. Most tools accept their own names through `x-<target>.tools`, which skips the change.

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
| [Qwen Code](@/docs/targets/qwen.md) | `maxTurns` | `background` |
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
| [Qwen Code](@/docs/targets/qwen.md) | Not written, with a note: Qwen maps Claude's names to its own approval modes with different strictness. Set `x-qwen.approvalMode` | Written to the agent file; `Stop` becomes `SubagentStop`. Project agent hooks need a trusted folder |
| [CodeBuddy](@/docs/targets/codebuddy.md) | Five values; any other value stops sync | Written to the agent file. Running them needs the user's `allowUntrustedFrontmatterHooks` choice |
| [Grok Build](@/docs/targets/grok.md) | Written to the agent file | Written to the agent file |
| [Junie](@/docs/targets/junie.md) | Written to the agent file | Copied with the frontmatter, with a note: Junie documents no agent hook field |
| Every other target | Sync note | Sync note |

On Qoder, `bypassPermissions` becomes `acceptEdits` when security policy disables it, and an agent runs fewer hook events than the project does.

## `color` support by target {#color-support-by-target}

`color` is written as given and not checked. An unrecognized value affects only the badge.

| Target | Values |
|--------|--------|
| [Augment](@/docs/targets/augment.md) | Free text, an ANSI color name |
| [Kilo Code](@/docs/targets/kilo.md) | Hex or a theme token, so `color: blue` may not show as expected |
| [Claude Code](@/docs/targets/claude.md), [Qoder](@/docs/targets/qoder.md) | One of eight names: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` |
| [OpenHands](@/docs/targets/openhands.md) | Dropped with a note, since it shares `.agents/agents/` with [Goose](@/docs/targets/goose.md), which has no `color`. Set `x-openhands.color` to a [Rich color name](https://rich.readthedocs.io/en/stable/appendix/colors.html) |

## `readonly` by target {#readonly-by-target}

| Target | Effect |
|--------|--------|
| Cursor | Restricted |
| Claude Code | `disallowedTools: Write, Edit, NotebookEdit` (Bash stays allowed) |
| Codex | `sandbox_mode = "read-only"`, which current Codex ignores (the agent keeps the session sandbox), and a sync note |
| Qwen Code | `disallowedTools: [write_file, edit, notebook_edit]` (shell stays allowed) |
| Factory | `tools: read-only` with `mcpServers: []` unless servers are listed. Wins over a portable `tools` list |
| Other targets | Sync note |

An explicit `x-claude.disallowedTools`, `x-qwen.disallowedTools`, `x-codex.sandbox_mode`, or `x-factory.tools` wins.
