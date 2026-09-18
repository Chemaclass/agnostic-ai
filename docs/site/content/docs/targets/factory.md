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
AGENTS.md                          # canonical entry-point pointer body + inlined rules (written by sync, shared path)
.factory/droids/<name>.md          # one custom-droid profile per agent
.agents/skills/<name>/SKILL.md     # one folder per skill (shared tree with codex/amp/zed/crush)
.factory/commands/<name>.md        # one Markdown slash command per command spec
.factory/hooks.json                # when hook entries exist
.factory/mcp.json                  # when MCP entries exist
```

Factory [Droid](https://docs.factory.ai/harness/subagents) reads the root `AGENTS.md` natively and loads custom droids from `.factory/droids/`. Each agent emits as one `<name>.md` profile with `name`, `description`, and optional `model` / `tools` frontmatter (`tools` translates onto Droid CLI's own tool IDs, see below); arbitrary `x-factory` keys pass through. A portable `mcpServers` list emits as-is, narrowing which servers the droid may reach: "Setting `mcpServers: []` excludes every MCP server, even globally configured ones", so write the servers you want rather than an empty list. A portable `effort` emits as Factory's own `reasoningEffort`, which documents `low`, `medium`, and `high` only; `xhigh`, `max`, and Qoder's integer budgets are dropped with a coverage note, and the vendor ignores the field entirely under `model: inherit`. See [`mcpServers`](@/docs/spec-format.md#mcpservers-support-by-target) and [`effort` support by target](@/docs/spec-format.md#effort-support-by-target).

An agent spec with an empty body skips instead of writing a frontmatter-only file: Droid CLI's own schema says the body "is the system prompt and cannot be empty", so the skip surfaces as a coverage note rather than landing a file the tool rejects. Factory has no per-rule directory, so rule bodies inline into the shared `AGENTS.md` `## Rules` block.

