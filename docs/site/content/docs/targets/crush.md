+++
title = "Crush"
description = "Crush: generated files, supported features, and output settings."
weight = 170

[extra]
group = "Reference"
target_id = "crush"

[extra.moved]
native-mappings = "@/docs/targets/crush-reference.md"
mcp = "@/docs/targets/crush-reference.md#mcp"
hooks = "@/docs/targets/crush-reference.md#hooks"
config-keys = "@/docs/targets/crush-reference.md#config-keys"
import = "@/docs/targets/crush-reference.md#import"
set-up-this-tool = "@/docs/targets/crush.md#first-rule"
+++

# Crush (`crush`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Charm [Crush](https://github.com/charmbracelet/crush) reads `AGENTS.md`, `.agents/skills/`, `crush.json`, and `.crushignore`.

## First rule

Select `crush` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the shared commit-prefix rule to `AGENTS.md`.

For existing files, check [what `import crush` reads](@/docs/targets/crush-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Entry-point pointer body and inlined rules | `AGENTS.md` (shared path) |
| Skills | `.agents/skills/<name>/SKILL.md`, one folder per skill (shared tree) |
| MCP servers and hooks | `crush.json`, when entries exist (merged with existing user config) |
| Ignore entries | `.crushignore`, project root |

For field mappings, MCP and hook output, and config keys, see the [native reference](@/docs/targets/crush-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `crush` from the project and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Crush has no per-rule directory, so rule bodies are included in `AGENTS.md`. It has no documented directory scope: sync skips scoped rules, and import cannot recover scope from the merged instructions. See [scoped context](@/docs/scoped-context.md).
- Crush has no native custom-agent profiles. By default, agents are skipped with a warning. Set `outputs.crush.agents: skill` to write them as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).
- Sync keeps your `models`, `providers`, `lsp`, and `options` keys in `crush.json`. Crush now prefers `crushrc`, a Bash script it runs on startup, and adds new options only there. JSON is deprecated but still works, so sync writes `crush.json` and never `crushrc`. An MCP field that exists only in `crushrc` cannot be set here.
- Hook matchers use lowercase tool names (`bash`, `edit`). A Claude-style matcher (`Bash`, `Edit`) matches nothing, so `sync` prints a note that the field has no effect. See [native lifecycle events and matcher limits](@/docs/targets/crush-reference.md#hooks).

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
