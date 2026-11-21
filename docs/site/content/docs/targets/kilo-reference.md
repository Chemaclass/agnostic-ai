+++
title = "Kilo native reference"
description = "Native field mappings, output overrides, and import coverage for kilo."
weight = 221

[extra]
nav_title = "Kilo reference"
group = "Reference"
+++

# Kilo native reference

Start with the [Kilo setup](@/docs/targets/kilo.md). For home configuration and ownership, see [global configuration](@/docs/global-configuration.md).

## Shell commands in global skills

`sync --scope user` writes skills to `~/.kilo/skills/`. Kilo supports inline `` !`command` `` placeholders there and in its trusted `~/.agents/skills/` and `~/.claude/skills/` directories ([shell commands in skills](https://kilo.ai/docs/customize/skills#shell-commands-in-skills)).

- Kilo asks for approval on load. `KILO_DISABLE_SKILL_SHELL` disables execution.
- Project and remote skills never execute them. A custom output directory is not trusted just because its source is global.
- The sync check accepts only inline placeholders at those user paths.
- `$ARGUMENTS`, positional arguments, and Claude's ` ```! ` blocks stay unsupported.
- Placeholders in ordinary code blocks are examples.

## Agents

Files go to `.kilo/agents/<name>.md`.

- The filename sets the agent name, so `name:` is never written.
- Frontmatter: `description` (falls back to the spec name), plus `color`, `mode`, `model` when set.
- `color` is not checked. Kilo takes hex (`#FF5733`) or a theme token (`primary`, `accent`, `error`), so `color: blue` may not show ([custom subagents](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/custom-subagents.md), [support by target](@/docs/spec-format/agents-reference.md#color-support-by-target)).
- `mode` takes OpenCode's `primary`, `subagent`, or `all`.
- `effort` is not mapped. Set `x-kilo: {variant: high}` for a variant your model supports ([config schema](https://app.kilo.ai/config.json)); names depend on the model.
- Portable `maxTurns` is written as `steps` ([agent options](https://kilo.ai/docs/customize/custom-subagents)). `x-kilo.steps` wins.
- `disable`, `hidden`, `temperature`, and `top_p` need `x-kilo`, for example `x-kilo: {temperature: 0.1}`.

### Tools become a permission map

Kilo has no `tools:` key, so a `tools` list becomes a [`permission`](https://kilo.ai/docs/customize/agent-permissions) map. `tools: [Read, Grep]` writes `permission: {"*": deny, read: {...}, grep: allow, doom_loop: ask, external_directory: ask}`.

- The catch-all comes first, because the last matching rule wins.
- `doom_loop` and `external_directory` ask at most (Kilo's default), so the catch-all never silently denies paths outside the project.
- A restricted agent keeps project permission decisions for its listed tools and denies the rest.
- Sync reads root `kilo.jsonc` and a hand-written `.kilo/kilo.jsonc` (also in preview), not global user policy.
- Pattern intersections that cannot be represented exactly become denies, so the agent can be stricter than the project. MCP server-name overlaps and Windows case differences are handled conservatively.
- `x-kilo: {permission: {...}}` wins outright.
- If no name translates, no map is written (`{"*": deny}` alone would lock the agent out).

These names translate ([permission keys](https://kilo.ai/docs/getting-started/settings/auto-approving-actions), [tool groups](https://kilo.ai/docs/automate/tools)): `Read`, `Glob`, `Grep`, `Edit`, `Write`, `Bash`, `WebFetch`, `WebSearch`, `Task`, `Skill`, `TodoRead`, `TodoWrite`, and `mcp__<server>__<tool>` as `{server}_{tool}`. Other names drop with a sync note.

## Skills

Skills go to the shared `.agents/skills/<name>/SKILL.md` tree, which Kilo loads beside its own `.kilo/skills/`. Codex, amp, zed, crush, openhands, windsurf, and augment share it.

- Kilo also scans `.claude/skills/` (in the VS Code extension, only with Claude Code Compatibility on).
- Any other `outputs.kilo.skills-dir` is added to `skills.paths` in `kilo.jsonc` ([skills](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/skills.md)).
- Your `skills.paths` entries and `skills.urls` stay.

## Rules

- Unscoped rules go into the root `AGENTS.md`, which Kilo [always loads](https://kilo.ai/docs/customize/agents-md) ("cannot be individually disabled").
- A rule already in that block gets no `.kilo/rules/` file or `instructions` entry, so it loads once.
- A rule whose text differs for Kilo, such as one with a path variable, keeps its file.
- Scoped rules use nested `AGENTS.md`. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- Kilo's [order](https://kilo.ai/docs/customize/agents-md): agent prompt, project `instructions`, `AGENTS.md`, global.
- Kilo still loads legacy `.kilocode/rules/`, which this adapter never writes.

{% <details summary="No root rules block"> %}

Kilo reads no rules block when a `file` output override moves the entry point off the root `AGENTS.md`, or when `AGENTS.md` is in `sync.unmanaged`. Check [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).

- Unscoped rules then get one file each under `.kilo/rules/`.
- Each is listed by explicit path (not a glob) in the [`instructions`](https://kilo.ai/docs/customize/custom-rules) array of `kilo.jsonc`.
- When no rule file is left, sync removes only the `instructions` entries for rules that `AGENTS.md` now carries. Your own entries stay.

{% </details> %}

## Commands

Kilo's slash-command path ([workflows](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/customize/workflows.md)). The filename sets the name.

- Frontmatter: `description`, `agent`, `model`, `variant` (reasoning-effort override, such as `low` or `high`), `subtask` when set. Close to OpenCode's list.
- Kilo reserves the name `goal` for session goals ([goals](https://kilo.ai/docs/code-with-ai/agents/goals)). It ignores a command file by that name, warns, and offers it as `/goal:command`.
- A command spec named `goal` is still written, with a note to rename it.

## MCP

Servers merge into the `mcp` map of `kilo.jsonc` (not the deprecated `mcpServers`).

- Stdio sets `type: "local"`, joins `command` and `args` into one `command` array, and uses `environment` for env vars.
- Remote sets `type: "remote"` with `url` and `headers`. `oauth: false` turns off automatic OAuth.
- `disabled: true` writes `"enabled": false`. An enabled server gets no key.
- `timeout` takes a whole number from 1 to `9007199254740991` milliseconds for either type. Sync reports an invalid value and leaves out the key, so Kilo uses its default timeout.
- `x-kilo` can override `timeout` and `oauth`.
- `sync` keeps your keys in `kilo.jsonc`. It reads JSONC, as Kilo documents, but drops comments and trailing commas with a warning.

## Hooks

Kilo auto-registers each plugin module at startup ([plugins](https://kilo.ai/docs/automate/extending/plugins)). They work like [OpenCode](@/docs/targets/opencode.md)'s, from the same sync code.

- `PreToolUse` and `PostToolUse` map to `tool.execute.before` and `tool.execute.after`.
- Other documented events use the `event` hook with an `event.type` guard.
- Matchers, exit 2 blocking, and `disabled: true` match OpenCode. Only the module shape differs: `export default { id: "<name>", server }`.
- Hooks on `shell.env`, `experimental.session.compacting`, or an unmapped event drop with a sync note.

## Settings

A settings spec's default `model` and its `x-kilo` block (for Kilo-only keys such as `sandbox`) merge into the top level of `kilo.jsonc`. An `x-kilo` `permission` instead merges tool by tool with the translated rules.

Portable `allow`, `deny`, and `ask` lists merge into the `permission` key of `kilo.jsonc` ([auto-approving actions](https://kilo.ai/docs/getting-started/settings/auto-approving-actions)). Each rule becomes one glob under one tool key, matched against the tool's arguments.

| Portable rule | Kilo `permission` |
|---|---|
| `Bash(npm run:*)` in `allow` | `bash: {"npm run *": "allow"}` |
| `Bash(rm -rf /)` in `deny` | `bash: {"rm -rf /": "deny"}` |
| `Read(docs/*)` in `allow` | `read: {"docs/*": "allow"}` |
| Bare `Bash` in `allow` | `bash: {"*": "allow"}` |
| `mcp__github__list_issues` | `github_list_issues` |

- A rule in two lists takes the stricter action.
- `Write` and `Edit` rules both land under `edit`, the one permission Kilo's edit and write tools check (`packages/opencode/src/tool/write.ts`). So `Edit(.env)` in `deny` blocks writing `.env` even with a `Write` allow, and an agent that lists `Write` can also use the edit tool.
- Inside one tool, patterns go allow, ask, deny. Kilo takes the last match, so a narrower allow never bypasses a broader deny (as in Claude Code).
- `WebFetch(domain:example.test)` drops with a sync note, because Kilo matches URLs. Unknown tool names drop too.
- `x-kilo.permission` writes Kilo's own map. It wins for the tool keys it names, and that spec's portable lists are skipped.
- Your entries for tools agnostic-ai does not set stay.

This adapter never writes `.kilo/kilo.jsonc`. A hand-written one merges over the root `kilo.jsonc` ([config order](https://github.com/Kilo-Org/kilocode/blob/main/packages/kilo-docs/pages/getting-started/settings/index.md#config-file-precedence)), so its `mcp` or `instructions` replace the generated ones.

## Ignore

Ignore specs write `.kilocodeignore` in the project root. Kilo's [migrator](https://kilo.ai/docs/customize/context/kilocodeignore), not this adapter, turns it into read and edit denials.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.kilo.rules-dir` | `.kilo/rules` | |
| `outputs.kilo.agents-dir` | `.kilo/agents` | |
| `outputs.kilo.skills-dir` | `.agents/skills` | |
| `outputs.kilo.commands-dir` | `.kilo/commands` | |
| `outputs.kilo.hooks-dir` | `.kilo/plugin` | Kilo only loads plugins from `plugin/` or `plugins/`, so moving this stops the hooks loading |
| `outputs.kilo.mcp-file` | `kilo.jsonc` | |
| `outputs.kilo.ignore-file` | `.kilocodeignore` | |

With `builtins: [memory]`, sync adds both [shared memory](@/docs/memory.md) indexes to `instructions`.

- While rules go into `AGENTS.md` (the default), your own entries stay. Dropping the built-in removes only those two.
- When sync lists rule files in `instructions`, it owns the whole list.
- With `memory.personal: repo`, the personal store is outside the project root, and Kilo [ignores project-declared files there](https://github.com/Kilo-Org/kilocode/blob/v7.8.8/packages/opencode/src/session/instruction.ts). Sync writes only the shared index and prints a note.
- To load that store, add its `MEMORY.md` to `instructions` in `~/.config/kilo/kilo.jsonc`. See [repo mode](@/docs/memory.md#one-store-per-repository).

## Import

`agnostic-ai import kilo` reads `.kilocodeignore` and two fields from the root `kilo.jsonc`.

| Source | Becomes |
|---|---|
| Default `model` | `settings/kilo.yaml` |
| Portable entries in the `permission` map | `settings/permissions-kilo.yaml` |

- Keys with no portable spelling (`external_directory`, `lsp`, `doom_loop`, namespaced MCP keys) stay in the file.
- Generated rule sections in `AGENTS.md` are recovered when their source specs are missing.
- Not imported: hand-written instructions, native rule directories, agents, skills, commands, MCP servers, and plugin hooks.
