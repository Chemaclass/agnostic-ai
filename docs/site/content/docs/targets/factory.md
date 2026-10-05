+++
title = "Factory"
description = "How agnostic-ai emits Factory configuration: native paths, capability limits, and output options."
weight = 210

[extra]
group = "Reference"
target_id = "factory"
+++

# Factory (`factory`)

Factory [Droid](https://docs.factory.com/harness/subagents) reads the root `AGENTS.md` and custom droids in `.factory/droids/`. Factory has no per-rule directory, so rule bodies go inline into the `AGENTS.md` `## Rules` block.

## Output

```
AGENTS.md                          # pointer body + inlined rules (shared path)
.factory/droids/<name>.md          # one custom-droid profile per agent
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree)
.factory/commands/<name>.md        # one Markdown slash command per command spec
.factory/hooks.json                # when hook entries exist
.factory/mcp.json                  # when MCP entries exist
.factory/settings.json             # when a settings entry carries a model, a shell-command rule, or an x-factory key
```

- **Agents**: each `<name>.md` carries `name`, `description`, and optional `model` and `tools`. Arbitrary `x-factory` keys pass through.
  - An empty body is skipped with a coverage note, since Droid CLI requires a system prompt.
  - A portable `mcpServers` list emits as-is. `mcpServers: []` excludes every MCP server, even global ones, so list the ones you want. See [`mcpServers`](@/docs/spec-format/agents.md#mcpservers-support-by-target).
  - A portable `effort` emits as `reasoningEffort`, which accepts only `low`, `medium`, and `high`. `xhigh`, `max`, and integer budgets drop with a coverage note. Factory ignores the field under `model: inherit`. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort).
  - `readonly: true` emits `tools: read-only`, Factory's read-only tool category. It replaces a portable `tools` list rather than narrowing it, with a coverage note naming `x-factory.tools` for a custom list. `x-factory.tools` wins over `readonly`. `readonly: false` does nothing.
  - A droid gets its MCP servers' tools, so a read-only droid without an `mcpServers` list writes `mcpServers: []`. A listed set is kept.
- **Tools**: the generic `tools` list maps onto Droid CLI's IDs (`Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`). One unknown ID makes Factory reject the droid.
  - `Bash` becomes `Execute`, `Write` becomes `Create`, and `WebFetch` becomes `FetchUrl`, as in Factory's Claude Code importer. The rest already match.
  - `TodoWrite` and `Skill` drop silently, since every droid gets them. `ExitSpecMode` and `GenerateDroid` drop because custom droids cannot enable them. `tools: all` is never written, since an omitted key allows every tool.
  - Any other name drops with a coverage note; translated names still emit.
  - `x-factory.tools` writes Factory's vocabulary directly and wins over the translation. Only it reaches a category (`read-only`, `edit`, `execute`, `web`, `mcp`) or an MCP tool ID. See [`tools` support by target](@/docs/spec-format/agents.md#tools-support-by-target).
- **Skills**: written once to the tree that codex, amp, zed, and crush share. Factory also reads [`.agent/skills/**/SKILL.md`](https://docs.factory.com/harness/skills), which this adapter does not write.
  - Scoped skills emit at `<scope>/.factory/skills/<name>/SKILL.md` with bundled assets. Unscoped skills keep `.agents/skills/`.
  - `outputs.factory.skills-dir` replaces the directory at the root and in each scope. Unmanaged files keep their contents.
- **Commands**: `.factory/commands/<name>.md` takes `description` and `argument-hint`. `$ARGUMENTS` is preserved. Factory prefers Skills for new workflows but still loads commands.
- **Hooks**: `.factory/hooks.json` is the committed project file ([hooks docs](https://docs.factory.com/harness/hooks)). `sync` overwrites it whole, so change the hook spec, not the file.
  - Nine events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`, `SessionStart`, `SessionEnd`.
  - The file is keyed by event name, `{"<Event>": [{matcher, hooks: [...]}]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
  - Per entry: `type` (always `"command"`), `command`, and optional `timeout` in seconds (default 60). Factory's `commandRegex` has no spec counterpart and is not emitted.
  - `matcher` is a regex over Factory's tool names, which match Claude's except `Bash`, `Write`, and `WebFetch`. A matcher using those emits verbatim, matching nothing, with a coverage note.
- **MCP**: `.factory/mcp.json` under `mcpServers`, the Claude Code and Cursor shape (stdio: `command`/`args`/`env`, no `type`; remote: `type` + `url`/`headers`).
  - `sync` overwrites the file whole. Factory's [MCP docs](https://docs.factory.com/harness/mcp) say to remove project servers by editing this file, but sync loses that edit, so remove the MCP spec.
  - `disabled: true` passes through as Factory's per-server boolean. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
  - Both transports keep `disabledTools`, `timeout`, and `connectTimeout` (milliseconds), explicit zeros included. Remote HTTP/SSE servers also accept `oauth: false` or an OAuth object with `scopes`, `resource`, `authorizationServerIssuer`, `clientId`, `clientSecret`, `clientMetadataUrl`, `tokenEndpointAuthMethod`, and `callbackPort`.
  - `x-factory` overrides each top-level option. A `type: ws` spec emits no server, with a coverage note, since Factory supports only stdio, HTTP, and SSE.
- **Settings**: a portable `model` merges into `<git-root>/.factory/settings.json`, the project tier of Factory's [hierarchical settings](https://docs.factory.com/enterprise/hierarchical-settings-and-org-control).
  - A portable `effort` merges as top-level `reasoningEffort` ([CLI settings](https://docs.factory.com/droid-cli/settings)): `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max`, each model accepting a subset. Another value raises a coverage note. The managed `sessionDefaultSettings.reasoningEffort` is not written.
  - Sync merges, setting only `model`, `reasoningEffort`, the command lists, and `x-factory` keys, so `disabledSkills` and other keys survive.
  - `x-factory` reaches unmodeled keys, mainly `sandbox` (kernel-enforced isolation, no portable equivalent). Its command-list entries add to the translated ones: `x-factory.commandBlocklist: ["author-only"]` beside `deny: ["Bash(rm:*)"]` writes both.
  - Portable `allow`, `deny`, and `ask` write Factory's shell-command lists. `Bash(npm:*)` becomes `"npm *"` and `Bash(curl)` becomes `"curl"`.

| portable | Factory key | why |
|---|---|---|
| `allow` | `commandAllowlist` | Always runs without approval. |
| `ask` | `commandDenylist` | Always asks for confirmation; the user can approve it. |
| `deny` | `commandBlocklist` | Never runs; no approval path. |

  - Factory's denylist prompts, so portable `deny` maps to the blocklist, not the denylist. Precedence matches agnostic-ai's: denylist beats allowlist, and blocklist beats both.
  - Rules scoping a path, URL, or MCP tool (such as `Read(src/**)`) have no shell form and raise a coverage note.
  - A list is written only when a rule translates into it, so a hand-kept list survives a sync with nothing for it. A sync with rules replaces that key.
  - Factory marks the three lists deprecated in favor of `permissionRules` but still reads them ([LLM safety and agent controls](https://docs.factory.com/enterprise/llm-safety-and-agent-controls)). A permission rule needs a stable `id`, a `match.prefix` token list, and `tests.match` and `tests.noMatch` examples, which portable lists lack. Put them under `x-factory.permissionRules`, which reaches `.factory/settings.json` unchanged.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.factory.agents-dir` | `.factory/droids` |
| `outputs.factory.skills-dir` | `.agents/skills` |
| `outputs.factory.commands-dir` | `.factory/commands` |
| `outputs.factory.hooks-file` | `.factory/hooks.json` |
| `outputs.factory.mcp-file` | `.factory/mcp.json` |
| `outputs.factory.conf-file` | `.factory/settings.json` |

## Import

`agnostic-ai import factory` reads the Factory layout back:

| Source | Becomes |
|--------|---------|
| `AGENTS.md` inlined `## Rules` block (`### <name>` children) | `<rules>/<name>.md` per rule |
| `.factory/droids/<name>.md` | `<agents>/<name>.md` |
| `.agents/skills/<name>/SKILL.md` and `.factory/skills/<name>/SKILL.md` | `<skills>/<name>/SKILL.md` |
| `<scope>/.factory/skills/<name>/SKILL.md` | `<skills>/<scope>/<name>/SKILL.md` |
| `.factory/commands/<name>.md` | `<commands>/<name>.md` |
| `.factory/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml` |
| `.factory/hooks.json`, else the legacy `.factory/hooks/hooks.json` | one hook spec per matcher group |
| `.factory/settings.json` `model`, `reasoningEffort`, and command lists | `<settings>/factory.yaml`; `reasoningEffort` becomes `effort`, or `x-factory.reasoningEffort` when another settings spec sets a different effort |
| `AGENTS.md` | `.agnostic-ai/AGNOSTIC_AI.md` |

- **Droids**: `tools` renames back (`Execute` to `Bash`, `Create` to `Write`, `FetchUrl` to `WebFetch`). A list with a category (`read-only`) or an MCP tool ID lands under `x-factory.tools` untouched. `reasoningEffort` becomes `effort`. Other droid keys land under `x-factory`.
- **Command lists**: these read back as `Bash(...)` rules: `commandAllowlist` to `allow`, `commandDenylist` to `ask`, `commandBlocklist` to `deny`. A pattern ending in ` *` becomes `Bash(x:*)`. Other `settings.json` keys stay in the file, which sync merges into.

{% <details summary="Read-only droids on import"> %}
A `tools: read-only` from a portable `readonly: true` also lands under `x-factory.tools`, since import does not guess `readonly` back. Re-syncing writes the same droid file through that override.
{% </details> %}

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the Factory CLI ([subagents docs](https://docs.factory.com/harness/subagents)).
2. Check the tree:
   - `ls AGENTS.md .factory/droids/ .agents/skills/ .factory/hooks.json .factory/mcp.json`
   - `grep "Generated by agnostic-ai" .factory/droids/*.md` (the header sits after the frontmatter)
   - `python -m json.tool .factory/hooks.json > /dev/null` when hook specs exist
   - `python -m json.tool .factory/mcp.json > /dev/null`
3. Launch `droid`:
   - Each `.factory/droids/<name>.md` appears in the droid picker.
   - Each `.agents/skills/<name>/` loads as a skill.
   - Each `mcpServers.<name>` from `.factory/mcp.json` connects.
   - `/hooks` shows each entry in `.factory/hooks.json` under the Project tab.
