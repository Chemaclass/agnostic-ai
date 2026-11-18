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

Follow the [official setup guide](https://junie.jetbrains.com/docs/), select `junie` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). Sync writes the commit-prefix rule to `.junie/AGENTS.md`.

Instructions apply to the IDE and the CLI. Subagents and slash commands are CLI only. Existing files? Check [what `import junie` reads](@/docs/targets/junie-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Root pointer body (shared path; fallback) | `AGENTS.md` |
| Preferred entry point: pointer body and rules | `.junie/AGENTS.md` |
| Agents (subagents) | `.junie/agents/<name>.md` |
| Skills and bundled assets | `.junie/skills/<name>/SKILL.md` |
| Commands (slash commands) | `.junie/commands/<name>.md` |
| MCP servers | `.junie/mcp/mcp.json`, when entries exist |
| Default model or effort | `.junie/config.json`, when a settings spec sets one |
| Ignore entries | `.aiignore`, when entries exist |

Field mappings and config keys: [native reference](@/docs/targets/junie-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `.junie/AGENTS.md` for the commit-prefix rule.
3. Open the project in Junie and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Junie uses the first guidelines source it finds and does not merge. Sync always writes `.junie/AGENTS.md`, so Junie [uses it exclusively](https://junie.jetbrains.com/docs/environment-variables.html). See the [lookup order](@/docs/targets/junie-reference.md#native-mappings).
- Agent names must match `[a-z0-9][a-z0-9_-]*`. `Deploy_Bot` fails sync, and the error shows the format.
- `.aiignore` makes Junie ask before it views or edits a listed file. It does not block. Brave Mode, or an allowlisted command that names the path, skips the prompt.
- Junie loads `.junie/config.json` only in a trusted project.
- Junie enables imported MCP servers by default. Disable one with `/mcp`, then **Disable**.

{% <details summary="More limits"> %}
More limits (`.aiignore` scope, identical `AGENTS.md` copies, agent bodies, Early Access, files from older versions) are in the [native reference](@/docs/targets/junie-reference.md#ignore).
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
