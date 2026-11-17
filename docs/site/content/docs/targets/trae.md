+++
title = "Trae"
description = "Trae: generated files, supported features, and output settings."
weight = 180

[extra]
group = "Reference"
target_id = "trae"

[extra.moved]
native-mappings = "@/docs/targets/trae-reference.md"
rules = "@/docs/targets/trae-reference.md#rules"
agents = "@/docs/targets/trae-reference.md#agents"
skills = "@/docs/targets/trae-reference.md#skills"
commands = "@/docs/targets/trae-reference.md#commands"
mcp-servers = "@/docs/targets/trae-reference.md#mcp-servers"
hooks = "@/docs/targets/trae-reference.md#hooks"
ignore = "@/docs/targets/trae-reference.md#ignore"
shared-memory = "@/docs/targets/trae-reference.md#shared-memory"
config-keys = "@/docs/targets/trae-reference.md#config-keys"
import = "@/docs/targets/trae-reference.md#import"
loaded-at-session-start = "@/docs/targets/trae.md#loading-and-limits"
set-up-this-tool = "@/docs/targets/trae.md#first-rule"
+++

# Trae (`trae`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads `.trae/rules/`, the root `AGENTS.md`, and project subagents in `.trae/agents/`.

## First rule

Follow the [official setup guide](https://www.trae.ai), select `trae` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the shared commit-prefix rule to `.trae/rules/conventional-commits.md`.

Project hooks and subagent directories need separate switches in Trae settings. Syncing their files does not enable those features. For existing files, check [what `import trae` reads](@/docs/targets/trae-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Instructions | `AGENTS.md`: pointer body, plus the rules block when another tool that inlines rules shares it |
| Rules | `.trae/rules/<name>.md` |
| Agents (project subagents) | `.trae/agents/<name>.md` |
| Skills | `.trae/skills/<name>/SKILL.md`, one folder per skill |
| Commands | `.trae/commands/<name>.md` |
| Hooks | `.trae/hooks.json`, when hook entries exist |
| Ignore entries | `.trae/.ignore`, when ignore entries exist |
| MCP servers | `.trae/mcp.json` |

For field mappings, config keys, and shared memory, see the [native reference](@/docs/targets/trae-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `.trae/rules/conventional-commits.md`. Open the project in Trae, confirm the rule is active in the Rules panel, then ask for a commit subject for a small fix; it should use `fix:`.

With `outputs.trae.rules-mode: entry-point` and Codex enabled, the rule is in the `## Rules` block of `AGENTS.md` instead.

## Loading and limits

- Trae always loads its own rule files. The [rules docs](https://docs.trae.ai/ide/rules) say a project rule "is effective for all AI chats within the current project".
- Trae reads `AGENTS.md` only after you turn on **Include AGENTS.md in the context** in Import Settings. The docs give no default, so sync treats it as off: every rule keeps its `.trae/rules/` file, and `lint` and `explain --context` do not count `AGENTS.md` for Trae.
- If you turn the switch on, set `outputs.trae.rules-mode: entry-point`. When Codex or another tool writes the `## Rules` block in `AGENTS.md`, sync then skips the `.trae/rules/` file of each always-on rule that block already holds, so the rule loads once (#3230). Scoped rules keep their files.
- Project hooks do nothing until you enable them under Settings > Hooks and accept the security warning.

{% <details summary="More limits"> %}
- Trae also runs Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json` once you turn on **Import Hook configuration in CLAUDE** (off by default, same warning). Syncing `claude` and `trae` then runs every hook twice.
- Subagents need Settings > Beta > Subagents > Enable Subagents Directory. Trae does not say whether it is on by default.
- Trae reads `AGENTS.md` only when **Include AGENTS.md in the context** is on (Settings > Rules & Memories).
{% </details> %}

## Protected paths

Not enforced. Trae takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
