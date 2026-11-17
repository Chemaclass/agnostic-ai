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

agnostic-ai writes Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) rules, agents, skills, and commands under `.qoder/`, and merges MCP, hooks, and settings into `.qoder/settings.json`.

## First rule

Follow the [official setup guide](https://qoder.com), open your project in the tool, select `qoder` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule). By default, sync writes the shared commit-prefix rule to `.qoder/rules/conventional-commits.md`, or to shared `AGENTS.md` when another selected target already puts the rule there.

Keep `AGENTS.md` in the CLI context file list so Qoder can read the shared copy. For existing files, check [what `import qoder` reads](@/docs/targets/qoder-reference.md#import) before syncing.

## Output

| Source | Default output |
|---|---|
| Shared instructions | `AGENTS.md` (entry-point pointer body, plus the rules block when another tool adds it) |
| Rules | `.qoder/rules/<name>.md`, for each rule `AGENTS.md` does not carry or that is not always-on |
| Agents | `.qoder/agents/<name>.md` |
| Skills and bundled files | `.qoder/skills/<name>/SKILL.md` |
| Commands | `.qoder/commands/<name>.md` |
| MCP servers, hooks, settings | `.qoder/settings.json` (merged; other keys are kept) |

For output overrides and field mappings, see the [native reference](@/docs/targets/qoder-reference.md).

## Verify

Run `agnostic-ai sync --check` and find the commit-prefix rule in `.qoder/rules/conventional-commits.md` or the shared `AGENTS.md`. Open the project in Qoder. Confirm the rule is active, then ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- Qoder reads `.qoder/rules/` and the root `AGENTS.md`. In the IDE, per-rule files win conflicts, so sync writes rules there. The CLI loads both and documents no order. See [rules](@/docs/targets/qoder-reference.md#rules).
- Rule activation modes (always, manual, model-selected, file matching) follow the rule's frontmatter; see [rule activation](@/docs/targets/qoder-reference.md#rule-activation).
- Hooks and MCP servers merge into `.qoder/settings.json`; Qoder takes 27 hook events, and `once` has no effect. See [hooks](@/docs/targets/qoder-reference.md#hooks).
- With `handoff` or `handoff-hook`, linked worktrees get shared handoff write access through `permissions.additionalDirectories` in the ignored `.qoder/settings.json`. See [shared handoff access](@/docs/handoff.md#write-from-a-linked-worktree) for preserved configuration and fallback behavior.

{% <details summary="More limits"> %}
- A `type: ws` MCP server gets only a sync note, and JSONC comments in `.qoder/settings.json` are dropped with a warning; see [MCP](@/docs/targets/qoder-reference.md#mcp).
- The `memory` agent field only works when `autoMemoryEnabled` is on in Qoder settings (default `false`). See [subagent memory](@/docs/targets/qoder-reference.md#subagent-memory).
- Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. Check [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).
- A leftover root `.mcp.json` from older versions still loads and wins over `.qoder/settings.json` for a server with the same name. Delete it by hand in a Qoder-only project; see [hooks and leftover files](@/docs/targets/qoder-reference.md#hooks).
{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md).
