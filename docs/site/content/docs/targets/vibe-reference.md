+++
title = "Mistral Vibe native reference"
description = "Native field mappings, connections, hooks, and import coverage for vibe."
weight = 271

[extra]
nav_title = "Mistral Vibe reference"
group = "Reference"
+++

# Mistral Vibe native reference

Start with the [Vibe setup](@/docs/targets/vibe.md).

## Agents and skills

Agents emit as `agent_type = "subagent"`, with inline `instructions`, a description, and an optional `active_model`; a subagent runs on that model only from v3.0.3. A native `x-vibe.agent_type: agent` preserves a primary profile for explicit selection with `vibe --agent <name>`. Vibe 3.0.3 reserves the agent name `main`, in any case, for hook matching and does not load a file with that name, so sync stops and asks you to rename the agent. Import notes such a file.

Portable agent tools map `Bash`, `Read`, `Write`, `Edit`, and `Grep` to `bash`, `read_file`, `write_file`, `edit`, and `grep`. Unresolved tools are omitted with a coverage note while supported tools keep the agent restricted. Sync fails if none of the requested tools can be mapped. Use `x-vibe.enabled_tools`, `x-vibe.disabled_tools`, and native `x-vibe.tools` permission tables for Vibe-specific policies. Empty `enabled_tools` would allow every tool, so sync rejects it. Portable `readonly` and permission patterns have no direct mapping. Vibe's `safety` field controls presentation, not access.

Skills keep `user-invocable` and `disable-model-invocation`, plus bundled files. Names use lowercase letters, digits, and single hyphens, at most 64 characters. Descriptions have at most 1024 characters. Vibe reads `allowed-tools` as descriptive metadata and does not enforce it.

## Connections, settings, and hooks

MCP names keep their original spelling. Vibe compares case-sensitive identities formed by replacing each character outside ASCII letters, digits, `_`, and `-` with `_`, stripping edge underscores and hyphens, then taking 256 characters. Empty or colliding identities stop sync and import.

MCP servers support stdio and streamable HTTP. HTTP headers use native static authentication under `[mcp_servers.auth]`. Preserve OAuth or environment-backed authentication with `x-vibe.auth`; for example, `type: static` and `api_key_env: DOCS_TOKEN`. Vibe reads that variable name at runtime. Arbitrary environment references in URLs, arguments, headers, and stdio environment values have no verified expansion and raise coverage notes. SSE and WebSocket servers are omitted with a note.

`model` maps to `active_model`. Other native project settings pass through `x-vibe`. Portable reasoning effort and permission settings raise coverage notes; Vibe's reasoning setting belongs to each model.

At user scope, portable [voice settings](@/docs/spec-format/settings.md#voice-dictation) write `voice_mode_enabled`; Vibe's experimental dictation has no mode or language key.

Hooks accept native `event: pre_tool`, `post_tool`, `pre_agent`, `post_agent`, `notification`, `session_start`, or `session_end` (Vibe 3.1.1 and later). `matcher` maps to native `match`; `timeout` is in seconds. `x-vibe.strict: true` is allowed only on `pre_tool`, `post_tool`, and `pre_agent`, matching the [released hook schema](https://github.com/mistralai/mistral-vibe/blob/97e31350d2adc15c8a33a091566e897cbe9f41bc/vibe/core/hooks/models.py). On `post_agent`, `match` runs the hook only after the named agent ends its turn, and `match: main` selects the main agent (Vibe 3.0.3 and later). The other four events reject `strict: true`. Native `match` values pass through unchanged. A shell guard should match both `bash` and `process_start`.

Hooks receive JSON on standard input and respond with Vibe JSON. Exit zero with `{"decision":"deny","reason":"..."}` denies according to the event: `pre_tool` blocks a call, `post_tool` rewrites its output, and `post_agent` requests a retry. A nonzero exit is a hook failure. Claude-style exit 2 and portable lifecycle events are not translated. Commands receive `AGNOSTIC_AI_TARGET=vibe`; arguments fold into the command because Vibe has no separate argument field. On Windows, arguments PowerShell cannot preserve cause an error; write the complete native command in that case.

## Import and ownership

Run `agnostic-ai import vibe` before syncing handwritten project files. It imports instructions, agent TOML, skill folders, MCP entries, model and native settings, and hook TOML. Native fields stay under `x-vibe`. Skill import moves native `allowed-tools` there too, retaining scalar or list form and names such as `read_file` and `ask_user_question`. These names remain descriptive Vibe metadata. Import refuses literal credentials in native settings, profiles, and connection fields; keep credentials in user configuration or use a native environment-variable setting. An agent that references a system prompt without inline instructions stops import; copy that prompt into `instructions` first.

Global sync adopts equal handwritten entries without rewriting them. Omitted `sampling_enabled` compares as `false`, Vibe's native default; a handwritten `true` conflicts unless the spec sets `x-vibe.sampling_enabled: true`.

If an earlier sync adopted `sampling_enabled: true` from a spec that omitted sampling, sync stops before changing the file. Set `x-vibe.sampling_enabled: true` to keep it or `false` to disable it.

Sync changes and removes only its recorded MCP and hook entries. Other entries, comments, and file permissions stay. A hand-edited owned entry stops sync. An equivalent inline array can be adopted, but changes require native `[[mcp_servers]]` or `[[hooks]]` tables. `import --global vibe` keeps unmanaged connections and hooks and excludes entries still equal to managed state. See [global configuration](@/docs/global-configuration.md) for source layout and commands.
