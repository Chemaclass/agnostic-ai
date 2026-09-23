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
- **Per-file check**: `agnostic-ai explain --file <path> --target cursor` classifies each planned `.mdc` rule and every `AGENTS.md` Cursor reads (root and nested) against one project file, using the `alwaysApply`/`description`/`globs` matrix from [Rules](https://cursor.com/docs/rules). (#1036)
- **Agents**: native [Cursor subagents](https://cursor.com/docs/subagents.md) at `.cursor/agents/<name>.md` (Cursor 2.4+): frontmatter `name` + `description` plus optional `model`, `readonly`, and `is_background` when the spec declares them; the body is the system prompt. Cursor subagents have no `tools` field, so a `tools` list drops with a coverage note; `readonly: true` is the coarse equivalent. They have no effort field either, so a portable `effort` is not written and raises a coverage note. Put it in the model id instead: `model: {cursor: "claude-opus-5[effort=high]"}` resolves through the per-target model map with no extra syntax. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort). The old flattened `.mdc` and agent-as-command emissions are gone; the ledger sweeps stale copies.
- **Commands**: each command spec emits as a [Cursor command](https://cursor.com/help/customization/skills.md) under `.cursor/commands/`: Markdown whose body is the prompt. The old `/docs/agent/chat/commands` page 308s to that link, a "migrate commands to skills" FAQ; no Cursor page documents `.cursor/commands` directly any more, so this is the closest surviving reference. Override the directory via `outputs.cursor.commands-dir`.
- **Skills**: native folders under `.cursor/skills/<name>/SKILL.md` (the [Agent Skills](https://cursor.com/docs/skills.md) layout Cursor 2.4+ discovers), with every bundled sibling file (scripts, references, assets) propagated byte-for-byte. A source-layout scope moves the native tree under that directory and survives import.

  Frontmatter carries `name` + `description`; optional `paths`, `disable-model-invocation`, `icon`, `color`, and `metadata` pass through when the spec declares them (`icon` and `color` style the badge when the skill backs a [Custom Mode](https://cursor.com/docs/agent/prompting.md#custom-modes)). The pre-native flattened `skill-<name>.mdc` copies are no longer written and get swept by the ledger on the next sync.

  Cursor also reads Claude's and Codex's skill directories, and it does so by default. [Skills](https://cursor.com/docs/skills.md) says: "For compatibility, Cursor also loads skills from Claude and Codex directories: `.claude/skills/`, `.codex/skills/`, `~/.claude/skills/`, and `~/.codex/skills/`." So a repo syncing `claude`, `codex`, and `cursor` together has one skill read from three project roots. Whether Cursor dedupes across them is undocumented: no Cursor page states a precedence or a merge rule for the compatibility roots, so do not assume either. The same [Third-party hooks](https://cursor.com/docs/reference/third-party-hooks.md) toggle gates it, **Include Third-Party Plugins, Skills, and Other Configs** under Cursor Settings → Agents → Third-Party Imports, and it is on by default. Turn it off, or give the skill spec a single `target:`, if the triple read matters (#957).

  Subagents cross-read the same way. [Subagents](https://cursor.com/docs/subagents.md) lists `.claude/agents/` ("Current project only (Claude compatibility)") and `.codex/agents/` ("Current project only (Codex compatibility)") beside `.cursor/agents/` in its file-locations table, and states the precedence for a name collision: "Project subagents take precedence when names conflict. When multiple locations contain subagents with the same name, `.cursor/` takes precedence over `.claude/` or `.codex/`." So a repo syncing `claude` and `cursor` has each agent read from two roots, and `.cursor/` wins a same-name clash. The page documents only Markdown agents, so it does not show that Cursor parses the TOML files the `codex` target writes to `.codex/agents/` (#1079).
- **Review**: review specs emit as [Bugbot](https://cursor.com/docs/bugbot) files inside `.cursor/` directories: `.cursor/BUGBOT.md` at the repo root for unscoped specs, `<scope>/.cursor/BUGBOT.md` for scoped ones, with same-scope specs concatenated. Bugbot always includes the root file and picks up per-directory copies while traversing up from changed files. Override the basename via `outputs.cursor.review-file`. (#433)
- **Environment**: environment specs emit as `.cursor/environment.json` ([background-agent](https://docs.cursor.com/background-agent) bootstrap). The spec keys pass through verbatim minus agnostic routing fields; multiple specs merge by top-level key. Override the path via `outputs.cursor.environment-file`. (#434)
- **Ignore**: ignore specs emit as `.cursorignore` (gitignore syntax). Multiple specs concatenate. Override via `outputs.cursor.ignore-file`. (#435)
- **Hooks**: emit as [Cursor Hooks](https://cursor.com/docs/hooks) in a managed `.cursor/hooks.json` (`version` + per-event arrays). Command hooks retain their `{command, matcher?}` shape. A `type: prompt` hook instead emits `prompt` and optional `model`. Both forms preserve `timeout`, `loop_limit` (including `null`), `failClosed`, and `matcher`. Override the file via `outputs.cursor.hooks-file`. (#438)

  Cursor uses camelCase event names (`beforeShellExecution`, `afterFileEdit`, ...), passed through verbatim; `validate` flags unrecognized ones. Fifteen of those events consume a `matcher`, per the vendor's own "Available matchers by hook" table: `preToolUse`, `postToolUse`, `postToolUseFailure` (tool name), `subagentStart`, `subagentStop` (subagent type), `beforeShellExecution`, `afterShellExecution` (the full command string), `beforeReadFile`, `afterFileEdit` (tool name), and `beforeTabFileRead`, `afterTabFileEdit`, `beforeSubmitPrompt`, `stop`, `afterAgentResponse`, `afterAgentThought` (one fixed value each).

  `lint` flags a matcher on none of the fifteen (#734, #860). It also stays quiet on `beforeMCPExecution` and `afterMCPExecution`, which that table does not list; a warning there would be the same false positive in the other direction.

  Cursor also reads Claude Code's own hook file, and it does so by default. [Third-party hooks](https://cursor.com/docs/reference/third-party-hooks.md) puts `.claude/settings.json` at rank 6 of a seven-rank merge with `.cursor/hooks.json` at rank 3, and "All matching hooks from every source run." The same page maps each Claude event name onto its Cursor equivalent, `PreToolUse` onto `preToolUse` and so on. So a repo syncing `claude` and `cursor` together runs every hook twice.

  Nothing gates that. The page now reads: "Claude Code hooks load when **Include Third-Party Plugins, Skills, and Other Configs** is enabled in Cursor Settings → Agents → Third-Party Imports. The setting is on by default." The two opt-in gates documented here until 2026-09 are both gone. Turn that setting off, or emit hooks to one of the two targets, if the double run matters (#756, #865).
- **MCP**: written into `.cursor/mcp.json` under the standard `mcpServers` map (the shared builder also used by Claude Code). A stdio server accepts `envFile`, a path to an env file loading additional variables. A remote (`url`) server accepts a static-OAuth `auth` object, `{CLIENT_ID, CLIENT_SECRET, scopes}` with `CLIENT_ID` required, for a provider without OAuth Dynamic Client Registration ([cursor.com/docs/mcp](https://cursor.com/docs/mcp.md), #661). A stdio server also carries an explicit `"type": "stdio"`, which Cursor's own stdio field table marks required: "**type** | Yes | Server connection type | `"stdio"`". That field is cursor-only. Claude Code documents the opposite, "Claude Code reads an entry with no `type` as a stdio server" ([code.claude.com/docs/en/mcp](https://code.claude.com/docs/en/mcp)), so the other targets sharing this builder keep their type-less stdio entries (#895).

  Neither field is documented for the other targets sharing this builder, so both stay scoped to Cursor rather than appearing everywhere the shared schema is used. A `roots` list still emits when a spec declares one, but no Cursor page documents `roots` as a per-server `mcp.json` key, so treat it as passthrough rather than a supported field. The capability matrix under "Protocol and extension support" rows **Roots** as "Supported", which is the MCP protocol capability, not a config field; neither per-server field table has a `roots` row, and "Config interpolation" resolves variables in `command`, `args`, `env`, `url`, and `headers` only ([cursor.com/docs/mcp](https://cursor.com/docs/mcp.md), target-audit 2026-09-20). `disabled: true` has no effect here; see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

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
| `.cursor/agents/<name>.md` | `<agents>/<name>.md`, provenance header stripped and the spec's own frontmatter keys kept |
| `.cursor/skills/<name>/` and `.agents/skills/<name>/` | `<skills>/<name>/`, full folder tree: bundled assets byte-for-byte, SKILL.md merged onto the existing spec |
| `.cursor/commands/<name>.md` | `<commands>/<name>.md`, provenance header stripped and the spec's own frontmatter keys kept |

Both skill directories are read because the [Skills](https://cursor.com/docs/skills.md) "Skill directories" table marks both project-level, at the repository root and in nested subdirectories (the nesting becomes the spec scope). `.cursor/skills` wins a same-name collision at the same scope (#854).

It round-trips cleanly: a later `sync` regenerates equivalent `.cursor/rules/*.mdc`, skill folders, and command files. Cursor writes no `argument-hint` or `allowed-tools` on a skill, and importing one leaves both on the spec rather than deleting what it cannot read back. Keys cursor does write, such as a skill's `icon` or an agent's `model`, follow the native file, so deleting one there deletes it from the spec. A rule's frontmatter comes from the `.mdc` alone, so widening `globs` to `**/*` unscopes the spec.

## Verify

1. Install Cursor from [cursor.com](https://cursor.com).
2. Check the tree: `ls .cursor/rules/ .cursor/skills/ .cursor/commands/ .cursor/mcp.json`, `grep "Generated by agnostic-ai" .cursor/rules/*.mdc` for the provenance header (it sits after the frontmatter block), `python -m json.tool .cursor/mcp.json > /dev/null`.
3. Open the project. The Rules panel loads every `.cursor/rules/*.mdc` (confirm `alwaysApply` matches each rule's frontmatter, no "failed to parse" warnings), the Skills list shows each `.cursor/skills/<name>/`, the agent picker lists each `.cursor/agents/<name>.md`, and the `/` command picker lists each `.cursor/commands/<name>.md`.
4. If MCPs are configured, Settings → MCP shows every `mcpServers.<name>` green.
5. If hooks are configured, `python -m json.tool .cursor/hooks.json > /dev/null` parses; trigger the matched event (e.g. a shell command for `beforeShellExecution`) and confirm the `command` runs.
