+++
title = "Cursor"
description = "How agnostic-ai emits Cursor configuration: native paths, capability limits, and output options."
weight = 40

[extra]
group = "Reference"
target_id = "cursor"
+++

# Cursor (`cursor`)

## Output

```
.cursor/rules/<name>.mdc
.cursor/agents/<name>.md             # one native subagent per agent spec
.cursor/skills/<name>/SKILL.md       # one folder per skill, bundled assets included
.cursor/commands/<name>.md           # one per command spec
.cursor/hooks.json                   # when hook specs exist (managed, overwritten each sync)
.cursor/mcp.json                     # when MCP entries exist
```

- **Rules**: emit with `alwaysApply: true` (override in spec frontmatter). An always-apply rule omits `globs`. A non-always rule without `globs` falls back to the Claude-spelled `paths` list (comma-joined). When both are absent, `globs` is omitted too rather than defaulted to `**/*`, so Cursor treats the rule as description-driven ("Apply Intelligently") or manual-only ("Apply Manually") instead of auto-attaching it to every file. Scalar globs keep minimal quoting so a hand-authored `.mdc` round-trips clean. (#443, #536)
- **Agents**: native [Cursor subagents](https://cursor.com/docs/subagents.md) at `.cursor/agents/<name>.md` (Cursor 2.4+): frontmatter `name` + `description` plus optional `model`, `readonly`, and `is_background` when the spec declares them; the body is the system prompt. Cursor subagents have no `tools` field, so a `tools` list drops with a coverage note; `readonly: true` is the coarse equivalent. The old flattened `.mdc` and agent-as-command emissions are gone; the ledger sweeps stale copies.
- **Commands**: each command spec emits as a [Cursor command](https://cursor.com/help/customization/skills.md) under `.cursor/commands/`: Markdown whose body is the prompt. The old `/docs/agent/chat/commands` page 308s to that link, a "migrate commands to skills" FAQ; no Cursor page documents `.cursor/commands` directly any more, so this is the closest surviving reference. Override the directory via `outputs.cursor.commands-dir`.
- **Skills**: native folders under `.cursor/skills/<name>/SKILL.md` (the [Agent Skills](https://cursor.com/docs/skills.md) layout Cursor 2.4+ discovers), with every bundled sibling file (scripts, references, assets) propagated byte-for-byte. A source-layout scope moves the native tree under that directory and survives import.

  Frontmatter carries `name` + `description`; optional `paths`, `disable-model-invocation`, `icon`, `color`, and `metadata` pass through when the spec declares them (`icon` and `color` style the badge when the skill backs a [Custom Mode](https://cursor.com/docs/agent/prompting.md#custom-modes)). The pre-native flattened `skill-<name>.mdc` copies are no longer written and get swept by the ledger on the next sync.
- **Review**: review specs emit as [Bugbot](https://cursor.com/docs/bugbot) files inside `.cursor/` directories: `.cursor/BUGBOT.md` at the repo root for unscoped specs, `<scope>/.cursor/BUGBOT.md` for scoped ones, with same-scope specs concatenated. Bugbot always includes the root file and picks up per-directory copies while traversing up from changed files. Override the basename via `outputs.cursor.review-file`. (#433)
- **Environment**: environment specs emit as `.cursor/environment.json` ([background-agent](https://docs.cursor.com/background-agent) bootstrap). The spec keys pass through verbatim minus agnostic routing fields; multiple specs merge by top-level key. Override the path via `outputs.cursor.environment-file`. (#434)
- **Ignore**: ignore specs emit as `.cursorignore` (gitignore syntax). Multiple specs concatenate. Override via `outputs.cursor.ignore-file`. (#435)
- **Hooks**: emit as [Cursor Hooks](https://cursor.com/docs/hooks) in a managed `.cursor/hooks.json` (`version` + per-event arrays). Command hooks retain their `{command, matcher?}` shape. A `type: prompt` hook instead emits `prompt` and optional `model`. Both forms preserve `timeout`, `loop_limit` (including `null`), `failClosed`, and `matcher`. Override the file via `outputs.cursor.hooks-file`. (#438)

  Cursor uses camelCase event names (`beforeShellExecution`, `afterFileEdit`, ...), passed through verbatim; `validate` flags unrecognized ones. Fifteen of those events consume a `matcher`, per the vendor's own "Available matchers by hook" table: `preToolUse`, `postToolUse`, `postToolUseFailure` (tool name), `subagentStart`, `subagentStop` (subagent type), `beforeShellExecution`, `afterShellExecution` (the full command string), `beforeReadFile`, `afterFileEdit` (tool name), and `beforeTabFileRead`, `afterTabFileEdit`, `beforeSubmitPrompt`, `stop`, `afterAgentResponse`, `afterAgentThought` (one fixed value each).

  `lint` flags a matcher on none of the fifteen (#734, #860). It also stays quiet on `beforeMCPExecution` and `afterMCPExecution`, which that table does not list; a warning there would be the same false positive in the other direction.

  Cursor also reads Claude Code's own hook file, and it does so by default. [Third-party hooks](https://cursor.com/docs/reference/third-party-hooks.md) puts `.claude/settings.json` at rank 6 of a seven-rank merge with `.cursor/hooks.json` at rank 3, and "All matching hooks from every source run." The same page maps each Claude event name onto its Cursor equivalent, `PreToolUse` onto `preToolUse` and so on. So a repo syncing `claude` and `cursor` together runs every hook twice.

  Nothing gates that. The page now reads: "Claude Code hooks load when **Include Third-Party Plugins, Skills, and Other Configs** is enabled in Cursor Settings → Agents → Third-Party Imports. The setting is on by default." The two opt-in gates documented here until 2026-09 are both gone. Turn that setting off, or emit hooks to one of the two targets, if the double run matters (#756, #865).
- **MCP**: written into `.cursor/mcp.json` under the standard `mcpServers` map (the shared builder also used by Claude Code). A stdio server accepts `envFile`, a path to an env file loading additional variables. A remote (`url`) server accepts a static-OAuth `auth` object, `{CLIENT_ID, CLIENT_SECRET, scopes}` with `CLIENT_ID` required, for a provider without OAuth Dynamic Client Registration ([cursor.com/docs/mcp](https://cursor.com/docs/mcp.md), #661). A stdio server also carries an explicit `"type": "stdio"`, which Cursor's own stdio field table marks required: "**type** | Yes | Server connection type | `"stdio"`". That field is cursor-only. Claude Code documents the opposite, "Claude Code reads an entry with no `type` as a stdio server" ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp)), so the other targets sharing this builder keep their type-less stdio entries (#895).

  Neither field is documented for the other targets sharing this builder, so both stay scoped to Cursor rather than appearing everywhere the shared schema is used. Every entry also accepts MCP `roots`, a list of `{uri, name}` objects. `disabled: true` has no effect here; see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

The MCP file is managed as a whole document. Each sync replaces `.cursor/mcp.json` from MCP specs.

## Config keys

| Key | Default |
|---|---|
| `outputs.cursor.rules-dir` | `.cursor/rules` |
| `outputs.cursor.agents-dir` | `.cursor/agents` |
| `outputs.cursor.skills-dir` | `.cursor/skills` |
| `outputs.cursor.commands-dir` | `.cursor/commands` |
| `outputs.cursor.mcp-file` | `.cursor/mcp.json` |
| `outputs.cursor.review-file` | `BUGBOT.md` |
| `outputs.cursor.environment-file` | `.cursor/environment.json` |
| `outputs.cursor.ignore-file` | `.cursorignore` |
| `outputs.cursor.hooks-file` | `.cursor/hooks.json` |

## Import

`agnostic-ai import cursor` reads `.cursor/rules/**` recursively, so nested rule directories are imported too:

| Source | Becomes |
|--------|---------|
| `.cursor/rules/<name>.mdc` | `<rules>/<name>.md` with frontmatter (`description`, `globs`, `alwaysApply`, plus any custom keys) preserved verbatim |
| `.cursor/rules/<sub>/<name>.mdc` | `<rules>/<sub>/<name>.md`, nested subdirectories preserved |
| (no `name:` in frontmatter) | `name:` injected from the filename |
| `.cursor/agents/<name>.md` | `<agents>/<name>.md` (byte-identical copy, provenance header stripped) |
| `.cursor/skills/<name>/` and `.agents/skills/<name>/` | `<skills>/<name>/`, full folder tree (SKILL.md + bundled assets) copied byte-for-byte |
| `.cursor/commands/<name>.md` | `<commands>/<name>.md` (byte-identical copy, provenance header stripped) |

Both skill directories are read because the [Skills](https://cursor.com/docs/skills.md) "Skill directories" table marks both project-level, at the repository root and in nested subdirectories (the nesting becomes the spec scope). `.cursor/skills` wins a same-name collision at the same scope (#854).

It round-trips cleanly: a later `sync` regenerates equivalent `.cursor/rules/*.mdc`, skill folders, and command files.

## Verify

1. Install Cursor from [cursor.com](https://cursor.com).
2. Check the tree: `ls .cursor/rules/ .cursor/skills/ .cursor/commands/ .cursor/mcp.json`, `grep "Generated by agnostic-ai" .cursor/rules/*.mdc` for the provenance header (it sits after the frontmatter block), `python -m json.tool .cursor/mcp.json > /dev/null`.
3. Open the project. The Rules panel loads every `.cursor/rules/*.mdc` (confirm `alwaysApply` matches each rule's frontmatter, no "failed to parse" warnings), the Skills list shows each `.cursor/skills/<name>/`, the agent picker lists each `.cursor/agents/<name>.md`, and the `/` command picker lists each `.cursor/commands/<name>.md`.
4. If MCPs are configured, Settings → MCP shows every `mcpServers.<name>` green.
5. If hooks are configured, `python -m json.tool .cursor/hooks.json > /dev/null` parses; trigger the matched event (e.g. a shell command for `beforeShellExecution`) and confirm the `command` runs.
