+++
title = "Zed"
description = "Zed: generated files, supported features, and output settings."
weight = 110

[extra]
group = "Reference"
target_id = "zed"

[extra.moved]
native-mappings = "@/docs/targets/zed-reference.md"
config-keys = "@/docs/targets/zed-reference.md#config-keys"
global-instructions = "@/docs/targets/zed-reference.md#global-instructions"
import = "@/docs/targets/zed-reference.md#import"
set-up-this-tool = "@/docs/targets/zed.md#first-rule"
+++

# Zed (`zed`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Zed reads `.rules`, `.agents/skills/`, `.zed/settings.json` MCP servers, and optional tasks.

Set `outputs.zed.agents: skill` to write agents as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

## First rule

Follow the [official setup guide](https://zed.dev), select `zed` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). Sync writes the commit-prefix rule to `.rules`.

Existing files? Check [what `import zed` reads](@/docs/targets/zed-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Entry-point pointer body and inlined rules | `.rules` |
| Skills | `.agents/skills/<name>/SKILL.md` (shared tree) |
| MCP servers | `.zed/settings.json`, when entries exist (merged with your config) |
| Hooks as tasks | `.zed/tasks.json`, one task per hook, only when `tasks-file` is set |

Field mappings, config keys, and global instructions: [native reference](@/docs/targets/zed-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `.rules` for the commit-prefix rule.
3. Open the project in Zed and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Zed reads the first matching [instruction file](https://zed.dev/docs/ai/instructions). Nothing agnostic-ai writes ranks above `.rules`. See the [full list](@/docs/targets/zed-reference.md#native-mappings).
- Hooks become optional Zed Tasks. They do not enforce anything before a tool runs.
- Zed has no per-agent support. Sync reports this unless `outputs.zed.rules-file` is set. `outputs.zed.agents: skill` writes agents as on-demand skills ([agents as skills](@/docs/spec-format/agents.md#agents-as-skills)).
- Skill names are 1-64 lowercase letters or digits, with single hyphens between segments. `Deploy`, `my_skill`, and `my--skill` fail sync, and the error shows the format.

{% <details summary="More limits: Copilot, skills folder, restricted mode"> %}
- With copilot enabled, rules in `AGENTS.md` never reach Zed. `AGENTS.md` ranks below Copilot's pointer file.
- For older Zed, `outputs.zed.rules-file: .rules` writes the legacy single document.
- A separate `outputs.zed.skills-dir` keeps output apart during sync but adds no native search path. See [skills](@/docs/targets/zed-reference.md#native-mappings).
- `WorktreeCreate` tasks are skipped in restricted mode (Zed 1.28.1 and later). Trust the project first.
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
