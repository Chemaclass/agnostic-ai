+++
title = "Augment"
description = "Augment: generated files, supported features, and output settings."
weight = 250

[extra]
group = "Reference"
target_id = "augment"

[extra.moved]
set-up-this-tool = "@/docs/targets/augment.md#first-rule"
native-mappings = "@/docs/targets/augment-reference.md"
mcp = "@/docs/targets/augment-reference.md"
hooks = "@/docs/targets/augment-reference.md"
shared-memory = "@/docs/targets/augment-reference.md"
permissions = "@/docs/targets/augment-reference.md"
config-keys = "@/docs/targets/augment-reference.md"
import = "@/docs/targets/augment-reference.md"
+++

# Augment (`augment`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## MCP {#mcp .legacy-anchor}

## Hooks {#hooks .legacy-anchor}

## Shared memory {#shared-memory .legacy-anchor}

## Permissions {#permissions .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Import {#import .legacy-anchor}

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, `.augment/`, and the shared `.agents/skills/` tree.

## First rule

1. Select `augment` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the commit-prefix rule to `AGENTS.md`.

MCP and permission settings apply to Auggie CLI. The IDE reads project rules and skills, and runs project hooks from `.augment/settings.json` once you trust the workspace ([Augment changelog](https://www.augmentcode.com/changelog)).

Already have Augment files? [Import](@/docs/targets/augment-reference.md#import) reads only agents, ignore patterns, selected permission rules, and the default model and effort. Review the other native files before syncing.

## Output

| Source | Default output |
|---|---|
| Shared instructions and rules | `AGENTS.md` (entry-point pointer body plus rules; shared path) |
| Rules `AGENTS.md` does not carry, and `agent_requested` rules | `.augment/rules/<name>.md` |
| Agents | `.augment/agents/<name>.md` |
| Commands | `.augment/commands/<name>.md`; a nested source scope becomes a namespace |
| MCP servers, hooks, permissions, default model and effort | `.augment/settings.json`, merged; only with MCP, hook, or settings specs |
| Skills | `.agents/skills/<name>/SKILL.md` (shared cross-tool tree) |
| Ignore patterns | `.augmentignore` (workspace indexing exclusions) |
| Legacy joined rules | `.augment-guidelines`, only when `rules-file` is set |

Overrides, field mappings, and import are in the [native reference](@/docs/targets/augment-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Look for the commit-prefix rule in `AGENTS.md`.
3. Open the project in Augment and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Augment also loads `.augment/rules/`. An always-applied rule already in the `AGENTS.md` block gets no file there ([target behavior](@/docs/target-behavior.md#entry-point-files)).
- Hooks use Auggie's seven events. A hook on another event, such as `UserPromptSubmit`, is skipped with a sync note and `validate` rejects it ([hooks](@/docs/targets/augment-reference.md#hooks)).
- Permissions are rule objects under `toolPermissions`: deny, then ask, then allow. Path or URL rules such as `Read(src/**)` are not written ([permissions](@/docs/targets/augment-reference.md#permissions)).

{% <details summary="More limits"> %}
- Auggie 1.x runs `SessionStart` hooks in the background and drops their output, so the memory rule tells the agent to read the indexes ([shared memory](@/docs/targets/augment-reference.md#shared-memory)).
- Disabled MCP servers and `type: ws` servers are not written. Each raises a sync note ([MCP](@/docs/targets/augment-reference.md#mcp)).
- Agent `tools` and `disabled_tools` take Augment's own tool names, so the generic `tools` field raises a sync note. Set `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}`.
- Moving the entry point off root `AGENTS.md`, or listing it in `sync.unmanaged`, makes every rule keep a `.augment/rules/` file. Check [the effect on other targets](@/docs/target-behavior.md#inlined-rules) first.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
