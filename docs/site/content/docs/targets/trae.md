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

1. Open your project in Trae ([setup guide](https://www.trae.ai)).
2. Add `trae` to `targets` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule). Sync writes the commit-prefix rule to `.trae/rules/conventional-commits.md`.
4. In Trae settings, turn on project hooks and the subagents directory if you use them. Syncing their files does not enable them.

Have existing files? Read [what `import trae` reads](@/docs/targets/trae-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Instructions | `AGENTS.md` (pointer body, plus the rules block when another tool that inlines rules shares it) |
| Rules | `.trae/rules/<name>.md` |
| Agents (project subagents) | `.trae/agents/<name>.md` |
| Skills | `.trae/skills/<name>/SKILL.md` |
| Commands | `.trae/commands/<name>.md` |
| Hooks | `.trae/hooks.json`, when hook entries exist |
| Ignore entries | `.trae/.ignore`, when ignore entries exist |
| MCP servers | `.trae/mcp.json` |

Field mappings, config keys, and shared memory: [native reference](@/docs/targets/trae-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Open the project in Trae and confirm the rule is active in the Rules panel.
3. Ask for a commit subject for a small fix. It should start with `fix:`.

With `outputs.trae.rules-mode: entry-point` and Codex enabled, the rule is in the `## Rules` block of `AGENTS.md` instead.

## Loading and limits

- Trae always loads its own rule files. A project rule is [effective for all AI chats in the project](https://docs.trae.ai/ide/rules).
- Trae reads `AGENTS.md` only when **Include AGENTS.md in the context** is on (Settings > Rules & Memories). The docs give no default, so sync treats it as off.
- Project hooks do nothing until you enable them under Settings > Hooks and accept the security warning ([hooks](@/docs/targets/trae-reference.md#hooks)).

{% <details summary="More limits"> %}
- With the switch off, every rule keeps its `.trae/rules/` file. `lint` and `explain --context` do not count `AGENTS.md` for Trae.
- With the switch on, set `outputs.trae.rules-mode: entry-point`. When Codex or another tool writes the `## Rules` block in `AGENTS.md`, sync skips the `.trae/rules/` file of each always-on rule that block holds, so the rule loads once (#3230). Scoped rules keep their files.
- Trae also runs Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json` once you turn on **Import Hook configuration in CLAUDE** (off by default, same warning). Syncing `claude` and `trae` then runs every hook twice.
- Subagents need Settings > Beta > Subagents > Enable Subagents Directory. Trae does not say whether it is on by default.
{% </details> %}

## Protected paths

Not enforced. Trae takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
