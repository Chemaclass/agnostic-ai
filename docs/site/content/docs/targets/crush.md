+++
title = "Crush"
description = "How agnostic-ai emits Crush configuration: native paths, capability limits, and output options."
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
- **Rules**: Crush has no per-rule directory, so rule bodies inline into `AGENTS.md`.
- **Skills**: `.agents/skills/` is the first project path Crush scans. It matches the other `.agents/skills/` targets byte for byte, so the shared tree dedupes. `x-crush.user-invocable: true` also adds a skill to the command palette (ctrl+p).
- **Agents**: no Crush surface; they skip with a warning.
- **`crush.json`**: sync keeps your `models`, `providers`, `lsp`, and `options` keys. Crush now prefers `crushrc`, a Bash script it sources on startup, and adds new options only there. JSON is deprecated but stays supported, so sync writes `crush.json` (no `crushrc` emitter). An MCP field that ships only in `crushrc` has no path here.
- **Ignore**: `.crushignore` uses gitignore syntax, supported since [Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md). The shared hand-authored-file protection applies.

{% <details summary="A hand-written crushrc too"> %}
Crush merges every config file: `./.crushrc`, then `./crushrc`, then `$XDG_CONFIG_HOME/crush/crushrc`. It merges legacy `crush.json` / `.crush.json` in too (project over global, `crushrc` over JSON in the same directory). A directory with both logs a warning on every launch.
{% </details> %}

### MCP

Servers merge into the `mcp` key of `crush.json`:

- stdio: `{type: stdio, command, args, env}`
- HTTP: `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`
- SSE: `{type: sse, url, headers, oauth, ...}`

The oauth fields are optional and need Crush v0.87.0. `sse` stays `sse`, because Crush routes it to a different transport than `http`. A spec's `remote` type defaults to `http`.

Every transport also takes these fields from [Crush's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json):

- `disabled` passes through.
- `sessionless` marks a server that sends no `Mcp-Session-Id`, so Crush skips the subscription stream (Crush v0.91.2). When unset, Crush detects known cases such as GitHub MCP.
- `enabled_tools` and `disabled_tools` choose which tools reach the agent.

Crush rejects unknown MCP keys, so MCP has no `x-crush` passthrough. Skills still take `x-crush` keys.

### Hooks

Hooks merge into `crush.json` under `hooks`, beside `mcp`.

- Crush supports only `PreToolUse` ([Crush hooks](https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md)). Other events get a coverage note.
- Sync reads any case or snake_case spelling (`PreToolUse`, `pretooluse`, `pre_tool_use`, `PRE_TOOL_USE`, ...) and writes `PreToolUse`.
- Each hook is one flat array item (`{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`), not the Claude-style `{"matcher": ..., "hooks": [...]}` group. `command` is required. `timeout` is in seconds, default 30.
- Crush runs a command as a script only when it starts with `./`, `../`, or `/`. A hook command that starts with a `.agnostic-ai/scripts/<name>` script, which sync copies, becomes `./.crush/hooks/<name>`. Other commands, such as `bin/guard` or `.crush/hooks/guard`, stay as written, and import keeps every command as written.
- Tool names are lowercase (`bash`, `edit`, `write`, `mcp_<server>_<tool>`, for example `^bash$`). A Claude-style matcher (`Bash`, `Edit`) matches nothing, so `sync` prints a field no-op note.
- [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them in Crush's embedded shell, with its payload and timeout, before a session does.

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
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http` / `type: sse`) | `<mcps>/<name>.yaml` |
| `crush.json` `hooks.PreToolUse` | hook specs; a named entry's `name` becomes the spec's `name:` field and its filename, so a re-import lands at the same path |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Crush has no verified directory scope. Sync skips scoped rules, and import cannot recover scope from flattened instructions. See [scoped context](@/docs/scoped-context.md).

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install: `brew install charmbracelet/tap/crush` (or see the [README](https://github.com/charmbracelet/crush)).
2. Check the tree: `ls AGENTS.md .agents/skills/`, `python -m json.tool crush.json > /dev/null`.
3. Launch `crush`. It loads `AGENTS.md`, lists each `.agents/skills/<name>/`, and connects each `mcp.<name>`. A `PreToolUse` hook's matcher fires (or stays silent) as expected on a real tool call.
