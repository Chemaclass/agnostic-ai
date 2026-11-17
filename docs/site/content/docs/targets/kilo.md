+++
title = "Kilo"
description = "Share project rules, agents, and skills with Kilo, then check the native files it loads."
weight = 220

[extra]
group = "Reference"
target_id = "kilo"

[extra.moved]
native-mappings = "@/docs/targets/kilo-reference.md"
shell-commands-in-global-skills = "@/docs/targets/kilo-reference.md#shell-commands-in-global-skills"
agents = "@/docs/targets/kilo-reference.md#agents"
skills = "@/docs/targets/kilo-reference.md#skills"
rules = "@/docs/targets/kilo-reference.md#rules"
commands = "@/docs/targets/kilo-reference.md#commands"
mcp = "@/docs/targets/kilo-reference.md#mcp"
hooks = "@/docs/targets/kilo-reference.md#hooks"
settings = "@/docs/targets/kilo-reference.md#settings"
ignore = "@/docs/targets/kilo-reference.md#ignore"
config-keys = "@/docs/targets/kilo-reference.md#config-keys"
import = "@/docs/targets/kilo-reference.md#import"
set-up-this-tool = "@/docs/targets/kilo.md#first-rule"
+++

# Kilo (`kilo`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

Kilo [Code](https://kilo.ai/docs) gets `AGENTS.md`, agents, commands, plugin hooks, shared skills, and a merged `kilo.jsonc`.

## First rule

Select `kilo` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes a section in `AGENTS.md`.

Import covers only selected settings, ignore patterns, and generated rule sections. It does not preserve a complete hand-written Kilo setup. Before syncing, review [import coverage](@/docs/targets/kilo-reference.md#import) and back up files it cannot import.

## Output

| Source | Default output |
|---|---|
| Shared instructions and unscoped rules | `AGENTS.md` (shared path) |
| Rules the root `AGENTS.md` does not carry | `.kilo/rules/<name>.md` |
| Agents | `.kilo/agents/<name>.md` |
| Skills and bundled files | `.agents/skills/<name>/SKILL.md` (shared cross-tool tree) |
| Commands | `.kilo/commands/<name>.md` |
| Hooks | `.kilo/plugin/<name>.ts`, auto-registered at startup |
| Instructions, MCP, permissions | `kilo.jsonc` (merged with existing user config) |
| Ignore entries | `.kilocodeignore`, a compatibility input for Kilo's permission migrator |

For output overrides, field mappings, and import coverage, see the [native reference](@/docs/targets/kilo-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Open the project in Kilo and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Kilo [always loads](https://kilo.ai/docs/customize/agents-md) the root `AGENTS.md`. Unscoped rules live there; scoped rules use nested `AGENTS.md`. See [rules](@/docs/targets/kilo-reference.md#rules).
- Kilo has no `tools:` key, so a `tools` list becomes a `permission` map. See [agents](@/docs/targets/kilo-reference.md#agents).
- Kilo only loads plugins from `plugin/` or `plugins/`, so moving `outputs.kilo.hooks-dir` stops the hooks loading.
- This adapter never writes `.kilo/kilo.jsonc`. A hand-written one merges over the root `kilo.jsonc`, so its `mcp` or `instructions` replace the generated ones.

{% <details summary="More limits"> %}
- `color` is not checked. Kilo accepts hex or a theme token, so `color: blue` may not show as you expect. See [`color` support by target](@/docs/spec-format/agents.md#color-support-by-target).
- Kilo reserves the command name `goal`. It ignores a command file by that name, warns, and offers it as `/goal:command`.
- `WebFetch(domain:...)` rules drop with a sync note, because Kilo matches URLs.
- With `memory.personal: repo`, the personal store lies outside the project root, and Kilo ignores project-declared files there. Sync writes only the shared index and prints a note. See [config keys](@/docs/targets/kilo-reference.md#config-keys) and [repo mode](@/docs/memory.md#one-store-per-repository).
- Kilo still loads the legacy `.kilocode/rules/`, which this adapter never writes.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
