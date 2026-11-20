+++
title = "Crush native reference"
description = "Native field mappings, MCP and hook output, config keys, and import coverage for crush."
weight = 171

[extra]
nav_title = "Crush reference"
group = "Reference"
+++

# Crush native reference

Start with the [Crush setup](@/docs/targets/crush.md).

## Native mappings

- **Rules**: inlined into `AGENTS.md`. Crush has no per-rule directory and no directory scope. Sync skips scoped rules; import cannot recover scope. See [scoped context](@/docs/scoped-context.md).
- **Skills**: written to `.agents/skills/`, the first project path Crush scans. Targets sharing the folder get identical files, written once. `x-crush.user-invocable: true` also adds a skill to the command palette (ctrl+p).
- **Agents**: skipped with a warning. `outputs.crush.agents: skill` converts them to skills.
- **`crush.json`**: sync keeps your `models`, `providers`, `lsp`, and `options` keys.
  - Crush prefers `crushrc`, a Bash startup script. Crush 0.102.0 also accepts [`options.prompts`](https://github.com/charmbracelet/crush/blob/v0.102.0/docs/config/README.md#option-prompt) in `crush.json`. Add that block directly to the native JSON file and sync keeps it.
  - JSON is deprecated but works. Sync writes `crush.json`, never `crushrc`.
  - An MCP field that exists only in `crushrc` cannot be set here.
- **Ignore**: `.crushignore` uses gitignore syntax ([Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md)). Hand-written files get the usual protection.

### User-only skills

`disable-model-invocation: true` is written with `user-invocable: true`. Crush then hides the skill from the model but keeps it in the command palette ([Crush README](https://github.com/charmbracelet/crush/blob/v0.98.1/README.md), since v0.71.0). Commands become skills you run by name ([tools without commands](@/docs/spec-format/commands.md#tools-without-commands)).

- When another target (such as Goose) shares the skills folder, sync drops both fields with a note and skips commands, so files stay identical.
- Set `outputs.crush.skills-dir: .crush/skills` to keep the fields and commands.
- An `x-crush` value for either field wins.
- Import restores the pair as `disable-model-invocation: true`, so the skill is user-only for every selected tool.
- A native file with `user-invocable` but no `disable-model-invocation` imports as `x-crush.disable-model-invocation: false`. Crush's model can start the skill; other tools keep it user-only.

<details>
<summary>A hand-written crushrc too</summary>

Crush merges these config files in order:

1. `./.crushrc`
2. `./crushrc`
3. `$XDG_CONFIG_HOME/crush/crushrc`

It also merges legacy `crush.json` and `.crush.json`. Project wins over global; `crushrc` wins over JSON in one directory. A directory with both logs a warning on every launch.

</details>

### MCP

Servers merge into the `mcp` key of `crush.json`.

| Transport | Written as |
|---|---|
| stdio | `{type: stdio, command, args, env}` |
| HTTP | `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}` |
| SSE | `{type: sse, url, headers, oauth, ...}` |

- The oauth fields are optional and need Crush v0.87.0.
- `sse` stays `sse`, because Crush uses a different transport for it than for `http`.
- A spec's `remote` type defaults to `http`.

Every transport also accepts these fields from [Crush's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/v0.98.1/schema.json):

| Field | Behavior |
|---|---|
| `disabled` | Written as is. |
| `sessionless` | Marks a server that sends no `Mcp-Session-Id`, so Crush skips the subscription stream (Crush v0.91.2). When unset, Crush detects known cases such as GitHub MCP. |
| `enabled_tools`, `disabled_tools` | Choose which tools the agent gets. |
| `timeout` | Whole seconds in Crush, milliseconds in the spec. See below. |
| `channel_enabled` | Opts into incoming MCP messages (Crush v0.97.0). An explicit `false` stays false. |
| `channel_reply` | Sets automatic replies. See below. |

`timeout`:

- Sync divides by 1000 and rounds up, with a note when rounding changes the value.
- A spec value of 0 or less writes no key, so Crush keeps its default.
- `x-crush.timeout` is already seconds, is written as given, and wins.
- A value that is not a whole number writes no key, with a note.

`channel_reply`:

- `user` routes direct messages, `group` routes group messages. Each route needs `tool` and `target_param`.
- `target_meta` picks the incoming attribute holding the reply target. Default: `sender` (direct), `group` (group).
- `message_param` names the reply text argument (default `message`).
- `suppress_tools` lists tools that prevent a duplicate automatic reply.

Set channel fields at the top level or under `x-crush` (wins). Import keeps them at the top level. Sync checks channel types, required route fields, and unknown nested keys before writing. Other `x-crush` MCP keys, except `timeout`, are ignored because Crush rejects unknown MCP fields. Skills still accept `x-crush` frontmatter.

```yaml
name: chat
command: channel-server
x-crush:
  channel_enabled: true
  channel_reply:
    user:
      tool: send_message_to_user
      target_param: user_id
    group:
      tool: send_message_to_group
      target_param: group_id
      target_meta: room
    message_param: message
    suppress_tools: [send]
```

### Hooks

Hooks merge into `crush.json` under `hooks`, beside `mcp`.

- Crush v0.99.0 supports `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionResult`, `SessionStart`, `SessionEnd`, `TurnStart`, `TurnEnd`, `StopFailure`, `Interrupt`, `PreCompact`, and `PostCompact` ([Crush hooks](https://github.com/charmbracelet/crush/blob/v0.99.0/docs/hooks/README.md)). Unknown events get a sync note.
- Any case or snake_case spelling is accepted and written canonically (`session_start` to `SessionStart`).
- Each hook is one flat array item: `{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`. It is not the Claude-style `{"matcher": ..., "hooks": [...]}` group.
- `command` is required. `timeout` is in seconds, default 30.
- Crush runs a command as a script only when it starts with `./`, `../`, or `/`.
  - A command starting with a `.agnostic-ai/scripts/<name>` script (which sync copies) becomes `./.crush/hooks/<name>`.
  - Other commands, such as `bin/guard` or `.crush/hooks/guard`, stay as written.
  - Import keeps every command as written.
- Matchers work only on `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionResult`. Sync rejects one elsewhere, because Crush treats it as a config error.
- `SessionStart` and permission events are observe-only. `SessionEnd` shares a two second budget across its hooks. Same event name does not mean same control behavior as another tool.
- Tool names are lowercase: `bash`, `edit`, `write`, `mcp_<server>_<tool>`, for example `^bash$`. A Claude-style matcher (`Bash`, `Edit`) matches nothing, so `sync` prints a note that the field has no effect.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests `PreToolUse` before a session, using Crush's embedded shell, event data, and timeout. It does not support other Crush events.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.crush.skills-dir` | `.agents/skills` | |
| `outputs.crush.agents` | unset (agents are dropped) | `skill` writes each agent as an on-demand skill. Sync keeps it off with a note while another tool with subagents writes the same skills folder; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills) |
| `outputs.crush.mcp-file` | `crush.json` | also the hooks file |
| `outputs.crush.ignore-file` | `.crushignore` | |

## Import

`agnostic-ai import crush` reads:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` embedded `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` (+ bundled files) | `<skills>/<name>/SKILL.md` (folder copied unchanged) |
| `crush.json` `mcp.<name>` (`stdio`, `http`, or `sse`) | `<mcps>/<name>.yaml`; `timeout` is multiplied by 1000, and 0 is left out |
| `crush.json` `hooks.<event>` | hook specs for each supported event. A unique safe name becomes the filename. Handlers with distinct labels or timeouts get separate spec names and `x-crush.name`, so native identities survive a round trip |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Import leaves commands from personal hook specs in the personal layer, for every supported lifecycle event.
