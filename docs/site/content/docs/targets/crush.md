+++
title = "Crush"
description = "How agnostic-ai emits Crush configuration: native paths, capability limits, and output options."
weight = 170

[extra]
group = "Reference"
target_id = "crush"
+++

# Crush (`crush`)

```
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed)
crush.json                         # when MCP or PreToolUse hook entries exist (merged with existing user config)
.crushignore                       # project-root ignore patterns
```

Charm [Crush](https://github.com/charmbracelet/crush) reads the root `AGENTS.md` natively and has no per-rule directory, so rule bodies inline into the shared entry-point. Skills emit into `.agents/skills/`, the first project path Crush scans; the render is byte-identical with codex/amp/zed so the shared tree dedupes. Set `x-crush.user-invocable: true` on a skill to also add it to Crush's command palette (ctrl+p).

MCP servers merge into the `mcp` key of `crush.json` (`{type: stdio, command, args, env}`, `{type: http, url, headers, oauth, oauth_client_id, oauth_client_secret, oauth_callback_port}`, or `{type: sse, url, headers, oauth, ...}`; oauth fields optional, shipped in Crush v0.87.0).

`sse` keeps its own type rather than collapsing into `http`: Crush's `MCPType` enum treats them as distinct values routed to different transports, and an SSE-only server does not speak the Streamable HTTP that a mislabelled `http` entry would connect with. A spec's `remote` type has no matching Crush value and defaults to `http`.

Either transport also carries `disabled`, `sessionless`, `enabled_tools`, and `disabled_tools`, all four read from [the vendor's published `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json) rather than the README, which is the only place the MCP property set appears closed. `disabled` ("Whether this MCP server is disabled", at `$defs.MCPConfig.properties.disabled`) passes through unchanged. It was dropped silently until #641, and that drop was worse than a warning: one spec synced to crush and trae printed a note for trae and nothing for crush, so the silence read as success.

`sessionless` marks a server that sends no `Mcp-Session-Id` "so Crush skips the subscriptions/listen stream it would otherwise reject" (shipped in v0.91.2; leave it unset to let Crush auto-detect known cases such as GitHub MCP). The two tool lists gate which of the server's tools reach the agent.

Every field here is mapped explicitly rather than merged from a generic `x-crush` block: `MCPConfig` sets `"additionalProperties": false`, so a typo in a namespaced passthrough would produce a config Crush rejects outright rather than one it ignores. Skill frontmatter has no such constraint, which is why the shared skill renderer still takes the generic merge.

User-managed keys (`models`, `providers`, `lsp`, `options`) survive every sync. Agents have no Crush surface and skip with a warning.

- **Hooks**: merge into that same `crush.json`, under a `hooks` key alongside `mcp`, in one `MergeJSONFile` write (not two, so neither key's snapshot goes stale during sync's collision-detection pass).
  - [Crush's own hooks doc](https://github.com/charmbracelet/crush/blob/main/docs/hooks/README.md) states "Crush currently supports just one hook, PreToolUse, with plans to support the full gamut" (re-verified 2026-09-10 against both that doc and [the vendor's `schema.json`](https://raw.githubusercontent.com/charmbracelet/crush/main/schema.json), whose `$defs.HookConfig` carries no per-event variant). A hook spec targeting any other event surfaces a coverage note instead of a dead JSON entry Crush never reads.
  - That one event answers to five spellings: "Event names are case insensitive and snake-caseable, so `PreToolUse`, `pretooluse`, `PRETOOLUSE`, `pre_tool_use`, and `PRE_TOOL_USE` all work" (verified 2026-09-11, #731). This adapter applies that rule when it decides whether a spec is a `PreToolUse` hook, and always writes the canonical `PreToolUse` key, so the output shape never depends on how the spec spelled it.
  - A `PreToolUse` entry renders flat, one array item per hook (`{"name": ..., "matcher": ..., "command": ..., "timeout": ...}`), unlike the Claude-style `{"matcher": ..., "hooks": [...]}` grouping Claude Code, Codex, OpenHands, and Qoder use. `command` is the only required field; `timeout` is seconds, defaulting to 30 when unset.
  - Crush's own tool names are lowercase (`bash`, `edit`, `write`, `mcp_<server>_<tool>`; its worked examples use `^bash$`), so a Claude-style matcher (`Bash`, `Edit`, ...) parses as a valid regex and then matches nothing, the same trap OpenHands and Windsurf hit with their own tool vocabularies. That case surfaces a field no-op note.

`crush.json` is Crush's legacy format: the vendor's own docs call it deprecated and say "new configuration options will only be added to Bash-based config", the documented primary format now, `crushrc` (a Bash script Crush sources on startup). JSON still loads today (the vendor: "we plan to support it for the forseeable future") and this adapter still targets it. Writing a `crushrc` emitter is a separate feature with its own design questions (shell-quoting header values, merge interaction) and is not done here.

Two things to know: any future crush-only MCP field ships Bash-only and has no path through this adapter. Crush's discovery order also merges files rather than picking one, lower numbers taking precedence: `./.crushrc`, then `./crushrc`, then `$XDG_CONFIG_HOME/crush/crushrc`, with legacy `crush.json` / `.crush.json` merged in alongside those paths (project over global, `crushrc` over JSON in the same directory), and a startup warning logged whenever a directory holds both.

A project that also hand-authors a `crushrc` gets that warning against our `crush.json` on every launch.

Ignore specs write project-root `.crushignore` with gitignore syntax, supported by [Crush v0.94.1](https://raw.githubusercontent.com/charmbracelet/crush/v0.94.1/README.md). The shared hand-authored-file protection applies.

Config keys:

| Key | Default | Notes |
| --- | --- | --- |
| `outputs.crush.skills-dir` | `.agents/skills` | |
| `outputs.crush.mcp-file` | `crush.json` | also the hooks file |
| `outputs.crush.ignore-file` | `.crushignore` | |

## Import

`agnostic-ai import crush` reverses the Crush layout. Crush has no per-rule directory, so rules ride inside the shared `AGENTS.md`:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.agents/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `crush.json` (`mcp.<name>`, `type: stdio` / `type: http` / `type: sse`) | `<mcps>/<name>.yaml` |
| `crush.json` `hooks.PreToolUse` | hook specs; a named entry's `name` becomes both the spec's `name:` field and its filename, so a re-import lands at the same path |
| `.crushignore` | an ignore spec |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

Crush imports root rules from its inlined block. It has no verified native directory scope; scoped source rules are skipped on sync. Import cannot recover scope from previously flattened instructions. See [scoped context](@/docs/scoped-context.md).

Verify with the real CLI:

1. Install: `brew install charmbracelet/tap/crush` (or see the [README](https://github.com/charmbracelet/crush)).
2. Check the tree: `ls AGENTS.md .agents/skills/`, `python -m json.tool crush.json > /dev/null`.
3. Launch `crush`; the context loads `AGENTS.md`, the skills list shows each `.agents/skills/<name>/`, each `mcp.<name>` connects, and a `PreToolUse` hook's matcher fires (or stays silent) as expected against a real tool call.
