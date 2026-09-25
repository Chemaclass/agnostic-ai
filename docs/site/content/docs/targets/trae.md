+++
title = "Trae"
description = "How agnostic-ai emits Trae configuration: native paths, capability limits, and output options."
weight = 180

[extra]
group = "Reference"
target_id = "trae"
+++

# Trae (`trae`)

## Output

```
AGENTS.md                     # pointer body (shared path)
.trae/rules/<name>.md         # one per rule
.trae/agents/<name>.md        # one per agent (project subagent)
.trae/skills/<name>/SKILL.md  # one folder per skill
.trae/commands/<name>.md      # one per command
.trae/hooks.json              # when hook entries exist
.trae/.ignore                 # when ignore entries exist
.trae/mcp.json                # MCP server registry
```

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads rules from `.trae/rules/`, the root `AGENTS.md`, and project subagents from `.trae/agents/`.

- **Rules**: every `.trae/rules/*.md` file carries `description`, `globs`, and `alwaysApply` frontmatter, the same activation fields as Cursor's `.mdc` rules.
  - `alwaysApply` defaults to `true`, and a `true` rule omits `globs`. `alwaysApply: false` with no `globs` falls back to the Claude `paths` list, comma-joined.
  - Trae does not document the default for a file with none of the keys, so all three are always written.
  - `x-trae.scene: git_message` marks a rule for AI-generated commit messages. It combines with the other activation fields.
