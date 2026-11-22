+++
title = "Trae native reference"
description = "Native field mappings, config keys, and import coverage for trae."
weight = 181

[extra]
nav_title = "Trae reference"
group = "Reference"
+++

# Trae native reference

Start with the [Trae setup](@/docs/targets/trae.md).

## Native mappings


Trae reads `AGENTS.md` only when **Include AGENTS.md in the context** is on (Settings > Rules & Memories). See [loading and limits](@/docs/targets/trae.md#loading-and-limits).
### Rules

Sync always writes `description`, `globs`, and `alwaysApply` (Cursor's `.mdc` fields for when a rule applies), because Trae documents no default for a file without them.

- `alwaysApply` is `true`, or `false` when the spec sets `globs` other than a catch-all such as `**/*`. A `true` rule omits `globs`.
- `alwaysApply: false` with no `globs` uses the Claude `paths` list, joined with commas.
- `x-trae.scene: git_message` marks a rule for AI-written commit messages.

### Agents

[Subagents docs](https://docs.trae.ai/ide/subagents). The body is the system prompt.

- Frontmatter: `name` and `description` (required); optional `model`, `tools`, `disallowedTools`, `mcpServers`.
- `tools` is written as a comma-joined string, since Trae uses Claude-style names (`Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write`, `WebFetch`, `WebSearch`, plus `Skill`, `LSP`, `TodoWrite`, `mcp__<server>__<tool>`).
- Omit `tools` to load every available tool.
- An explicitly empty list (`can: []`, `tools: []`, `tools: ""`, or `x-trae.tools`) writes `tools: ""`, which disables all tools. Import keeps this through later syncs.
- Trae accepts only its built-in model IDs. A generic `model` drops with a sync note. Use `model: {trae: <id>}` or `x-trae.model`.
- `disallowedTools` and `mcpServers` come from `x-trae`. `x-trae.disallowedTools` is a comma-separated list of blocked tools and wins over `tools`.
- A name starts with an ASCII letter, ends with a letter or digit, uses only letters, digits, or hyphens, and has at most 50 characters. Sync rejects any other name.
- Subagents need Settings > Beta > Subagents > Enable Subagents Directory. Trae does not say whether it is on by default.

### Skills

[Skills docs](https://docs.trae.ai/ide/skills).

- Path: `.trae/skills/<name>/SKILL.md`, with `name` and `description`.
- Files beside it (`examples/`, `templates/`, `resources/`) are copied as they are.
- A flat file under `.trae/rules/` never loads as a skill.

### Commands

[Slash-commands docs](https://docs.trae.ai/ide/slash-commands). Frontmatter is only `name` and `description` (the documented fields). The body is the prompt. Trae reads commands up to 3 levels deep. Sync writes them flat.

### MCP servers

[MCP docs](https://docs.trae.ai/ide/add-mcp-servers). A root `mcpServers` map.

- Stdio: `command` (required), `args`, `env`. HTTP: `url` (required), `headers`.
- Trae works out `type` from `command` or `url`, so none is written.
- A stdio `command` with spaces fails to parse in Trae.
- Trae has no per-server `disabled` key, only a project toggle (Settings > MCP). `disabled: true` is dropped with a sync note.

### Hooks

`.trae/hooks.json` is the project tier of Trae's [hook configuration](https://docs.trae.ai/ide/hook-configuration-reference). [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests hooks before a session, with Trae's event data, shell, and timeout.

- A `version` field (always 1) wraps the `hooks` map. Each event holds `{matcher, hooks: [{type, command, timeout}]}` groups.
- Claude Code, Codex, OpenHands, and Qoder use the same shape, so one hook spec feeds all five.
- The `hook run` context preview recognizes successful plain stdout on `SessionStart` and `UserPromptSubmit`. JSON `hookSpecificOutput.additionalContext` applies to those events and `PreToolUse` and `PostToolUse`. `Notification` ignores stdout; `Stop` does not read `additionalContext`.
- Six [events](https://docs.trae.ai/ide/automate-actions-with-hooks): `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`.
- Each entry takes `type` (only `command`), `command`, and `timeout` (seconds, default 30).
- `loop_limit` caps how often a `Stop` hook can block the agent (default 5).
- **Hook `tool_name` values differ from subagent `tools`**: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `LS`, `RunCommand`, `WebSearch`, `WebFetch`, `AskUserQuestion`, `Skill`, `mcp__<serverName>__<toolName>`. There is no `TodoWrite`.
- The terminal tool is `RunCommand`, so `matcher: Bash` is written as is, raises a sync note, and matches nothing.
- `matcher` applies to `PreToolUse`, `PostToolUse`, and `Notification` only. On `Notification` it selects a type (`idle_prompt`, `permission_prompt`, ...).
- Project hooks do nothing until you enable them under Settings > Hooks and accept the security warning.
- Trae also runs Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json` once you turn on **Import Hook configuration in CLAUDE** (off by default, same warning). Syncing `claude` and `trae` then runs every hook twice.

### Ignore

[Ignore docs](https://docs.trae.ai/ide/ignore-files). This is the file Trae's Settings > Indexing & Docs creates.

- It adds to `.gitignore`, which Trae already honors. Specs are joined. Override with `outputs.trae.ignore-file`.
- On clients released before October 22, 2026, it keeps paths out of indexing and `@Folder` context. Run a Build under Settings > Indexing & Docs after a sync, because the change applies only after re-indexing.
- Trae removes Indexing & Docs from clients released on or after that date and does not say how `.trae/.ignore` applies there.

## Shared memory

With `builtins: [memory]`, sync adds the native `memory-trae-session-start` hook for `SessionStart` to `.trae/hooks.json`.

- Once you enable project hooks in Settings > Hooks, it runs `agnostic-ai project -- hook memory --target trae` before the first message. It adds up to 6,000 bytes from the personal and project memory indexes.
- Trae's own Memories stay available.
- Trae sets `TRAE_PROJECT_DIR` to the hook's workspace, so the command finds that project's indexes even when its shell starts elsewhere.
- The command uses the project's installed agnostic-ai package when one is declared. Install or upgrade it before starting Trae.
- If hooks are disabled, the shared memory rule still tells Trae to read the indexes.

<details>
<summary>Agents from older versions</summary>

For every current agent, sync removes a generated copy at the old `.trae/rules/agent-<name>.md` path.

</details>

## Config keys

| Key | Default |
| --- | --- |
| `outputs.trae.rules-dir` | `.trae/rules` |
| `outputs.trae.rules-mode` | unset; set `entry-point` when Trae's **Include AGENTS.md in the context** switch is on |
| `outputs.trae.agents-dir` | `.trae/agents` |
| `outputs.trae.skills-dir` | `.trae/skills` |
| `outputs.trae.commands-dir` | `.trae/commands` |
| `outputs.trae.hooks-file` | `.trae/hooks.json` |
| `outputs.trae.ignore-file` | `.trae/.ignore` |
| `outputs.trae.mcp-file` | `.trae/mcp.json` |

## Import

`agnostic-ai import trae` reads these back:

| Source | Becomes |
|--------|---------|
| `.trae/rules/*.md` | rules, agents, and skills by [filename prefix](@/docs/cli-reference/import.md#filename-prefix-reclassification) |
| `.trae/agents/<name>.md` | `<agents>/<name>.md`, copied, minus the generated header |
| `.trae/skills/<name>/SKILL.md` and `.agents/skills/<name>/SKILL.md` (+ bundled files) | `<skills>/<name>/SKILL.md` (folder copied as is) |
| `.trae/commands/<name>.md` | `<commands>/<name>.md` |
| `.trae/hooks.json` | one spec for consecutive commands with the same `timeout` in a matcher group, carrying `loop_limit` |
| `.trae/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml`, a `url`-only entry becomes `type: http` |
| `.trae/.ignore` | an ignore spec |

- Import reads `.trae/skills/` first, then `.agents/skills/`. For the same folder name, Trae's own copy wins.
- Shared skills keep the same checks for existing specs and generated assets. Sync still writes `.trae/skills/` by default.
- Consecutive hook commands with the same event, matcher, and timeout share a spec with a `command:` list.
- Fresh imports keep native command order with position prefixes in spec names. Keep those prefixes when renaming imported hooks.
- Import skips `version` (always 1), a `loop_limit` of 0 (Trae treats it as unset), and the user `~/.trae/hooks.json`.
