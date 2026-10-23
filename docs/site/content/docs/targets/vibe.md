+++
title = "Mistral Vibe"
description = "Generate and import native Mistral Vibe instructions, agents, skills, connections, hooks, and model settings."
weight = 270

[extra]
group = "Reference"
target_id = "vibe"
+++

# Mistral Vibe (`vibe`)

Sync writes the files [Mistral Vibe Code CLI v3.0.2](https://github.com/mistralai/mistral-vibe/releases/tag/v3.0.2) reads. Start Vibe from the project root and make its trust choice yourself. The normal CLI loads project configuration only for trusted roots. In host sandbox or agent-socket mode, Vibe reads project instructions and skills but ignores project `.vibe` configuration, agents, and hooks.

## Set up this tool

Add `vibe` to your `targets` list. This example creates a model setting and a native hook that denies a tool call through Vibe's JSON response:

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
matcher: 're:bash|process_start'
command: "printf '%s\n' '{\"decision\":\"deny\",\"reason\":\"Review this command first\"}'"
YAML
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

The model must already exist in Vibe's model catalog or user configuration. Unknown model names fall back to Vibe's default. Sync does not create a model definition or change the primary agent.

## Native paths

| Spec | Project output | User output with `sync --global --only vibe` |
|---|---|---|
| Rules | `AGENTS.md`, with scoped instructions in deeper `AGENTS.md` files | `~/.vibe/AGENTS.md` |
| Agents | `.vibe/agents/<name>.toml` | `~/.vibe/agents/<name>.toml` |
| Skills and bundled files | `.vibe/skills/<name>/SKILL.md` | `~/.vibe/skills/<name>/SKILL.md` |
| MCP servers | `.vibe/config.toml`, `[[mcp_servers]]` | `~/.vibe/config.toml`, one entry per `name` |
| Hooks | `.vibe/hooks.toml`, `[[hooks]]` | `~/.vibe/hooks.toml`, one entry per `name` |
| Model settings | `.vibe/config.toml`, `active_model` | `~/.vibe/config.toml`, `active_model` |

`VIBE_HOME` moves every user path in this table. Project output overrides use `file`, `rules-file`, `agents-dir`, `skills-dir`, `conf-file`, `mcp-file`, and `hooks-file` under `outputs.vibe`. `conf-file` moves the shared model and MCP file unless `mcp-file` names a separate file. An override changes the generated path; configure Vibe to read a path outside its discovery locations.

Commands use the shared conversion to skills invoked by the user. Vibe has no native command file emitted by this target.

## Agents and skills

Agents emit as `agent_type = "subagent"`, with inline `instructions`, a description, and an optional `active_model`. A native `x-vibe.agent_type: agent` preserves a primary profile for explicit selection with `vibe --agent <name>`.

Portable agent tools map `Bash`, `Read`, `Write`, `Edit`, and `Grep` to `bash`, `read_file`, `write_file`, `edit`, and `grep`. Unresolved tools are omitted with a coverage note while supported tools keep the agent restricted. Sync fails if none of the requested tools can be mapped. Use `x-vibe.enabled_tools`, `x-vibe.disabled_tools`, and native `x-vibe.tools` permission tables for Vibe-specific policies. Empty `enabled_tools` would allow every tool, so sync rejects it. Portable `readonly` and permission patterns have no direct mapping. Vibe's `safety` field controls presentation, not access.

Skills keep `user-invocable` and `disable-model-invocation`, plus bundled files. Names use lowercase letters, digits, and single hyphens, at most 64 characters. Descriptions have at most 1024 characters. Vibe reads `allowed-tools` as descriptive metadata and does not enforce it.

## Connections, settings, and hooks

MCP names keep their original spelling. Vibe compares case-sensitive identities formed by replacing each character outside ASCII letters, digits, `_`, and `-` with `_`, stripping edge underscores and hyphens, then taking 256 characters. Empty or colliding identities stop sync and import.

MCP servers support stdio and streamable HTTP. HTTP headers use native static authentication under `[mcp_servers.auth]`. Preserve OAuth or environment-backed authentication with `x-vibe.auth`; for example, `type: static` and `api_key_env: DOCS_TOKEN`. Vibe reads that variable name at runtime. Arbitrary environment references in URLs, arguments, headers, and stdio environment values have no verified expansion and raise coverage notes. SSE and WebSocket servers are omitted with a note.

`model` maps to `active_model`. Other native project settings pass through `x-vibe`. Portable reasoning effort and permission settings raise coverage notes; Vibe's reasoning setting belongs to each model.

Hooks accept native `event: pre_tool`, `post_tool`, or `post_agent`. `matcher` maps to native `match`; `timeout` is in seconds. Use `x-vibe.strict: true` on tool hooks to deny or clear output when a hook fails. `post_agent` rejects `match` and `strict: true`. A shell guard should match both `bash` and `process_start`.

Hooks receive JSON on standard input and respond with Vibe JSON. Exit zero with `{"decision":"deny","reason":"..."}` denies according to the event: `pre_tool` blocks a call, `post_tool` rewrites its output, and `post_agent` requests a retry. A nonzero exit is a hook failure. Claude-style exit 2 and portable lifecycle events are not translated. Commands receive `AGNOSTIC_AI_TARGET=vibe`; arguments fold into the command because Vibe has no separate argument field. On Windows, arguments PowerShell cannot preserve cause an error; write the complete native command in that case.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).

## Import and ownership

Run `agnostic-ai import vibe` before syncing handwritten project files. It imports instructions, agent TOML, skill folders, MCP entries, model and native settings, and hook TOML. Native fields stay under `x-vibe`. Import refuses literal credentials in native settings, profiles, and connection fields; keep credentials in user configuration or use a native environment-variable setting. An agent that references a system prompt without inline instructions stops import; copy that prompt into `instructions` first.

Global sync adopts equal handwritten entries without rewriting them. It changes and removes only its recorded MCP and hook entries, preserving other entries, comments, and file permissions. A hand-edited owned entry stops sync. An equivalent inline array can be adopted, but changes require native `[[mcp_servers]]` or `[[hooks]]` tables. `import --global vibe` keeps unmanaged connections and hooks and excludes entries still equal to managed state. See [global configuration](@/docs/global-configuration.md) for source layout and commands.
