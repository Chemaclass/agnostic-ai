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

1. Select `kilo` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes a section in `AGENTS.md`.

Import covers only selected settings, ignore patterns, and generated rule sections. Before syncing over a hand-written Kilo setup, read [import coverage](@/docs/targets/kilo-reference.md#import) and back up what it cannot import.

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
| Ignore entries | `.kilocodeignore`, an input for Kilo's permission migrator |

Overrides, field mappings, and import are in the [native reference](@/docs/targets/kilo-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `AGENTS.md`.
3. Open the project in Kilo and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Kilo [always loads](https://kilo.ai/docs/customize/agents-md) the root `AGENTS.md`. Scoped rules use nested `AGENTS.md` ([rules](@/docs/targets/kilo-reference.md#rules)).
- Kilo has no `tools:` key. A `tools` list becomes a `permission` map ([agents](@/docs/targets/kilo-reference.md#agents)).
- Kilo loads plugins only from `plugin/` or `plugins/`. Moving `outputs.kilo.hooks-dir` stops hooks loading.
- This adapter never writes `.kilo/kilo.jsonc`. A hand-written one merges over the root `kilo.jsonc` and replaces its `mcp` or `instructions`.

{% <details summary="More limits"> %}
- `color` is not checked. Kilo takes hex or a theme token, so `color: blue` may not show as expected ([`color` support](@/docs/spec-format/agents.md#color-support-by-target)).
- Kilo reserves the command name `goal`. It ignores a file by that name, warns, and offers it as `/goal:command`.
- `WebFetch(domain:...)` rules drop with a sync note. Kilo matches URLs.
- With `memory.personal: repo`, the personal store is outside the project root and Kilo ignores project-declared files there. Sync writes only the shared index and prints a note ([config keys](@/docs/targets/kilo-reference.md#config-keys), [repo mode](@/docs/memory.md#one-store-per-repository)).
- Kilo still loads the legacy `.kilocode/rules/`. This adapter never writes it.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
