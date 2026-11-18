+++
title = "Qoder"
description = "Qoder: generated files, supported features, and output settings."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"

[extra.moved]
set-up-this-tool = "@/docs/targets/qoder.md#first-rule"
native-mappings = "@/docs/targets/qoder-reference.md"
rules = "@/docs/targets/qoder-reference.md"
agents = "@/docs/targets/qoder-reference.md"
skills = "@/docs/targets/qoder-reference.md"
commands = "@/docs/targets/qoder-reference.md"
mcp = "@/docs/targets/qoder-reference.md"
settings = "@/docs/targets/qoder-reference.md"
hooks = "@/docs/targets/qoder-reference.md"
config-keys = "@/docs/targets/qoder-reference.md"
rule-activation = "@/docs/targets/qoder-reference.md"
subagent-memory = "@/docs/targets/qoder-reference.md"
auto-memory = "@/docs/targets/qoder-reference.md"
import = "@/docs/targets/qoder-reference.md"
+++

# Qoder (`qoder`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Rules {#rules .legacy-anchor}

## Agents {#agents .legacy-anchor}

## Skills {#skills .legacy-anchor}

## Commands {#commands .legacy-anchor}

## MCP {#mcp .legacy-anchor}

## Settings {#settings .legacy-anchor}

## Hooks {#hooks .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Rule activation {#rule-activation .legacy-anchor}

## Subagent memory {#subagent-memory .legacy-anchor}

## Auto memory {#auto-memory .legacy-anchor}

## Import {#import .legacy-anchor}

agnostic-ai writes Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) rules, agents, skills, and commands under `.qoder/`. It merges MCP, hooks, and settings into `.qoder/settings.json`.

## First rule

1. Open your project in Qoder ([setup guide](https://qoder.com)).
2. Add `qoder` to `targets` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule).
4. Keep `AGENTS.md` in the CLI context file list, so Qoder can read the shared copy.

Sync writes the commit-prefix rule to `.qoder/rules/conventional-commits.md`. If another selected target already puts it in `AGENTS.md`, it goes there instead. Have existing files? Read [what `import qoder` reads](@/docs/targets/qoder-reference.md#import) first.

## Output

| Source | Default output |
|---|---|
| Shared instructions | `AGENTS.md` (pointer body, plus the rules block when another tool adds it) |
| Rules | `.qoder/rules/<name>.md` (skipped when `AGENTS.md` carries an always-on rule) |
| Agents | `.qoder/agents/<name>.md` |
| Skills and bundled files | `.qoder/skills/<name>/SKILL.md` |
| Commands | `.qoder/commands/<name>.md` |
| MCP servers, hooks, settings | `.qoder/settings.json` (merged; other keys are kept) |

Field mappings and output overrides: [native reference](@/docs/targets/qoder-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Open the project in Qoder and confirm the commit-prefix rule is active.
3. Ask for a commit subject for a small fix. It should start with `fix:`.

## Loading and limits

- Rules: the IDE lets per-rule files win, so sync writes them there ([rules](@/docs/targets/qoder-reference.md#rules)).
- Activation modes follow rule frontmatter ([rule activation](@/docs/targets/qoder-reference.md#rule-activation)).
- Hooks and MCP merge into `.qoder/settings.json` ([hooks](@/docs/targets/qoder-reference.md#hooks)).

{% <details summary="More limits"> %}
- The CLI loads `.qoder/rules/` and `AGENTS.md` and documents no order between them.
- Qoder takes 27 hook events. `once` has no effect.
- With `handoff` or `handoff-hook`, linked worktrees get shared handoff write access through `permissions.additionalDirectories` in the ignored `.qoder/settings.json`. See [shared handoff access](@/docs/handoff.md#write-from-a-linked-worktree).
- A `type: ws` MCP server gets only a sync note ([MCP](@/docs/targets/qoder-reference.md#mcp)).
- JSONC comments in `.qoder/settings.json` are dropped with a warning.
- The `memory` agent field needs `autoMemoryEnabled` on in Qoder settings (default `false`) ([subagent memory](@/docs/targets/qoder-reference.md#subagent-memory)).
- `sync.unmanaged: [AGENTS.md]` makes every rule keep its file ([effect on other targets](@/docs/target-behavior.md#inlined-rules)).
- A leftover root `.mcp.json` from older versions still loads and wins for a server with the same name. Delete it by hand in a Qoder-only project ([details](@/docs/targets/qoder-reference.md#hooks)).
{% </details> %}

## Protected paths

Not enforced. The portable `protected` setting produces a [coverage note](@/docs/spec-format/settings.md#protected-paths), without generating a blocking hook. Qoder documents `PreToolUse` hooks that block tool calls with exit code 2 ([native hooks](https://docs.qoder.com/cli/hooks#pretooluse)). Write a separate [hook spec](@/docs/targets/qoder-reference.md#hooks) for a custom guard.

Next, [add a skill](@/docs/spec-format/skills.md).
