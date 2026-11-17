+++
title = "Junie"
description = "Junie: generated files, supported features, and output settings."
weight = 150

[extra]
group = "Reference"
target_id = "junie"

[extra.moved]
native-mappings = "@/docs/targets/junie-reference.md"
rules = "@/docs/targets/junie-reference.md#rules"
agents = "@/docs/targets/junie-reference.md#agents"
commands = "@/docs/targets/junie-reference.md#commands"
cli-only = "@/docs/targets/junie-reference.md#cli-only"
skills = "@/docs/targets/junie-reference.md#skills"
mcp = "@/docs/targets/junie-reference.md#mcp"
settings = "@/docs/targets/junie-reference.md#settings"
ignore = "@/docs/targets/junie-reference.md#ignore"
config-keys = "@/docs/targets/junie-reference.md#config-keys"
import = "@/docs/targets/junie-reference.md#import"
set-up-this-tool = "@/docs/targets/junie.md#first-rule"
+++

# Junie (`junie`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

JetBrains Junie reads `.junie/AGENTS.md` (instructions that point to source specs and rules), the rest of `.junie/`, and `.aiignore`.

## First rule

Follow the [official setup guide](https://junie.jetbrains.com/docs/), select `junie` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the shared commit-prefix rule to `.junie/AGENTS.md`.

Instructions apply to the IDE and CLI. Native subagents and slash commands are CLI features. For existing files, check [what `import junie` reads](@/docs/targets/junie-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Root pointer body (shared path; fallback) | `AGENTS.md` |
| Preferred entry point: pointer body and rules | `.junie/AGENTS.md` |
| Agents (subagents) | `.junie/agents/<name>.md` |
| Skills, plus any bundled assets | `.junie/skills/<name>/SKILL.md` |
| Commands (slash commands) | `.junie/commands/<name>.md` |
| MCP servers | `.junie/mcp/mcp.json`, when entries exist |
| Default model or effort | `.junie/config.json`, when a settings spec selects one |
| Ignore entries | `.aiignore`, when entries exist |

For field mappings and config keys, see the [native reference](@/docs/targets/junie-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `.junie/AGENTS.md` for the commit-prefix rule. Open the project in Junie and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Junie uses the first guidelines source it finds, with no merging. `sync` always writes `.junie/AGENTS.md`, so Junie [uses it exclusively](https://junie.jetbrains.com/docs/environment-variables.html). The lookup order is in the [native reference](@/docs/targets/junie-reference.md#native-mappings).
- Agent names must match `[a-z][a-z0-9_-]*`. An invalid name such as `Deploy_Bot` fails sync, and the error shows the required format.
- `.aiignore` makes Junie ask before it views or edits a listed file instead of blocking it. Brave Mode, or an allowlisted command that names the path, skips the prompt.
- Junie loads `.junie/config.json` only in a trusted project. Imported MCP servers are enabled by default; disable one with `/mcp`, then **Disable**.

{% <details summary="More limits"> %}
- The `.aiignore` file protects contents, not names. Only the IDE plugin docs mention `.aiignore`. No CLI page does.
- With `sync.target-overview` off and another tool that reads `AGENTS.md` enabled, `.junie/AGENTS.md` and the root `AGENTS.md` are identical.
- Agent bodies are not copied into `.junie/AGENTS.md`, as with Augment and Kilo Code.
- Early Access covers only the automatic model-selection toggle (`/settings → Subagents`), not the file format or how Junie finds agents.
- Older versions wrote rules and agents to `.junie/rules/`, which `.junie/AGENTS.md` hides. Sync removes generated leftovers there and keeps hand-written files.
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
