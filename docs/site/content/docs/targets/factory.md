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
.factory/settings.json             # when a settings entry carries a model, a shell-command rule, or an x-factory key
```

Factory [Droid](https://docs.factory.ai/harness/subagents) reads the root `AGENTS.md` natively and loads custom droids from `.factory/droids/`. Each agent emits as one `<name>.md` profile with `name`, `description`, and optional `model` / `tools` frontmatter (`tools` translates onto Droid CLI's own tool IDs, see below); arbitrary `x-factory` keys pass through. A portable `mcpServers` list emits as-is, narrowing which servers the droid may reach: "Setting `mcpServers: []` excludes every MCP server, even globally configured ones", so write the servers you want rather than an empty list. A portable `effort`, scalar or per-target map, emits as Factory's own `reasoningEffort`, which documents `low`, `medium`, and `high` only; `xhigh`, `max`, and Qoder's integer budgets are dropped with a coverage note, and the vendor ignores the field entirely under `model: inherit`. See [`mcpServers`](@/docs/spec-format.md#mcpservers-support-by-target) and [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort).

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
- **Settings**: a portable `model` merges into `<git-root>/.factory/settings.json`, and a portable `effort` merges as the top-level `reasoningEffort` the [CLI settings](https://docs.factory.ai/droid-cli/settings) page documents: `none`, `dynamic`, `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, or `max`, with each model accepting a subset. Another value raises a coverage note. The managed `sessionDefaultSettings.reasoningEffort` is a different key and is not written. Factory documents that project tier on its hierarchical-settings page, not on the CLI settings page whose "Where settings live" table lists `~/.factory/settings.json` alone: "Settings are authored in `.factory/` folders, using the same schema at every level", with the levels table rowing "**Project** | `<git-root>/.factory/` | Repo maintainers" and "Each `.factory/` folder can contain: `settings.json`: general settings (models, safety, preferences, telemetry)" ([docs.factory.ai/enterprise/hierarchical-settings-and-org-control](https://docs.factory.ai/enterprise/hierarchical-settings-and-org-control)). The skills page names the same file: "the **Project** tab writes to `<project>/.factory/settings.json`" ([docs.factory.ai/harness/skills](https://docs.factory.ai/harness/skills)).
  - The file is merged, not overwritten, unlike `.factory/hooks.json` and `.factory/mcp.json`. Only `model` and the keys written under `x-factory` are ever set, so `disabledSkills` and anything else in that file survives the sync.
  - An `x-factory` block on a settings spec merges into the same file, for the Factory keys this tool does not model. `sandbox` is the case it exists for: kernel-enforced isolation where "a blocked read, write, or connection is denied by the kernel rather than relying on Droid to police itself", enabling it moves the baseline for every unlisted path, `denyWrite` overrides `allowWrite`, there is no `ask` tier, and `network.allowedDomains` is egress filtering. None of that maps onto a portable permission rule, so the hatch is the answer rather than a spec field (#949). Writing one of the three command lists there adds to it: `x-factory.commandBlocklist: ["author-only"]` beside a portable `deny: ["Bash(rm:*)"]` writes both patterns, so a hard block this tool translated cannot fall out of the file because a Factory-only pattern joined it (#966).
  - The portable `allow`, `deny`, and `ask` lists write Factory's three command lists. The vendor types each as `string[]` of "Shell command patterns ... (accumulated across levels)" ([docs.factory.ai/enterprise/hierarchical-settings-and-org-control](https://docs.factory.ai/enterprise/hierarchical-settings-and-org-control)) and shows both spellings, bare (`["ls", "pwd", "dir"]`) and prefix-glob (`["npm *", "sudo *"]`), so `Bash(npm:*)` becomes `"npm *"` and `Bash(curl)` becomes `"curl"` (#948).

| portable | Factory key | why |
|---|---|---|
| `allow` | `commandAllowlist` | "Patterns that are always allowed to run without additional approval." |
| `ask` | `commandDenylist` | "Patterns that always require explicit confirmation. A denylisted command can still run if the user approves it." |
| `deny` | `commandBlocklist` | "Patterns that can never run ... a blocked command has no approval path." |

  - Read that table before assuming the names match. Factory's denylist prompts, so mapping portable `deny` onto it would turn a hard block into an approval prompt. The vendor asserts the prompt behavior on three pages, not only in the blocklist's contrast clause.
  - Precedence needs no work on this side, because Factory's matches agnostic-ai's own: "Commands that appear in both the allowlist and denylist default to the denylist behavior. The blocklist always takes precedence over both."
  - These three keys are shell-command patterns, so a rule scoping a path, a URL, or an MCP tool has no spelling among them and raises a coverage note instead. `Read(src/**)` still reaches nothing here.
  - A list is written only when at least one rule translates into it, so a list you maintain by hand survives a sync that has nothing to put there. A sync that does have something replaces that key.

Scoped skills emit at `<scope>/.factory/skills/<name>/SKILL.md`, with bundled assets. Unscoped skills keep `.agents/skills/`. An explicit `outputs.factory.skills-dir` replaces the skills directory at both the root and each scope; scopes remain distinct. Files listed as unmanaged retain their contents.

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

A droid's `tools` renames back: `Execute` to `Bash`, `Create` to `Write`, `FetchUrl` to `WebFetch`. A list holding a category (`read-only`) or an MCP tool ID has no portable spelling, so it lands under `x-factory.tools` untouched. `reasoningEffort` becomes `effort`, and every other droid key lands under `x-factory`.

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
   - `grep "Generated by agnostic-ai" .factory/droids/*.md` for the provenance header (it sits after the frontmatter)
   - `python -m json.tool .factory/hooks.json > /dev/null` when hook specs exist
   - `python -m json.tool .factory/mcp.json > /dev/null`
3. Launch `droid`:
   - Each `.factory/droids/<name>.md` appears in the droid picker.
   - Each `.agents/skills/<name>/` loads as a skill.
   - Each `mcpServers.<name>` from `.factory/mcp.json` connects.
   - `/hooks` shows each entry in `.factory/hooks.json` under the Project tab.
