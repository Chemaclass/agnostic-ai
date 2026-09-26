+++
title = "Factory"
description = "How agnostic-ai emits Factory configuration: native paths, capability limits, and output options."
weight = 210

[extra]
group = "Reference"
target_id = "factory"
+++

# Factory (`factory`)

## Output

```
AGENTS.md                          # pointer body + inlined rules (shared path)
.factory/droids/<name>.md          # one custom-droid profile per agent
.agents/skills/<name>/SKILL.md     # one folder per skill (shared with codex/amp/zed/crush)
.factory/commands/<name>.md        # one Markdown slash command per command spec
.factory/hooks.json                # when hook entries exist
.factory/mcp.json                  # when MCP entries exist
.factory/settings.json             # when a settings entry carries a model, a shell-command rule, or an x-factory key
```

Factory [Droid](https://docs.factory.ai/harness/subagents) reads the root `AGENTS.md` and loads custom droids from `.factory/droids/`. Factory has no per-rule directory, so rule bodies inline into the `AGENTS.md` `## Rules` block.

- **Agents**: one `<name>.md` profile per agent with `name`, `description`, and optional `model` and `tools` frontmatter. Arbitrary `x-factory` keys pass through.
  - An agent with an empty body is skipped with a coverage note, because Droid CLI requires a non-empty system prompt.
  - A portable `mcpServers` list emits as-is. `mcpServers: []` excludes every MCP server, even global ones, so list the servers you want instead of an empty list. See [`mcpServers`](@/docs/spec-format.md#mcpservers-support-by-target).
  - A portable `effort` emits as `reasoningEffort`, which accepts `low`, `medium`, and `high` only. `xhigh`, `max`, and integer budgets drop with a coverage note. Factory ignores the field under `model: inherit`. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort).
  - **Read-only agents**: `readonly: true` emits `tools: read-only`, Factory's own category for `Read`, `LS`, `Grep`, `Glob`. It wins outright over a portable `tools` list on the same agent rather than narrowing it, with a coverage note naming `x-factory.tools` as the escape hatch for a custom list. `x-factory.tools` still wins over `readonly` itself. `readonly: false` is a no-op.
- **Tools**: the generic `tools` list is translated onto Droid CLI's tool IDs (`Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`). Factory rejects the whole droid on one unknown ID.
  - `Bash` becomes `Execute`, `Write` becomes `Create`, and `WebFetch` becomes `FetchUrl`, the same renames Factory's Claude Code importer makes. The rest already match.
  - `TodoWrite` and `Skill` drop silently, since every droid gets them anyway. `ExitSpecMode` and `GenerateDroid` drop because custom droids cannot enable them. `tools: all` is never written; an omitted key already allows every tool.
  - Any other name drops with a coverage note. The names that translate still emit.
  - `x-factory.tools` writes Factory's own vocabulary directly and wins over the translation. It is the only way to reach a category (`read-only`, `edit`, `execute`, `web`, `mcp`) or an MCP tool ID. See [`tools` support by target](@/docs/spec-format.md#tools-support-by-target).
- **Skills**: load from `.agents/skills/`, the tree codex, amp, zed, and crush share. The render is identical, so the tree is written once. Factory also reads [`.agent/skills/**/SKILL.md`](https://docs.factory.ai/harness/skills), which this adapter does not write.
- **Commands**: written to `.factory/commands/<name>.md` with `description` and `argument-hint` frontmatter. `$ARGUMENTS` is preserved. Factory recommends Skills for new workflows but still loads commands.
- **Hooks**: written to `.factory/hooks.json`, the committed project file ([hooks docs](https://docs.factory.ai/harness/hooks)).
  - `sync` overwrites the file whole, with no merge. A hand edit is lost on the next sync; change the hook spec instead.
  - Nine events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`, `SessionStart`, `SessionEnd`.
  - The file is keyed directly by event name, `{"<Event>": [{matcher, hooks: [...]}]}`, with no `hooks` wrapper (unlike Claude Code, Codex, Gemini, and Qoder).
  - Per entry: `type` (always `"command"`), `command`, and optional `timeout` in seconds (default 60).
  - `matcher` is a regex over Factory's tool names, which match Claude's except `Bash`, `Write`, and `WebFetch`. A matcher using one of those still emits verbatim, with one coverage note, because it matches nothing.
  - Factory's `commandRegex` field has no generic spec counterpart and is not emitted.
- **MCP**: written to `.factory/mcp.json` under `mcpServers`, the Claude Code and Cursor shape (stdio: `command`/`args`/`env`, no `type`; remote: `type` + `url`/`headers`).
  - `sync` overwrites the file whole. Factory's [MCP docs](https://docs.factory.ai/harness/mcp) tell you to remove project servers by editing this file; that edit is lost on the next sync, so remove the MCP spec instead.
  - Factory supports a per-server `disabled` boolean, so `disabled: true` passes through. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).
  - Both transports keep `disabledTools`, `timeout`, and `connectTimeout` (milliseconds), including explicit zeros.
  - Remote HTTP/SSE servers also accept `oauth: false` or an OAuth object with `scopes`, `resource`, `authorizationServerIssuer`, `clientId`, `clientSecret`, `clientMetadataUrl`, `tokenEndpointAuthMethod`, and `callbackPort`.
  - `x-factory` overrides each top-level option, scoped to Factory. A `type: ws` spec emits no server and raises a coverage note, since Factory supports only stdio, HTTP, and SSE.
- **Settings**: a portable `model` merges into `<git-root>/.factory/settings.json`, the project tier of Factory's [hierarchical settings](https://docs.factory.ai/enterprise/hierarchical-settings-and-org-control). A portable `effort` merges as top-level `reasoningEffort` ([CLI settings](https://docs.factory.ai/droid-cli/settings)): `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max`, with each model accepting a subset. Another value raises a coverage note. The managed `sessionDefaultSettings.reasoningEffort` key is not written.
  - This file is merged, not overwritten. Only `model`, `reasoningEffort`, the command lists, and `x-factory` keys are set, so `disabledSkills` and other keys survive.
  - An `x-factory` block merges into the same file for keys this tool does not model. `sandbox` is the main case: kernel-enforced isolation with no portable equivalent. Entries in one of the three command lists add to the translated ones, so `x-factory.commandBlocklist: ["author-only"]` beside `deny: ["Bash(rm:*)"]` writes both.
  - The portable `allow`, `deny`, and `ask` lists write Factory's shell-command lists. `Bash(npm:*)` becomes `"npm *"` and `Bash(curl)` becomes `"curl"`.

| portable | Factory key | why |
|---|---|---|
| `allow` | `commandAllowlist` | Always runs without approval. |
| `ask` | `commandDenylist` | Always asks for confirmation; the user can approve it. |
| `deny` | `commandBlocklist` | Never runs; no approval path. |

  - The names mislead: Factory's denylist prompts, so portable `deny` maps to the blocklist, not the denylist.
  - Precedence matches agnostic-ai's: denylist wins over allowlist, and blocklist wins over both.
  - Rules scoping a path, a URL, or an MCP tool (such as `Read(src/**)`) have no shell-pattern form and raise a coverage note.
  - A list is written only when at least one rule translates into it, so a hand-maintained list survives a sync with nothing for it. A sync that has rules replaces that key.

Scoped skills emit at `<scope>/.factory/skills/<name>/SKILL.md` with bundled assets; unscoped skills keep `.agents/skills/`. `outputs.factory.skills-dir` replaces the directory at the root and in each scope. Unmanaged files keep their contents.

## Import

`agnostic-ai import factory` reverses the Factory layout:

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

A droid's `tools` renames back: `Execute` to `Bash`, `Create` to `Write`, `FetchUrl` to `WebFetch`. A list holding a category (`read-only`) or an MCP tool ID has no portable spelling, so it lands under `x-factory.tools` untouched, including a `tools: read-only` written from a portable `readonly: true`: import does not guess a category back into `readonly`, so re-syncing the imported spec still reaches the same droid file through the `x-factory.tools` override. `reasoningEffort` becomes `effort`, and every other droid key lands under `x-factory`.

The command lists read back as `Bash(...)` rules: `commandAllowlist` to `allow`, `commandDenylist` to `ask`, `commandBlocklist` to `deny`. A pattern ending in ` *` becomes the prefix form `Bash(x:*)`. Other `settings.json` keys stay in the file, which sync merges into.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.factory.agents-dir` | `.factory/droids` |
| `outputs.factory.skills-dir` | `.agents/skills` |
| `outputs.factory.commands-dir` | `.factory/commands` |
| `outputs.factory.hooks-file` | `.factory/hooks.json` |
| `outputs.factory.mcp-file` | `.factory/mcp.json` |
| `outputs.factory.conf-file` | `.factory/settings.json` |

## Verify

1. Install the Factory CLI ([subagents docs](https://docs.factory.ai/harness/subagents)).
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
