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

Sync writes the files [Mistral Vibe Code CLI v3.1.0](https://github.com/mistralai/mistral-vibe/releases/tag/v3.1.0) reads. Start Vibe from the project root and decide in Vibe whether to trust the project.

## First rule

1. Add `vibe` to `targets` in `agnostic-ai.yaml`.
2. Follow [First rule](@/docs/getting-started.md#first-rule). The commit-prefix rule becomes root `AGENTS.md`.

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

The model must already exist in Vibe's catalog or your user configuration. Sync does not create a model definition or change the primary agent.

{% </details> %}

Already have native files? Run `agnostic-ai import vibe` first ([import and ownership](@/docs/targets/vibe-reference.md#import-and-ownership)).

## Output

| Source | Project output | User output with `sync --scope user --only vibe` |
|---|---|---|
| Rules | `AGENTS.md`, scoped ones in deeper `AGENTS.md` files | `~/.vibe/AGENTS.md` |
| Agents | `.vibe/agents/<name>.toml` | `~/.vibe/agents/<name>.toml` |
| Skills and bundled files | `.vibe/skills/<name>/SKILL.md` | `~/.vibe/skills/<name>/SKILL.md` |
| MCP servers | `.vibe/config.toml`, `[[mcp_servers]]` | `~/.vibe/config.toml`, one entry per `name` |
| Hooks | `.vibe/hooks.toml`, `[[hooks]]` | `~/.vibe/hooks.toml`, one entry per `name` |
| Model settings | `.vibe/config.toml`, `active_model` | `~/.vibe/config.toml`, `active_model` |

`VIBE_HOME` moves every user path. Commands become skills you run by name; no native command files are written. Field mappings: [Vibe native reference](@/docs/targets/vibe-reference.md).

{% <details summary="Output overrides"> %}
- Keys under `outputs.vibe`: `file`, `rules-file`, `agents-dir`, `skills-dir`, `conf-file`, `mcp-file`, `hooks-file`.
- `conf-file` moves the shared model and MCP file, unless `mcp-file` names a separate file.
- An override changes only the generated path. A path outside Vibe's discovery locations needs Vibe configured to read it.
{% </details> %}

## Verify

1. Run `agnostic-ai sync --check`.
2. Check `AGENTS.md` for the commit-prefix rule.
3. Start Vibe from the project root, trust the project, and ask for a commit subject for a small fix. It should use `fix:`.

## Loading and limits

- The normal CLI loads project configuration only for trusted roots.
- In host sandbox or agent-socket mode, Vibe reads project instructions and skills but ignores project `.vibe` configuration, agents, and hooks.
- A model setting must name a model already in Vibe's catalog or your user configuration. Unknown names fall back to Vibe's default.

{% <details summary="More limits"> %}
- Vibe 3.0.3 reserves the agent name `main` (any case) for hook matching and does not load a file with that name. Sync stops and asks you to rename the agent. Import notes such a file.
- A subagent runs on its own `active_model` only from v3.0.3.
- SSE and WebSocket MCP servers are omitted with a note.
- Environment references in MCP URLs, arguments, headers, and stdio environment values have no verified expansion and give coverage notes.
- Portable `readonly` and permission patterns have no mapping. Portable reasoning effort and permission settings give coverage notes.
- Hooks answer with Vibe JSON, not Claude-style exit 2. Portable lifecycle events are not translated. Events and Windows rules: [reference](@/docs/targets/vibe-reference.md#connections-settings-and-hooks).
{% </details> %}

## Protected paths

Not enforced. The portable `protected` setting produces a [coverage note](@/docs/spec-format/settings.md#protected-paths), without generating a blocking hook. Vibe documents `pre_tool` hooks that deny tool calls through a JSON response ([native hooks](https://docs.mistral.ai/vibe/code/cli/hooks)). Write a separate [hook spec](@/docs/targets/vibe-reference.md#connections-settings-and-hooks) for a custom guard.

Next, see the [Vibe native reference](@/docs/targets/vibe-reference.md) for agents, skills, MCP, hooks, and import.
