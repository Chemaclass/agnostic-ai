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

- **Rules**: emit with `alwaysApply: true` (override in spec frontmatter). An always-apply rule omits `globs`. A non-always rule without `globs` falls back to the Claude-spelled `paths` list (comma-joined). With neither, `globs` is omitted rather than defaulted to `**/*`, so Cursor treats the rule as "Apply Intelligently" (description-driven) or "Apply Manually" instead of attaching it to every file. Scalar globs keep minimal quoting so a hand-authored `.mdc` round-trips clean.
- **Per-file check**: `agnostic-ai explain --file <path> --target cursor` classifies each planned `.mdc` rule and every `AGENTS.md` Cursor reads (root and nested) against one project file, using the `alwaysApply`/`description`/`globs` matrix from [Rules](https://cursor.com/docs/rules).
- **Agents**: native [Cursor subagents](https://cursor.com/docs/subagents.md) at `.cursor/agents/<name>.md` (Cursor 2.4+). Frontmatter has `name` and `description`, plus `model`, `readonly`, and `is_background` when the spec declares them. The body is the system prompt.
  - Cursor subagents have no `tools` field, so a `tools` list is dropped with a coverage note. `readonly: true` is the coarse equivalent.
  - They have no effort field, so a portable `effort` is not written and raises a coverage note. Put it in the model id instead: `model: {cursor: "claude-opus-5[effort=high]"}`. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort).
  - Older flattened `.mdc` and agent-as-command copies are swept by the ledger.
- **Commands**: each command spec emits as a Markdown [Cursor command](https://cursor.com/help/customization/skills.md) under `.cursor/commands/`, with the body as the prompt. Cursor no longer has a page documenting `.cursor/commands` directly; that link (a "migrate commands to skills" FAQ) is the closest reference. Override the directory via `outputs.cursor.commands-dir`.
- **Skills**: native folders under `.cursor/skills/<name>/SKILL.md` (the [Agent Skills](https://cursor.com/docs/skills.md) layout, Cursor 2.4+), with every bundled file copied byte-for-byte. A source-layout scope moves the tree under that directory and survives import.
  - Frontmatter has `name` and `description`. `paths`, `disable-model-invocation`, `icon`, `color`, and `metadata` pass through when declared (`icon` and `color` style the badge when the skill backs a [Custom Mode](https://cursor.com/docs/agent/prompting.md#custom-modes)).
  - Older flattened `skill-<name>.mdc` copies are swept by the ledger on the next sync.
  - Cursor also loads skills from `.claude/skills/`, `.codex/skills/`, `~/.claude/skills/`, and `~/.codex/skills/` by default ([Skills](https://cursor.com/docs/skills.md)). A repo syncing `claude`, `codex`, and `cursor` has each skill read from three roots, and Cursor documents no dedupe or precedence for them. The **Include Third-Party Plugins, Skills, and Other Configs** setting (Cursor Settings → Agents → Third-Party Imports, [on by default](https://cursor.com/docs/reference/third-party-hooks.md)) controls this. Turn it off, or give the skill spec a single `target:`, if the triple read matters.
  - Subagents are cross-read too: [Subagents](https://cursor.com/docs/subagents.md) reads `.claude/agents/` and `.codex/agents/` (current project only) beside `.cursor/agents/`. On a name clash, `.cursor/` wins. So a repo syncing `claude` and `cursor` has each agent read from two roots. The page documents only Markdown agents, so it is unclear whether Cursor parses the TOML files the `codex` target writes to `.codex/agents/`.
- **Review**: review specs emit as [Bugbot](https://cursor.com/docs/bugbot) files: `.cursor/BUGBOT.md` at the repo root for unscoped specs, `<scope>/.cursor/BUGBOT.md` for scoped ones. Same-scope specs concatenate. Bugbot always includes the root file and picks up per-directory copies while walking up from changed files. Override the basename via `outputs.cursor.review-file`.
  - [Rule limits](https://cursor.com/docs/bugbot#rule-limits): each BUGBOT.md is one rule, truncated at 30,000 characters. All rules in one review are capped at 100,000 characters, and Bugbot may omit rules past that.
  - agnostic-ai never truncates. An over-cap file still emits in full, and `sync` raises a surface-gap note measured on the emitted file (header included), so small same-scope specs can trip it once concatenated. Split the review across sibling scopes to stay under the per-file cap.
  - `sync` also notes any scope whose chain (the scope and its ancestors) passes 100,000 characters on its own. Team and repository rules count against that budget too and live outside the repo, so a chain under it can still lose rules. Run `bugbot run verbose=true` or `cursor review verbose=true` on a pull request to see what Bugbot included, truncated, or omitted.
- **Environment**: environment specs emit as `.cursor/environment.json` ([background-agent](https://docs.cursor.com/background-agent) bootstrap). Spec keys pass through verbatim minus agnostic routing fields; multiple specs merge by top-level key. Override the path via `outputs.cursor.environment-file`.
- **Ignore**: ignore specs emit as `.cursorignore` (gitignore syntax). Multiple specs concatenate. Override via `outputs.cursor.ignore-file`.
- **Hooks**: emit as [Cursor Hooks](https://cursor.com/docs/hooks) in a managed `.cursor/hooks.json` (`version` + per-event arrays). Command hooks keep their `{command, matcher?}` shape. A `type: prompt` hook emits `prompt` and optional `model`. Both keep `timeout`, `loop_limit` (including `null`), `failClosed`, and `matcher`. Override the file via `outputs.cursor.hooks-file`.
  - Cursor's camelCase event names (`beforeShellExecution`, `afterFileEdit`, ...) pass through verbatim; `validate` flags unknown ones.
  - Per Cursor's "Available matchers by hook" table, fifteen events take a `matcher`: `preToolUse`, `postToolUse`, `postToolUseFailure` (tool name), `subagentStart`, `subagentStop` (subagent type), `beforeShellExecution`, `afterShellExecution` (full command string), `beforeReadFile`, `afterFileEdit` (tool name), and `beforeTabFileRead`, `afterTabFileEdit`, `beforeSubmitPrompt`, `stop`, `afterAgentResponse`, `afterAgentThought` (one fixed value each).
  - `lint` flags a matcher on any other event, except `beforeMCPExecution` and `afterMCPExecution`, which the table omits, so a warning there would be a false positive.
  - Cursor also runs Claude Code's hooks from `.claude/settings.json` by default. [Third-party hooks](https://cursor.com/docs/reference/third-party-hooks.md) merges seven sources (`.cursor/hooks.json` at rank 3, `.claude/settings.json` at rank 6), runs every matching hook from each, and maps Claude event names to Cursor ones (`PreToolUse` to `preToolUse`, and so on). So a repo syncing `claude` and `cursor` runs every hook twice. The only control is the **Include Third-Party Plugins, Skills, and Other Configs** setting (Cursor Settings → Agents → Third-Party Imports, on by default). Turn it off, or emit hooks to only one of the two targets, if the double run matters.
- **MCP**: written into `.cursor/mcp.json` under the standard `mcpServers` map (a builder shared with Claude Code). Three fields are Cursor-only ([cursor.com/docs/mcp](https://cursor.com/docs/mcp.md)):
  - A stdio server accepts `envFile`, a path to an env file with extra variables.
  - A remote (`url`) server accepts a static-OAuth `auth` object, `{CLIENT_ID, CLIENT_SECRET, scopes}` with `CLIENT_ID` required, for providers without OAuth Dynamic Client Registration.
  - A stdio server carries an explicit `"type": "stdio"`, which Cursor marks required. Claude Code [reads a type-less entry as stdio](https://code.claude.com/docs/en/mcp), so other targets sharing this builder keep type-less stdio entries.
  - A declared `roots` list still emits, but Cursor documents no per-server `roots` key (its "Roots: Supported" row is the protocol capability, and config interpolation covers only `command`, `args`, `env`, `url`, and `headers`). Treat it as passthrough.
  - `disabled: true` has no effect here; see [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

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

Both skill directories are read because [Skills](https://cursor.com/docs/skills.md) marks both as project-level, at the repository root and in nested subdirectories (the nesting becomes the spec scope). `.cursor/skills` wins a same-name clash at the same scope.

Import round-trips cleanly: a later `sync` regenerates equivalent rules, skill folders, and command files. Cursor writes no `argument-hint` or `allowed-tools` on a skill, so import leaves both on the spec. Keys Cursor does write, such as a skill's `icon` or an agent's `model`, follow the native file, so deleting one there deletes it from the spec. A rule's frontmatter comes from the `.mdc` alone, so widening `globs` to `**/*` unscopes the spec.

## Verify

1. Install Cursor from [cursor.com](https://cursor.com).
2. Check the tree: `ls .cursor/rules/ .cursor/skills/ .cursor/commands/ .cursor/mcp.json`, `grep "Generated by agnostic-ai" .cursor/rules/*.mdc` for the provenance header (it sits after the frontmatter block), `python -m json.tool .cursor/mcp.json > /dev/null`.
3. Open the project. The Rules panel loads every `.cursor/rules/*.mdc` with the right `alwaysApply` and no "failed to parse" warnings. The Skills list shows each `.cursor/skills/<name>/`, the agent picker lists each `.cursor/agents/<name>.md`, and the `/` picker lists each `.cursor/commands/<name>.md`.
4. If MCPs are configured, Settings → MCP shows every `mcpServers.<name>` green.
5. If hooks are configured, `python -m json.tool .cursor/hooks.json > /dev/null` parses. Trigger the matched event (e.g. a shell command for `beforeShellExecution`) and confirm the `command` runs.
