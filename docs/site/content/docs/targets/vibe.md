+++
title = "Mistral Vibe"
description = "Generate and import native Mistral Vibe instructions, agents, skills, connections, hooks, and model settings."
weight = 270

[extra]
group = "Reference"
target_id = "vibe"

[extra.moved]
set-up-this-tool = "@/docs/targets/vibe.md#first-rule"
native-paths = "@/docs/targets/vibe.md#output"
agents-and-skills = "@/docs/targets/vibe-reference.md"
connections-settings-and-hooks = "@/docs/targets/vibe-reference.md"
import-and-ownership = "@/docs/targets/vibe-reference.md"
+++

# Mistral Vibe (`vibe`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## Native paths {#native-paths .legacy-anchor}

## Agents and skills {#agents-and-skills .legacy-anchor}

## Connections, settings, and hooks {#connections-settings-and-hooks .legacy-anchor}

## Import and ownership {#import-and-ownership .legacy-anchor}

Sync writes the files [Mistral Vibe Code CLI v3.1.0](https://github.com/mistralai/mistral-vibe/releases/tag/v3.1.0) reads. Start Vibe from the project root and choose whether to trust it in Vibe.

## First rule

Add `vibe` to your `targets` list, then follow [First rule](@/docs/getting-started.md#first-rule). The shared commit-prefix rule becomes root `AGENTS.md`.

{% <details summary="Quick start with a model setting and a hook"> %}

This example creates a model setting and a native hook that denies a tool call through Vibe's JSON response:

```sh
mkdir -p .agnostic-ai/settings .agnostic-ai/hooks
cat > agnostic-ai.yaml <<'YAML'
version: 1
targets: [vibe]
YAML
cat > .agnostic-ai/settings/model.yaml <<'YAML'
model: devstral-small
YAML
cat > .agnostic-ai/hooks/guard.yaml <<'YAML'
name: guard
target: vibe
event: pre_tool
x-vibe:
  match: 're:bash|process_start'
command: |
  printf '%s\n' '{"decision":"deny","reason":"Review this command first"}'
YAML
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

The model must already exist in Vibe's model catalog or user configuration. Unknown model names fall back to Vibe's default. Sync does not create a model definition or change the primary agent.

{% </details> %}

Already have native files? Run `agnostic-ai import vibe` first; see [import and ownership](@/docs/targets/vibe-reference.md#import-and-ownership).

## Output

| Source | Project output | User output with `sync --global --only vibe` |
|---|---|---|
| Rules | `AGENTS.md`, with scoped instructions in deeper `AGENTS.md` files | `~/.vibe/AGENTS.md` |
| Agents | `.vibe/agents/<name>.toml` | `~/.vibe/agents/<name>.toml` |
| Skills and bundled files | `.vibe/skills/<name>/SKILL.md` | `~/.vibe/skills/<name>/SKILL.md` |
| MCP servers | `.vibe/config.toml`, `[[mcp_servers]]` | `~/.vibe/config.toml`, one entry per `name` |
| Hooks | `.vibe/hooks.toml`, `[[hooks]]` | `~/.vibe/hooks.toml`, one entry per `name` |
| Model settings | `.vibe/config.toml`, `active_model` | `~/.vibe/config.toml`, `active_model` |

`VIBE_HOME` moves every user path in this table. Project output overrides use `file`, `rules-file`, `agents-dir`, `skills-dir`, `conf-file`, `mcp-file`, and `hooks-file` under `outputs.vibe`. `conf-file` moves the shared model and MCP file unless `mcp-file` names a separate file. An override changes the generated path; configure Vibe to read a path outside its discovery locations.

Command specs become skills you run by name. This target writes no native Vibe command files. Field mappings for agents, skills, MCP, and hooks are in the [Vibe native reference](@/docs/targets/vibe-reference.md).

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Start Vibe from the project root, trust the project, and ask for a commit subject for a small fix; it should use `fix:`.

## Loading and limits

- The normal CLI loads project configuration only for trusted roots.
- In host sandbox or agent-socket mode, Vibe reads project instructions and skills but ignores project `.vibe` configuration, agents, and hooks.
- The model in a model setting must already exist in Vibe's catalog or user configuration; unknown names fall back to Vibe's default.
- Hooks answer with Vibe JSON, not Claude-style exit 2. Portable lifecycle events are not translated.

{% <details summary="More limits"> %}

- Vibe 3.0.3 reserves the agent name `main`, in any case, for hook matching and does not load a file with that name, so sync stops and asks you to rename the agent. Import notes such a file.
- A subagent runs on its own `active_model` only from v3.0.3.
- SSE and WebSocket MCP servers are omitted with a note. Arbitrary environment references in URLs, arguments, headers, and stdio environment values have no verified expansion and raise coverage notes.
- Portable `readonly` and permission patterns have no direct mapping, and portable reasoning effort and permission settings raise coverage notes.
- On Windows, hook arguments PowerShell cannot preserve cause an error; write the complete native command in that case.
- Hook events from Vibe 3.1.1 and later are `pre_tool`, `post_tool`, `pre_agent`, `post_agent`, `notification`, `session_start`, and `session_end`.

{% </details> %}

## Protected paths

Not enforced. This target cannot write a rule or hook that blocks edits. Sync prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

Next, see the [Vibe native reference](@/docs/targets/vibe-reference.md) for agents, skills, MCP, hooks, and import.
