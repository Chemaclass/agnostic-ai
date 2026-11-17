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

Factory [Droid](https://docs.factory.com/harness/subagents) reads the root `AGENTS.md` and custom droids in `.factory/droids/`. Factory has no per-rule directory, so rule bodies go into the `## Rules` block of `AGENTS.md`.

## First rule

Select `factory` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes a section in the `## Rules` block of `AGENTS.md`.

Custom droids come from agent specs. A read-only droid also needs its MCP access restricted; sync supplies an empty server list unless you set one. Already have Factory files? Check [what `import factory` reads](@/docs/targets/factory-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `AGENTS.md` (shared path) |
| Agents | `.factory/droids/<name>.md` |
| Skills and bundled files | `.agents/skills/<name>/SKILL.md` (shared tree) |
| Commands | `.factory/commands/<name>.md` |
| Hooks | `.factory/hooks.json`, when hook entries exist |
| MCP servers | `.factory/mcp.json`, when MCP entries exist |
| Settings | `.factory/settings.json`, when a settings entry carries a model, a shell-command rule, or an `x-factory` key |

For output overrides, field mappings, and import coverage, see the [native reference](@/docs/targets/factory-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `droid` from the project and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- An agent with an empty body is skipped with a sync note, because Droid CLI requires a system prompt.
- One unknown tool ID makes Factory reject the droid. Sync translates the generic `tools` list to Droid CLI's IDs and drops names it cannot map. See [tools](@/docs/targets/factory-reference.md#tools).
- `readonly: true` writes `tools: read-only`, which replaces a portable `tools` list instead of narrowing it. Set `x-factory.tools` for a custom list.
- `sync` rewrites `.factory/mcp.json` whole, so remove an MCP server by deleting its spec, not by editing the file.

{% <details summary="More limits"> %}
- Factory also reads [`.agent/skills/**/SKILL.md`](https://docs.factory.com/harness/skills), which this adapter does not write. Scoped skills go to `<scope>/.factory/skills/<name>/SKILL.md`.
- Factory ignores `reasoningEffort` under `model: inherit`, and accepts only `low`, `medium`, and `high` on a droid. Other values drop with a sync note.
- Factory's denylist only prompts, so portable `deny` maps to the blocklist. Rules scoping a path, URL, or MCP tool have no shell form and raise a sync note. See [settings](@/docs/targets/factory-reference.md#settings).
- With `handoff` or `handoff-hook` on, sync adds the main checkout's handoff store to `.factory/settings.local.json` so Factory's sandbox lets handoff saves reach it. See [handoff from a linked worktree](@/docs/targets/factory-reference.md#handoff-from-a-linked-worktree).
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or read the [native reference](@/docs/targets/factory-reference.md).