- **Agents**: one project subagent per agent at `.trae/agents/<name>.md` ([subagents docs](https://docs.trae.ai/ide/subagents)).
  - Frontmatter carries `name` and `description` (required), plus optional `model`, `tools`, `disallowedTools`, and `mcpServers`. The body is the system prompt.
  - A managed copy at the old `.trae/rules/agent-<name>.md` path is swept for every current agent.
  - `tools` passes through untranslated, because Trae uses Claude-style names (`Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write`, `WebFetch`, `WebSearch`, plus `Skill`, `LSP`, `TodoWrite`, and `mcp__<server>__<tool>`). It is written as a comma-joined string, not a YAML list.
  - Trae accepts only its own built-in model IDs (`gpt-5.4`, `minimax-m3`, ...), so a generic `model` drops with a coverage note. Set one with `model: {trae: <id>}` or `x-trae.model`.
  - `disallowedTools` and `mcpServers` come from `x-trae`. `x-trae.disallowedTools` is a comma-joined denylist that wins over `tools`.
  - Subagents need Settings > Beta > Subagents > Enable Subagents Directory turned on. Trae does not state the default.
- **Skills**: one folder per skill at `.trae/skills/<name>/SKILL.md` ([skills docs](https://docs.trae.ai/ide/skills)) with `name` and `description` frontmatter. Sibling assets (`examples/`, `templates/`, `resources/`) copy byte-for-byte. A flat file under `.trae/rules/` never loads as a skill.
- **Commands**: one file per command at `.trae/commands/<name>.md`, with `name` and `description` frontmatter and the body as the prompt ([slash-commands docs](https://docs.trae.ai/ide/slash-commands)). No other fields are documented, so none are written.
  - Trae reads commands up to 3 levels deep. This adapter writes every command flat.
- **MCP servers**: merge into `.trae/mcp.json` under a root `mcpServers` map ([MCP docs](https://docs.trae.ai/ide/add-mcp-servers)).
  - Stdio entries carry `command` (required) plus optional `args` and `env`. HTTP entries carry `url` (required) plus optional `headers`. No `type` is written: Trae infers it from `command` or `url`.
  - Trae has no per-server `disabled` key, only a project-level toggle under Settings > MCP, so `disabled: true` is stripped with a coverage note.
  - A stdio `command` must not contain spaces, or Trae fails to parse it.
- **Hooks**: written to `.trae/hooks.json`, the project tier of Trae's [hook configuration](https://docs.trae.ai/ide/hook-configuration-reference).
  - The file wraps a `hooks` map in a `version` field (always 1). Each event holds `{matcher, hooks: [{type, command, timeout}]}` groups, the same shape as Claude Code, Codex, OpenHands, and Qoder, so one hook spec feeds all five.
  - Six [events](https://docs.trae.ai/ide/automate-actions-with-hooks): `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`. Per entry: `type` (only `command`), `command`, and `timeout` (seconds, default 30).
  - A spec's `loop_limit` also emits. It caps how often a `Stop` hook may block the agent. Leave it unset for the default of 5.
  - **Watch the matcher names.** Hook `tool_name` values differ from subagent `tools`: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `LS`, `RunCommand`, `WebSearch`, `WebFetch`, `AskUserQuestion`, `Skill`, and `mcp__<serverName>__<toolName>`. The terminal tool is `RunCommand`, not `Bash`, and there is no `TodoWrite`. A `matcher: Bash` emits verbatim with a coverage note, because it matches nothing.
  - `matcher` applies to `PreToolUse`, `PostToolUse`, and `Notification` only. On `Notification` it selects a notification type (`idle_prompt`, `permission_prompt`, ...).
  - Project hooks stay inert until you enable them under Settings > Hooks and accept the security warning.

  Trae can also read Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json`, and runs all enabled sources together. So syncing `claude` and `trae` runs every hook twice once you turn on **Import Hook configuration in CLAUDE**. That switch is off by default, behind the same security warning.
- **Ignore**: ignore specs emit as `.trae/.ignore`, the file Trae's Settings > Indexing & Docs creates ([ignore docs](https://docs.trae.ai/ide/ignore-files)).
  - It supplements `.gitignore`, which Trae already honors, and excludes paths from indexing and `#Workspace` / `#Folder` context.
  - It applies only after re-indexing. Run a Build under Settings > Indexing & Docs after a sync.
  - Multiple specs concatenate. Override via `outputs.trae.ignore-file`.

Agent names must start with an ASCII letter, end with a letter or digit, contain only letters, digits or hyphens, and be at most 50 characters. Sync rejects invalid names before writing the native agent.

## Import

`agnostic-ai import trae` reverses the Trae layout:

| Source | Becomes |
|--------|---------|
| `.trae/rules/*.md` | rules, agents, and skills by [filename prefix](@/docs/cli-reference.md#filename-prefix-reclassification) |
| `.trae/agents/<name>.md` | `<agents>/<name>.md`, byte-for-byte minus the provenance header |
| `.trae/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `.trae/commands/<name>.md` | `<commands>/<name>.md` |
| `.trae/hooks.json` | one hook spec per matcher group, carrying `loop_limit` |
| `.trae/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml`, a `url`-only entry inferring `type: http` |
| `.trae/.ignore` | an ignore spec |

Commands sharing an event and matcher collapse into one spec with a `command:` list, since sync merges them into one group. The `version` field is not imported (it is always 1), and neither is a `loop_limit` of 0, which Trae treats as unset.

The global `~/.trae/hooks.json` is not read.

## Config keys

| Key | Default |
| --- | --- |
| `outputs.trae.rules-dir` | `.trae/rules` |
| `outputs.trae.agents-dir` | `.trae/agents` |
| `outputs.trae.skills-dir` | `.trae/skills` |
| `outputs.trae.commands-dir` | `.trae/commands` |
| `outputs.trae.hooks-file` | `.trae/hooks.json` |
| `outputs.trae.ignore-file` | `.trae/.ignore` |
| `outputs.trae.mcp-file` | `.trae/mcp.json` |

## Verify

1. Install Trae from [trae.ai](https://www.trae.ai).
2. Check the tree:
   - `ls AGENTS.md .trae/rules/ .trae/agents/ .trae/skills/ .trae/commands/ .trae/mcp.json .trae/hooks.json .trae/.ignore`
   - `grep "Generated by agnostic-ai" .trae/rules/*.md`
   - `python -m json.tool .trae/mcp.json > /dev/null`
   - `python -m json.tool .trae/hooks.json > /dev/null`
3. Open the project:
   - The Rules panel lists every `.trae/rules/*.md` with no parse warnings.
   - Each `.trae/skills/<name>/` loads as a skill.
   - Each `.trae/commands/<name>.md` is invokable from chat.
   - Settings > MCP lists each `.trae/mcp.json` server (project-level MCP toggled on).
4. Turn on Settings > Beta > Subagents > Enable Subagents Directory, then ask the Agent to delegate to one by name; each `.trae/agents/<name>.md` is routable and the frontmatter parses with no BOM or delimiter error.
5. Enable the project hook under Settings > Hooks (accept the security panel). Configured Hooks lists the file, and triggering the matched tool runs the command.
6. Edit `.trae/.ignore`, then Build under Settings > Indexing & Docs. The ignored paths drop out of `#Workspace` context.
