+++
title = "Google Antigravity"
description = "Google Antigravity: generated files, supported features, and output settings."
weight = 140

[extra]
group = "Reference"
target_id = "antigravity"

[extra.moved]
set-up-this-tool = "@/docs/targets/antigravity.md#first-rule"
native-mappings = "@/docs/targets/antigravity-reference.md"
entry-point = "@/docs/targets/antigravity-reference.md"
paths = "@/docs/targets/antigravity-reference.md"
plugins = "@/docs/targets/antigravity-reference.md"
rules = "@/docs/targets/antigravity-reference.md"
agents = "@/docs/targets/antigravity-reference.md"
skills = "@/docs/targets/antigravity-reference.md"
mcp = "@/docs/targets/antigravity-reference.md"
hooks = "@/docs/targets/antigravity-reference.md"
commands = "@/docs/targets/antigravity-reference.md"
config-keys = "@/docs/targets/antigravity-reference.md"
global-configuration = "@/docs/targets/antigravity-reference.md"
import = "@/docs/targets/antigravity-reference.md"
+++

# Google Antigravity (`antigravity`)

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Global configuration {#global-configuration .legacy-anchor}

## Import {#import .legacy-anchor}

## Set up this tool {#set-up-this-tool .legacy-anchor}

Antigravity reads per-rule files under `.agents/rules/` and custom subagents under `.agents/agents/`. agnostic-ai writes both, plus skills, MCP servers, and hooks.

## First rule

Follow the [official setup guide](https://antigravity.google/download), then open your project in the tool.

Select `antigravity` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. By default, sync writes it to `.agents/rules/conventional-commits.md`.

Rules need native trigger frontmatter. Sync supplies it from the portable activation fields.

For existing files, check [what `import antigravity` reads](@/docs/targets/antigravity-reference.md#import) before syncing.

## Output

```
.agents/AGENTS.md              # entry-point file with a short pointer body
.agents/rules/<name>.md        # one per rule
<scope>/.agents/rules/<name>.md # one per scoped rule
.agents/agents/<name>/agent.md # one per agent (custom subagent)
.agents/skills/<name>/SKILL.md # one folder per skill (Antigravity's native path)
.agents/mcp_config.json        # when MCP entries exist
.agents/hooks.json             # when hook entries exist
.agents/plugins/<name>/plugin.json  # only when an output key points into a plugin
```

For field mappings, output overrides, and import coverage, see the [Antigravity native reference](@/docs/targets/antigravity-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `.agents/rules/conventional-commits.md` for the commit-prefix rule.
3. Open the project in Antigravity and confirm the rule is Always On.
4. Ask for a commit subject for a small fix. It should use `fix:`.

## Shared memory

With `builtins: [memory]`, sync writes `antigravity-memory-index.md` as an always-on rule. Native `@[label](path)` includes load the project index and your checkout's personal index.

{% <details summary="Paths, personal memory, and size limits"> %}
- Paths resolve from the generated rule file, including when `outputs.antigravity.rules-dir` changes.
- Sync writes the same rule whether the indexes exist or not. It reads existing indexes to check the expanded size and never creates empty ones.
- With `memory.personal: repo` in local configuration, only project memory is included. The shared policy finds personal memory through `agnostic-ai memory path`.
- Global sync keeps that policy and omits project-file includes.
- Antigravity expands includes before checking its 24,000-byte rule limit.
- All always-on rules share a 20,000-token budget, which can replace large rules with pointers.
- Missing-index behavior is not documented. See the [official include and size rules](https://www.antigravity.google/docs/rules/).
{% </details> %}

## Protected paths

Not enforced. Antigravity takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
