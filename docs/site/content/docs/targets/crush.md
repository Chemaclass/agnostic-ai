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

Select `crush` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). Sync writes the commit-prefix rule to `AGENTS.md`.

Existing files? Check [what `import crush` reads](@/docs/targets/crush-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Entry-point pointer body and inlined rules | `AGENTS.md` (shared path) |
| Skills | `.agents/skills/<name>/SKILL.md` (shared tree) |
| MCP servers and hooks | `crush.json`, when entries exist (merged with your config) |
| Ignore entries | `.crushignore` |

Field mappings, MCP and hook output, and config keys: [native reference](@/docs/targets/crush-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `AGENTS.md` for the commit-prefix rule.
3. Run `crush` and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Rule bodies are inlined into `AGENTS.md`. Crush has no per-rule directory.
- Scoped rules are skipped. See [scoped context](@/docs/scoped-context.md).
- Agents are skipped with a warning. Set `outputs.crush.agents: skill` to write them as skills ([agents as skills](@/docs/spec-format/agents.md#agents-as-skills)).
- Hook matchers use lowercase tool names (`bash`, `edit`). See [hooks](@/docs/targets/crush-reference.md#hooks).

{% <details summary="More limits: crush.json and crushrc"> %}
- Sync keeps your `models`, `providers`, `lsp`, and `options` keys in `crush.json`.
- Crush prefers `crushrc`, a Bash script it runs on startup. Crush 0.102.0 also supports `options.prompts` in `crush.json`. Add that block directly to the native JSON file. [Crush prompt overrides](https://github.com/charmbracelet/crush/blob/v0.102.0/docs/config/README.md#option-prompt)
- JSON is deprecated but works, so sync writes `crush.json` and never `crushrc`.
- An MCP field that exists only in `crushrc` cannot be set here.
- Import cannot recover rule scope from the merged instructions.
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
