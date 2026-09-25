+++
title = "Crush"
description = "How agnostic-ai emits Crush configuration: native paths, capability limits, and output options."
weight = 170

[extra]
group = "Reference"
target_id = "crush"
+++

# Crush (`crush`)

## Output

```
AGENTS.md                          # entry-point pointer body + inlined rules (shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed)
crush.json                         # when MCP or PreToolUse hook entries exist (merged with existing user config)
.crushignore                       # project-root ignore patterns
```

Charm [Crush](https://github.com/charmbracelet/crush) reads the root `AGENTS.md` and has no per-rule directory, so rule bodies inline there. Skills emit into `.agents/skills/`, the first project path Crush scans; the render matches codex/amp/zed byte for byte, so the shared tree dedupes. Set `x-crush.user-invocable: true` on a skill to also add it to Crush's command palette (ctrl+p).

MCP servers merge into the `mcp` key of `crush.json` (`{type: stdio, command, args, env}`, `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`, or `{type: sse, url, headers, oauth, ...}`). The oauth fields are optional and need Crush v0.87.0.

`sse` stays `sse`, since Crush routes it to a different transport than `http`. A spec's `remote` type has no Crush equivalent and defaults to `http`.

Every transport also carries these fields from [Crush's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json):

- `disabled` passes through unchanged.
- `sessionless` marks a server that sends no `Mcp-Session-Id`, so Crush skips the subscription stream (Crush v0.91.2). Leave it unset to let Crush auto-detect known cases such as GitHub MCP.
- `enabled_tools` and `disabled_tools` gate which of the server's tools reach the agent.

Each field maps explicitly, with no generic `x-crush` passthrough, because Crush rejects any unknown MCP key. Skill frontmatter has no such limit, so skills still take `x-crush` keys.

User-managed keys (`models`, `providers`, `lsp`, `options`) survive every sync. Agents have no Crush surface and skip with a warning.

- **Hooks**: merge into the same `crush.json` under `hooks`, alongside `mcp`, in one write.
  - Crush supports only `PreToolUse` ([Crush hooks](https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md)). A hook for any other event gets a coverage note instead of a dead entry.
  - Crush accepts any case or snake_case spelling (`PreToolUse`, `pretooluse`, `pre_tool_use`, `PRE_TOOL_USE`, ...). The adapter recognizes all of them and always writes `PreToolUse`.
  - Each entry renders flat, one array item per hook (`{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`), not the Claude-style `{"matcher": ..., "hooks": [...]}` grouping. `command` is required; `timeout` is in seconds, default 30.
  - Crush's tool names are lowercase (`bash`, `edit`, `write`, `mcp_<server>_<tool>`, e.g. `^bash$`). A Claude-style matcher (`Bash`, `Edit`) is valid regex but matches nothing, so `sync` prints a field no-op note.

`crush.json` is Crush's deprecated format. Crush now prefers `crushrc`, a Bash script it sources on startup, and adds new options only there. JSON still loads, and Crush plans to keep supporting it, so this adapter still writes `crush.json`. There is no `crushrc` emitter.

Two consequences:

- A future Crush-only MCP field that ships only in `crushrc` has no path through this adapter.
- Crush merges config files instead of picking one: `./.crushrc`, then `./crushrc`, then `$XDG_CONFIG_HOME/crush/crushrc`, with legacy `crush.json` / `.crush.json` merged in (project over global, `crushrc` over JSON in the same directory). A directory with both logs a startup warning, so a project that also has a hand-written `crushrc` sees it on every launch.

Ignore specs write project-root `.crushignore` in gitignore syntax, supported since [Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md). The shared hand-authored-file protection applies.

## Config keys

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.crush.skills-dir` | `.agents/skills` | |
| `outputs.crush.mcp-file` | `crush.json` | also the hooks file |
| `outputs.crush.ignore-file` | `.crushignore` | |

## Import

`agnostic-ai import crush` reverses the Crush layout. Rules come from the inlined block in `AGENTS.md`:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http` / `type: sse`) | `<mcps>/<name>.yaml` |
| `crush.json` `hooks.PreToolUse` | hook specs; a named entry's `name` becomes both the spec's `name:` field and its filename, so a re-import lands at the same path |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Crush has no verified directory scope, so scoped source rules are skipped on sync, and import cannot recover scope from flattened instructions. See [scoped context](@/docs/scoped-context.md).

## Verify

1. Install: `brew install charmbracelet/tap/crush` (or see the [README](https://github.com/charmbracelet/crush)).
2. Check the tree: `ls AGENTS.md .agents/skills/`, `python -m json.tool crush.json > /dev/null`.
3. Launch `crush`. The context loads `AGENTS.md`, the skills list shows each `.agents/skills/<name>/`, each `mcp.<name>` connects, and a `PreToolUse` hook's matcher fires (or stays silent) as expected on a real tool call.
