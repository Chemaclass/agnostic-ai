+++
title = "Crush"
description = "What agnostic-ai writes for Crush: file paths, what it supports, and output settings."
weight = 170

[extra]
group = "Reference"
target_id = "crush"
+++

# Crush (`crush`)

Charm [Crush](https://github.com/charmbracelet/crush) reads `AGENTS.md`, `.agents/skills/`, `crush.json`, and `.crushignore`.

Set `outputs.crush.agents: skill` to write agents as on-demand skills; see [agents as skills](@/docs/spec-format/agents.md#agents-as-skills).

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree)
crush.json                         # when MCP or PreToolUse hook entries exist (merged with existing user config)
.crushignore                       # project-root ignore patterns
```
- **Rules**: Crush has no per-rule directory, so rule bodies go inline into `AGENTS.md`.
- **Skills**: `.agents/skills/` is the first project path Crush scans. The files are identical to those the other `.agents/skills/` tools get, so the shared folder is written once. `x-crush.user-invocable: true` also adds a skill to the command palette (ctrl+p).
- **Agents**: Crush has no agent support. Agents are skipped with a warning.
- **`crush.json`**: sync keeps your `models`, `providers`, `lsp`, and `options` keys. Crush now prefers `crushrc`, a Bash script it runs on startup, and adds new options only there. JSON is deprecated but still works, so sync writes `crush.json` and never `crushrc`. An MCP field that exists only in `crushrc` cannot be set here.
- **Ignore**: `.crushignore` uses gitignore syntax, supported since [Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md). The usual protection for hand-written files applies.

{% <details summary="A hand-written crushrc too"> %}
Crush merges every config file: `./.crushrc`, then `./crushrc`, then `$XDG_CONFIG_HOME/crush/crushrc`. It also merges the legacy `crush.json` and `.crush.json` (project over global, `crushrc` over JSON in the same directory). A directory with both logs a warning on every launch.
{% </details> %}

### MCP

Servers merge into the `mcp` key of `crush.json`:

- stdio: `{type: stdio, command, args, env}`
- HTTP: `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`
- SSE: `{type: sse, url, headers, oauth, ...}`

The oauth fields are optional and need Crush v0.87.0. `sse` stays `sse`, because Crush uses a different transport for it than for `http`. A spec's `remote` type defaults to `http`.

Every transport also accepts these fields from [Crush's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json):

- `disabled` is written as is.
- `sessionless` marks a server that sends no `Mcp-Session-Id`, so Crush skips the subscription stream (Crush v0.91.2). When unset, Crush detects known cases such as GitHub MCP.
- `enabled_tools` and `disabled_tools` choose which tools the agent gets.

Crush rejects unknown MCP keys, so MCP entries take no `x-crush` keys. Skills still do.

### Hooks

Hooks merge into `crush.json` under `hooks`, beside `mcp`.

- Crush supports only `PreToolUse` ([Crush hooks](https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md)). Other events get a coverage note.
- Sync accepts any case or snake_case spelling (`PreToolUse`, `pretooluse`, `pre_tool_use`, `PRE_TOOL_USE`, ...) and writes `PreToolUse`.
- Each hook is one flat array item (`{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`), not the Claude-style `{"matcher": ..., "hooks": [...]}` group. `command` is required. `timeout` is in seconds, default 30.
- Crush runs a command as a script only when it starts with `./`, `../`, or `/`. A hook command that starts with a `.agnostic-ai/scripts/<name>` script, which sync copies, becomes `./.crush/hooks/<name>`. Other commands, such as `bin/guard` or `.crush/hooks/guard`, stay as written. Import keeps every command as written.
- Tool names are lowercase (`bash`, `edit`, `write`, `mcp_<server>_<tool>`, for example `^bash$`). A Claude-style matcher (`Bash`, `Edit`) matches nothing, so `sync` prints a note that the field has no effect.
- [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them in Crush's embedded shell, with its event data and timeout, so you can test them before a session does.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.crush.skills-dir` | `.agents/skills` | |
| `outputs.crush.mcp-file` | `crush.json` | also the hooks file |
| `outputs.crush.ignore-file` | `.crushignore` | |

## Import

`agnostic-ai import crush` reads:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied unchanged) |
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http` / `type: sse`) | `<mcps>/<name>.yaml` |
| `crush.json` `hooks.PreToolUse` | hook specs; a named entry's `name` becomes the spec's `name:` field and its filename, so a re-import gives the same path |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Crush has no documented directory scope. Sync skips scoped rules, and import cannot recover scope from the merged instructions. See [scoped context](@/docs/scoped-context.md).

## Protected paths

Not enforced. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install: `brew install charmbracelet/tap/crush` (or see the [README](https://github.com/charmbracelet/crush)).
2. Check the files: `ls AGENTS.md .agents/skills/`, `python -m json.tool crush.json > /dev/null`.
3. Launch `crush`. It loads `AGENTS.md`, lists each `.agents/skills/<name>/`, and connects each `mcp.<name>`. A `PreToolUse` hook fires, or stays silent, as its matcher says on a real tool call.
