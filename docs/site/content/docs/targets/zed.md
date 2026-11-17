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

Follow the [official setup guide](https://zed.dev), select `zed` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the shared commit-prefix rule to `.rules`.

For existing files, check [what `import zed` reads](@/docs/targets/zed-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Entry-point pointer body and inlined rules | `.rules` |
| Skills | `.agents/skills/<name>/SKILL.md`, one folder per skill (shared tree) |
| MCP servers | `.zed/settings.json`, when entries exist (merged with existing user config) |
| Hooks as tasks | `.zed/tasks.json`, one task per hook, only when `tasks-file` is set |

For field mappings, config keys, and global instructions, see the [native reference](@/docs/targets/zed-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `.rules` for the commit-prefix rule. Open the project in Zed and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Zed reads [the first matching file](https://zed.dev/docs/ai/instructions) of `.rules`, `.cursorrules`, `.windsurfrules`, `.clinerules`, `.github/copilot-instructions.md`, `AGENT.md`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`. Nothing agnostic-ai writes ranks above `.rules`. With copilot enabled, rules in `AGENTS.md` never reach Zed.
- Hooks become optional Zed Tasks. They do not provide general before-tool enforcement.
- Zed has no per-agent support. Sync reports this unless `outputs.zed.rules-file` is set. `outputs.zed.agents: skill` writes agents as on-demand skills.
- Skill names must be 1-64 lowercase letters or digits, with single hyphens between segments. Sync fails on names such as `Deploy`, `my_skill`, and `my--skill` and states the format. It does not rename them.

{% <details summary="More limits"> %}
- `AGENTS.md` ranks below Copilot's file that only points to source specs. Zed calls `AGENTS.md` primary and `.rules` a compatibility file; if Zed drops `.rules`, the rules move back to `AGENTS.md`. For older Zed, `outputs.zed.rules-file: .rules` writes the legacy single document.
- A separate `outputs.zed.skills-dir` keeps the output apart during sync, but does not add a native search path. Zed loads skills only as direct children of `~/.agents/skills/` or `<worktree>/.agents/skills/`. To use another backing folder, expose it through a symlink at a documented root; sync does not create the link.
- `WorktreeCreate` tasks are skipped while the project is in restricted mode (Zed 1.28.1 and later), so trust the project first.
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