Skills load from `.agents/skills/`, the same cross-tool tree codex, amp, zed, and crush emit; the render is byte-identical, so the shared tree dedupes into one write. [docs.factory.ai/harness/skills](https://docs.factory.ai/harness/skills) also documents a second compatibility path, `.agent/skills/**/SKILL.md`, which this adapter does not additionally write.

- **Tools**: a spec's generic `tools` list is translated onto Droid CLI's own tool IDs, not passed through. The vendor's table is the complete set of valid IDs (`Read`, `LS`, `Grep`, `Glob`, `Create`, `Edit`, `ApplyPatch`, `Execute`, `WebSearch`, `FetchUrl`) and "Unknown IDs cause a validation error", so one unknown name costs the author the whole droid, not just that tool.
  - `Bash` becomes `Execute`, `Write` becomes `Create`, and `WebFetch` becomes `FetchUrl`, the same three renames Factory's own Claude Code importer performs; the rest of agnostic-ai's vocabulary is already valid and carries over.
  - Three load-time rules shape the rest: `TodoWrite` and `Skill` are "always included for every droid ... You do not list them", so they drop without a note since the droid keeps them anyway; `ExitSpecMode` and `GenerateDroid` "cannot be enabled by a custom droid", so they drop like any unknown name; and the literal `tools: all` is rejected by Droid CLI, so a scalar value never reaches the frontmatter and the omitted key means "allow every tool", which is Factory's own way to spell it.
  - Any other name drops with a coverage note rather than being written unconfirmed, while the names that do translate still emit.
  - Set `x-factory.tools` to bypass the table with Factory's own vocabulary directly, the only way to reach a category name (`read-only`, `edit`, `execute`, `web`, `mcp`) or a registered MCP tool ID; it wins outright over the translated form. See [`tools` support by target](@/docs/spec-format.md#tools-support-by-target).
- **Commands**: written to `.factory/commands/<name>.md` with `description` and `argument-hint` frontmatter. The body stays Markdown and `$ARGUMENTS` is preserved. Factory recommends Skills for new reusable workflows but continues to load this command surface.
- **Hooks**: written to `.factory/hooks.json`: "Project | `.factory/hooks.json` | Commit to share with teammates." ([docs.factory.ai/harness/hooks](https://docs.factory.ai/harness/hooks), #629).
  - The file is managed: `sync` overwrites it whole, the same as `.factory/mcp.json` below. A hand edit is lost on the next sync; change the hook spec instead (target-audit 2026-09-11, #745).
  - Nine events: `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Notification`, `Stop`, `SubagentStop`, `PreCompact`, `SessionStart`, `SessionEnd`. Unlike Claude Code, Codex, Gemini, and Qoder's shared `{"hooks": {...}}` wrapper, "Standalone `hooks.json` files are keyed directly by event name", so this adapter's document renders `{"<Event>": [{matcher, hooks: [...]}]}` at the top level with no wrapper, the same divergence Windsurf/Devin CLI's `.devin/hooks.v1.json` carries.
  - Per entry: `type` (always `"command"`, the vendor's field table documents no other value), `command`, and optional `timeout` (seconds, vendor default 60 when absent, not milliseconds).
  - `matcher` is a regex; the vendor's own "Common tool matchers" (`Execute`, `Read`, `Edit`, `Create`, `ApplyPatch`, `LS`, `Glob`, `Grep`, `Task`, `FetchUrl`, `WebSearch`) already match Claude's own spelling except three: `Bash`, `Write`, and `WebFetch`, the same three names `tools.go`'s translation table renames to `Execute`, `Create`, and `FetchUrl` for the `tools` frontmatter field. A matcher carried over from a Claude spec using one of those three still emits verbatim but folds into one coverage note, since it parses as a valid regex and then matches nothing.
  - The vendor's other matcher-group field, `commandRegex` ("Additional regex filter for Execute commands"), has no counterpart on agnostic-ai's generic hook spec and is not emitted.
- **MCP**: written to `.factory/mcp.json` under the standard `mcpServers` map, the same shape Claude Code and Cursor use (stdio: `command`/`args`/`env`, no `type`; remote: `type` + `url`/`headers`).
  - The file is managed: `sync` overwrites it whole, the same as Claude Code, Cursor, Junie, and Kiro. That is worth knowing here, because Factory's own docs send you to hand-edit it: "**Project servers cannot be removed** with `droid mcp remove` or the `/mcp` manager. To remove them, edit `.factory/mcp.json` directly" ([docs.factory.ai/harness/mcp](https://docs.factory.ai/harness/mcp)). Such an edit is lost on the next sync; remove the MCP spec instead (target-audit 2026-09-11, #737).
  - Factory's schema documents a working per-server `disabled` boolean (default `false`), unlike Claude Code, Cursor, and Copilot, so agnostic-ai passes a spec's `disabled: true` straight through instead of stripping it.
  - Both transports preserve `disabledTools`, `timeout`, and `connectTimeout` (milliseconds), including explicit zero timeouts.
  - Remote HTTP/SSE servers also accept `oauth: false` or an OAuth object with `scopes`, `resource`, `authorizationServerIssuer`, `clientId`, `clientSecret`, `clientMetadataUrl`, `tokenEndpointAuthMethod`, and `callbackPort`.
  - `x-factory` overrides each top-level option. These fields stay scoped to Factory. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target). A `type: ws` spec emits no server and raises a coverage note because Factory documents only stdio, HTTP, and SSE.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.factory.agents-dir` | `.factory/droids` |
| `outputs.factory.skills-dir` | `.agents/skills` |
| `outputs.factory.commands-dir` | `.factory/commands` |
| `outputs.factory.hooks-file` | `.factory/hooks.json` |
| `outputs.factory.mcp-file` | `.factory/mcp.json` |

## Verify

1. Install the Factory CLI ([subagents docs](https://docs.factory.ai/harness/subagents)).
2. Check the tree:
   - `ls AGENTS.md .factory/droids/ .agents/skills/ .factory/hooks.json .factory/mcp.json`
   - `grep "Generated by agnostic-ai" .factory/droids/*.md` for the provenance header (it sits after the frontmatter)
   - `python -m json.tool .factory/hooks.json > /dev/null` when hook specs exist
   - `python -m json.tool .factory/mcp.json > /dev/null`
3. Launch `droid`:
   - Each `.factory/droids/<name>.md` appears in the droid picker.
   - Each `.agents/skills/<name>/` loads as a skill.
   - Each `mcpServers.<name>` from `.factory/mcp.json` connects.
   - `/hooks` shows each entry in `.factory/hooks.json` under the Project tab.
