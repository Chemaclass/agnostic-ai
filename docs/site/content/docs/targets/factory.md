+++
title = "Factory"
description = "Share project rules, droids, and skills with Factory Droid, then check the native files it loads."
weight = 210

[extra]
group = "Reference"
target_id = "factory"

[extra.moved]
native-mappings = "@/docs/targets/factory-reference.md"
agents = "@/docs/targets/factory-reference.md#agents"
tools = "@/docs/targets/factory-reference.md#tools"
skills = "@/docs/targets/factory-reference.md#skills"
commands = "@/docs/targets/factory-reference.md#commands"
hooks = "@/docs/targets/factory-reference.md#hooks"
mcp = "@/docs/targets/factory-reference.md#mcp"
settings = "@/docs/targets/factory-reference.md#settings"
handoff-from-a-linked-worktree = "@/docs/targets/factory-reference.md#handoff-from-a-linked-worktree"
config-keys = "@/docs/targets/factory-reference.md#config-keys"
import = "@/docs/targets/factory-reference.md#import"
adopt-a-changed-hook-order = "@/docs/targets/factory-reference.md#adopt-a-changed-hook-order"
set-up-this-tool = "@/docs/targets/factory.md#first-rule"
+++

# Factory (`factory`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

Factory [Droid](https://docs.factory.com/harness/subagents) reads the root `AGENTS.md` and custom droids in `.factory/droids/`. It has no per-rule directory, so rules go into the `## Rules` block of `AGENTS.md`.

## First rule

1. Select `factory` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in the `## Rules` block.
3. Already have Factory files? Check [what `import factory` reads](@/docs/targets/factory-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `AGENTS.md` (shared path) |
| Agents | `.factory/droids/<name>.md` |
| Skills and bundled files | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Commands | `.factory/commands/<name>.md` |
| Hooks | `.factory/hooks.json`, when hook entries exist |
| MCP servers | `.factory/mcp.json`, when MCP entries exist |
| Settings | `.factory/settings.json`, when a settings entry has a model, a shell-command rule, or an `x-factory` key |

Overrides, field mappings, and import are in the [native reference](@/docs/targets/factory-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `AGENTS.md`.
3. Run `droid` in the project and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- An agent with an empty body is skipped with a sync note. Droid CLI needs a system prompt.
- One unknown tool ID makes Factory reject the droid. Sync maps tool names and drops the ones it cannot map ([tools](@/docs/targets/factory-reference.md#tools)).
- `readonly: true` writes `tools: read-only` and replaces a portable `tools` list. Use `x-factory.tools` for a custom list.
- A read-only droid gets an empty MCP server list unless you set one.
- `sync` rewrites `.factory/mcp.json` whole. Remove a server by deleting its spec.

{% <details summary="More limits"> %}
- Factory also reads `.agent/skills/**/SKILL.md`; this adapter does not write it ([skills](@/docs/targets/factory-reference.md#skills)).
- `reasoningEffort` takes `low`, `medium`, `high`, and is ignored under `model: inherit` ([agents](@/docs/targets/factory-reference.md#agents)).
- Portable `deny` maps to Factory's blocklist, because its denylist only prompts. Path, URL, and MCP-tool rules raise a sync note ([settings](@/docs/targets/factory-reference.md#settings)).
- With `handoff` or `handoff-hook` on, sync lets Factory's sandbox write to the main checkout's handoff store ([handoff](@/docs/targets/factory-reference.md#handoff-from-a-linked-worktree)).
{% </details> %}

## Protected paths

Not enforced. The portable `protected` setting produces a [coverage note](@/docs/spec-format/settings.md#protected-paths), without generating a blocking hook. Factory documents `PreToolUse` hooks that deny tool calls ([native hooks](https://docs.factory.com/harness/hooks#pretooluse-control)). Write a separate [hook spec](@/docs/targets/factory-reference.md#hooks) for a custom guard.

Next, [add a skill](@/docs/spec-format/skills.md) or read the [native reference](@/docs/targets/factory-reference.md).
