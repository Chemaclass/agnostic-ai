+++
title = "Warp"
description = "Warp: generated files, supported features, and output settings."
weight = 120

[extra]
group = "Reference"
target_id = "warp"

[extra.moved]
set-up-this-tool = "@/docs/targets/warp.md#first-rule"
native-mappings = "@/docs/targets/warp-reference.md"
config-keys = "@/docs/targets/warp.md#output"
global-configuration = "@/docs/targets/warp-reference.md"
voice-dictation = "@/docs/targets/warp-reference.md"
app-and-cli = "@/docs/targets/warp-reference.md"
import = "@/docs/targets/warp-reference.md"
+++

# Warp (`warp`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native mappings {#native-mappings .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Global configuration {#global-configuration .legacy-anchor}

## Voice dictation {#voice-dictation .legacy-anchor}

## Warp app and Agent CLI {#app-and-cli .legacy-anchor}

## Import {#import .legacy-anchor}

Warp reads `AGENTS.md`, `.agents/skills/`, and `.warp/.mcp.json`, plus optional workflows. Sync writes them from your specs.

## First rule

1. Install Warp with the [official guide](https://www.warp.dev) and open your project.
2. Select `warp` in `agnostic-ai.yaml`.
3. Follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. It lands in `AGENTS.md`.

Already have Warp files? See [what `import warp` reads](@/docs/targets/warp-reference.md#import) first.

To write agents as on-demand skills, set `outputs.warp.agents: skill` ([agents as skills](@/docs/spec-format/agents.md#agents-as-skills)). Workflow output runs the agent body as a shell command, so use skills for written procedures.

## Output

| Source | Default output |
|---|---|
| Instructions and rules | `AGENTS.md` (pointer body written by sync, shared by every `AGENTS.md` tool) |
| Skills | `.agents/skills/<name>/SKILL.md`, one folder per skill (shared tree) |
| Workflows | `.warp/workflows/<name>.yaml`, one per agent, only when `workflows-dir` is set |
| MCP servers | `.warp/.mcp.json`, when MCP entries exist |

{% <details summary="Output settings"> %}

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.warp.skills-dir` | `.agents/skills` | |
| `outputs.warp.agents` | unset (agents are dropped) | `skill` writes each agent as an on-demand skill. Sync keeps it off with a note while another tool with subagents writes the same skills folder. |
| `outputs.warp.workflows-dir` | empty | opt-in |
| `outputs.warp.mcp-file` | `.warp/.mcp.json` | |
| `outputs.warp.rules-file` | unset | writes all rules into one legacy file and skips instructions that point to source specs |

{% </details> %}

Field mappings, global configuration, and import: [Warp native reference](@/docs/targets/warp-reference.md).

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `AGENTS.md` for the commit-prefix rule.
3. In Warp, ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- Warp still supports `WARP.md` and prefers it over `AGENTS.md` in a directory with both. See [upgrading from WARP.md](@/docs/targets/warp-reference.md#upgrading-from-warp-md).
- Warp also loads MCP servers from Claude Code's `.mcp.json` and Codex's `.codex/config.toml`, after approval. With `claude` or `codex` synced too, approve only one copy.
- Global MCP servers reach the Warp app only, not the Agent CLI ([details](@/docs/targets/warp-reference.md#app-and-cli)).

{% <details summary="More limits"> %}
- No skill can be kept user-only, so `disable-model-invocation` is dropped with a note and commands are skipped.
- Warp scans more skill folders than agnostic-ai writes; the [list is in the reference](@/docs/targets/warp-reference.md#skills).
- `description`, `disabled`, and `roots` are not written to MCP output. Set `description` and `roots` through `x-warp`. `disabled` gives a sync note.
- The MCP file has no transport field. Remote servers carry no `type`. A stdio server with no arguments gets `"args": []`.
{% </details> %}

## Protected paths

Not enforced. This target takes no settings specs. Sync reports a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).

Next, [add a skill](@/docs/spec-format/skills.md) or see the [Warp native reference](@/docs/targets/warp-reference.md).
