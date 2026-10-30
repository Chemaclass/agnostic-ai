+++
title = "Crush"
description = "What agnostic-ai writes for Crush: file paths, what it supports, and output settings."
weight = 170

[extra]
group = "Reference"
target_id = "crush"

[extra.moved]
set-up-this-tool = "@/docs/targets/crush.md#first-rule"
+++

# Crush (`crush`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

Charm [Crush](https://github.com/charmbracelet/crush) reads `AGENTS.md`, `.agents/skills/`, `crush.json`, and `.crushignore`.

Set `outputs.crush.agents: skill` to write agents as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

## First rule

Select `crush` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `AGENTS.md`.

Crush has no native custom-agent profiles.

For existing files, check [what `import crush` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `AGENTS.md` for the commit-prefix rule. Launch `crush` from the project and ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree)
crush.json                         # when MCP or hook entries exist (merged with existing user config)
.crushignore                       # project-root ignore patterns
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>


- **Rules**: Crush has no per-rule directory, so rule bodies are included in `AGENTS.md`.
- **Skills**: `.agents/skills/` is the first project path Crush scans. While another target writes that folder, the files are identical to those the other `.agents/skills/` tools get, so it is written once. `x-crush.user-invocable: true` also adds a skill to the command palette (ctrl+p).
  - `disable-model-invocation: true` is written with `user-invocable: true`. Crush then leaves the skill out of the model's list and keeps it in the command palette ([Crush README](https://github.com/charmbracelet/crush/blob/v0.98.1/README.md), since v0.71.0). So each command reaches Crush as a skill the user runs by name ([tools without commands](@/docs/spec-format/commands.md#tools-without-commands)). When another target, including Goose, also writes the skills folder, the files must match, so both fields are dropped with a note and commands are skipped; set `outputs.crush.skills-dir: .crush/skills` to keep them. An `x-crush` value for either field wins. Import reads the pair back as `disable-model-invocation: true`, so after import every selected tool treats that skill as user-only. A native file that keeps `user-invocable` but drops `disable-model-invocation` imports as `x-crush.disable-model-invocation: false`, so Crush's model can start the skill again while other tools keep it user-only.
- **Agents**: Crush has no native agent profiles. By default, agents are skipped with a warning; `outputs.crush.agents: skill` converts them to skills.
- **`crush.json`**: sync keeps your `models`, `providers`, `lsp`, and `options` keys. Crush now prefers `crushrc`, a Bash script it runs on startup, and adds new options only there. JSON is deprecated but still works, so sync writes `crush.json` and never `crushrc`. An MCP field that exists only in `crushrc` cannot be set here.
- **Ignore**: `.crushignore` uses gitignore syntax, supported since [Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md). The usual protection for hand-written files applies.

<details>
<summary>A hand-written crushrc too</summary>

Crush merges every config file: `./.crushrc`, then `./crushrc`, then `$XDG_CONFIG_HOME/crush/crushrc`. It also merges the legacy `crush.json` and `.crush.json` (project over global, `crushrc` over JSON in the same directory). A directory with both logs a warning on every launch.

</details>

### MCP

Servers merge into the `mcp` key of `crush.json`:

- stdio: `{type: stdio, command, args, env}`
- HTTP: `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`
- SSE: `{type: sse, url, headers, oauth, ...}`

The oauth fields are optional and need Crush v0.87.0. `sse` stays `sse`, because Crush uses a different transport for it than for `http`. A spec's `remote` type defaults to `http`.

Every transport also accepts these fields from [Crush's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/v0.98.1/schema.json):

- `disabled` is written as is.
- `sessionless` marks a server that sends no `Mcp-Session-Id`, so Crush skips the subscription stream (Crush v0.91.2). When unset, Crush detects known cases such as GitHub MCP.
- `enabled_tools` and `disabled_tools` choose which tools the agent gets.
- `channel_enabled` opts into incoming MCP messages (Crush v0.97.0). An explicit `false` stays false.
- `channel_reply` sets automatic replies: `user` routes direct messages, `group` routes group messages. Each route requires `tool` and `target_param`; optional `target_meta` chooses the incoming message attribute with the reply target. It defaults to `sender` for direct messages and `group` for group messages. `message_param` names the reply text argument (default `message`), and `suppress_tools` lists tools that prevent a duplicate automatic reply.

Set the channel fields at the top level or under `x-crush`, which wins. Import keeps them at the top level. Sync checks channel types, required route fields and unknown nested keys before writing `crush.json`. Other `x-crush` MCP keys are ignored because Crush rejects unknown MCP fields. Skills still accept `x-crush` frontmatter.

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
- Sync accepts any case or snake_case spelling and writes the canonical event name, such as `session_start` to `SessionStart`.
- Each hook is one flat array item (`{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`), not the Claude-style `{"matcher": ..., "hooks": [...]}` group. `command` is required. `timeout` is in seconds, default 30.
- Crush runs a command as a script only when it starts with `./`, `../`, or `/`. A hook command that starts with a `.agnostic-ai/scripts/<name>` script, which sync copies, becomes `./.crush/hooks/<name>`. Other commands, such as `bin/guard` or `.crush/hooks/guard`, stay as written. Import keeps every command as written.
- Matchers are accepted only for `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, and `PermissionResult`. Sync rejects a matcher on another event because Crush treats it as a configuration error.
- `SessionStart` and permission events are observe-only. `SessionEnd` shares a two second budget across its hooks. Native event names do not imply the same control behavior as another tool.
- Tool names are lowercase (`bash`, `edit`, `write`, `mcp_<server>_<tool>`, for example `^bash$`). A Claude-style matcher (`Bash`, `Edit`) matches nothing, so `sync` prints a note that the field has no effect.
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests `PreToolUse` before a session, using Crush's embedded shell, event data, and timeout. Other Crush events are not supported by that diagnostic.

</details>

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
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http` / `type: sse`) | `<mcps>/<name>.yaml` |
| `crush.json` `hooks.<event>` | hook specs for each supported event; a unique safe name becomes the filename. Repeated native labels use distinct spec names and `x-crush.name`, so the labels and events survive a round trip |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Crush has no documented directory scope. Sync skips scoped rules, and import cannot recover scope from the merged instructions. See [scoped context](@/docs/scoped-context.md).

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
