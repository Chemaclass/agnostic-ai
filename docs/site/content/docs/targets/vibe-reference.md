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

### Agents

- Agents are written as `agent_type = "subagent"` with inline `instructions`, a description, and an optional `active_model`. A subagent runs on that model only from v3.0.3.
- `x-vibe.agent_type: agent` keeps a primary profile, selected with `vibe --agent <name>`.
- Vibe 3.0.3 reserves the name `main` (any case) for hook matching and does not load a file with that name. Sync stops and asks you to rename the agent. Import notes such a file.
- Portable tools map `Bash`, `Read`, `Write`, `Edit`, `Grep` to `bash`, `read_file`, `write_file`, `edit`, `grep`.
- Tools that cannot be mapped are omitted with a note, and the rest keep the agent restricted. If none can be mapped, sync fails.
- For Vibe-specific policy use `x-vibe.enabled_tools`, `x-vibe.disabled_tools`, and native `x-vibe.tools` permission tables.
- Empty `enabled_tools` would allow every tool, so sync rejects it.
- Portable `readonly` and permission patterns have no mapping. Vibe's `safety` field controls presentation, not access.

### Skills

- Skills keep `user-invocable`, `disable-model-invocation`, and bundled files.
- Skill descriptions accept 1 to 1,024 Unicode characters. Accented letters and other multi-byte characters count as one character each, matching the [native metadata limit](https://github.com/mistralai/mistral-vibe/blob/v3.1.2/vibe/core/skills/models.py).
- Names: lowercase letters, digits, and single hyphens, up to 64 characters.
- `allowed-tools` is descriptive metadata. Vibe does not enforce it.

## Connections, settings, and hooks

### MCP servers

- Names keep their original spelling.
- Vibe compares case-sensitive identities: each character outside ASCII letters, digits, `_`, `-` becomes `_`, edge underscores and hyphens are stripped, then the first 256 characters are kept. An empty or colliding identity stops sync and import.
- Supported: stdio and streamable HTTP. SSE and WebSocket are omitted with a note.
- HTTP headers use native static authentication under `[mcp_servers.auth]`.
- For OAuth or environment-backed authentication use `x-vibe.auth`, for example `type: static` and `api_key_env: DOCS_TOKEN`. Vibe reads that variable name at runtime.
- Environment references in URLs, arguments, headers, and stdio environment values have no verified expansion and give coverage notes.

### Settings

- `model` maps to `active_model`. Other native project settings pass through `x-vibe`.
- Portable reasoning effort and permission settings give coverage notes. Vibe's reasoning setting belongs to each model.
- User scope: `approvals` set to `review` for Vibe writes `default_agent = "smart-approve"` to `~/.vibe/config.toml`. See [review through the tool's own reviewer](@/docs/global-configuration-reference.md#global-approvals-review).
- User scope: portable [voice settings](@/docs/spec-format/settings-reference.md#voice-dictation) write `voice_mode_enabled`. Vibe's experimental dictation has no mode or language key.

### Hooks

- Vibe 3.1.1 supports `pre_tool`, `post_tool`, `pre_agent`, `post_agent`, `notification`, `session_start`, and `session_end`.
- [Vibe 3.1.3](https://github.com/mistralai/mistral-vibe/releases/tag/v3.1.3) adds `pre_llm` before each model request and `post_llm` after each model response.
- `matcher` maps to native `match`. Native `match` values pass through. A shell guard should match both `bash` and `process_start`.
- `timeout` is in seconds.
- Set `x-vibe.strict: true` on `pre_tool`, `post_tool`, `pre_agent`, `pre_llm`, or `post_llm`. See the [Vibe 3.1.3 hook schema](https://github.com/mistralai/mistral-vibe/blob/63dab302305c80131ea1e5b1c3cb18c2a4fd96d3/vibe/core/hooks/models.py).
- On `post_agent`, `match` runs the hook only after the named agent ends its turn. `match: main` selects the main agent (Vibe 3.0.3 and later).
- Hooks get JSON on standard input and answer with Vibe JSON. Exit zero with `{"decision":"deny","reason":"..."}` denies by event: `pre_tool` blocks the call, `post_tool` rewrites its output, `post_agent` requests a retry.
- A nonzero exit is a hook failure. Claude-style exit 2 and portable lifecycle events are not translated.
- Commands get `AGNOSTIC_AI_TARGET=vibe`. Arguments fold into the command, because Vibe has no argument field.
- Windows: arguments PowerShell cannot preserve cause an error. Write the full native command instead.

## Import and ownership

### Import

Run `agnostic-ai import vibe` before syncing handwritten project files.

- Reads: instructions, agent TOML, skill folders, MCP entries, model and native settings, hook TOML.
- Native fields stay under `x-vibe`. That includes skill `allowed-tools`, kept as a scalar or list, with names such as `read_file` and `ask_user_question`. They stay descriptive metadata.
- Literal credentials in native settings, profiles, and connection fields are refused. Keep credentials in user configuration or use a native environment-variable setting.
- An agent that points at a system prompt without inline instructions stops import. Copy the prompt into `instructions` first.

### Ownership

- User sync adopts equal handwritten entries without rewriting them.
- An omitted `sampling_enabled` compares as `false`, Vibe's native default. A handwritten `true` conflicts unless the spec sets `x-vibe.sampling_enabled: true`.
- If an earlier sync adopted `sampling_enabled: true` from a spec that omitted it, sync stops before changing the file. Set `x-vibe.sampling_enabled` to `true` to keep it or `false` to turn it off.
- Sync changes and removes only its recorded MCP and hook entries. Other entries, comments, and file permissions stay.
- A hand-edited owned entry stops sync.
- An equivalent inline array can be adopted. Changes need native `[[mcp_servers]]` or `[[hooks]]` tables.
- `import --scope user vibe` keeps unmanaged connections and hooks and skips entries still equal to managed state.
- Source layout and commands: [user configuration](@/docs/global-configuration.md).
