+++
title = "Trae"
description = "How agnostic-ai emits Trae configuration: native paths, capability limits, and output options."
weight = 180

[extra]
group = "Reference"
target_id = "trae"
+++

# Trae (`trae`)

```
AGENTS.md                     # canonical entry-point pointer body (written by sync, shared path)
.trae/rules/<name>.md         # one per rule
.trae/agents/<name>.md        # one per agent (project subagent)
.trae/skills/<name>/SKILL.md  # one folder per skill
.trae/commands/<name>.md      # one per command
.trae/hooks.json              # when hook entries exist
.trae/.ignore                 # when ignore entries exist
.trae/mcp.json                # MCP server registry
```

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads persistent rules from `.trae/rules/` and the root `AGENTS.md` natively, and project subagents from `.trae/agents/`.

- **Rules**: every `.trae/rules/*.md` file carries `description` / `globs` / `alwaysApply` YAML frontmatter, the same three-field activation matrix Cursor's `.mdc` rules use.
  - `alwaysApply` defaults to `true`. A `true` rule omits `globs` entirely. `alwaysApply: false` with no explicit `globs` falls back to the Claude spelling (`paths`, comma-joined).
  - The Trae docs document all three keys but not what a file carrying none of them defaults to, so this adapter always emits them rather than leave the activation mode to guesswork (#607).
  - Set `x-trae.scene: git_message` on a rule to mark it for AI-generated Git commit messages. The vendor states the field "is compatible with existing fields such as alwaysApply, description, and globs", so it merges onto the same block instead of gating behind it (#635).
- **Agents**: one project subagent per agent at `.trae/agents/<name>.md`, the path [Trae's subagents doc](https://docs.trae.ai/ide/subagents) tables as `{project_folder}/.trae/agents/{my_agent}.md`.
  - Frontmatter carries `name` and `description` (both required), plus optional `model`, `tools`, `disallowedTools`, and `mcpServers`. The body after the closing delimiter is the system prompt.
  - Agents used to flatten into `.trae/rules/agent-<name>.md`, which reached the rules loader instead of the subagent loader and dropped every field rule frontmatter has no key for (target-audit 2026-08-27, #638). A managed copy at the old name is swept for every current agent.
  - `tools` passes through with no translation table: Trae's vocabulary is Claude-style and covers agnostic-ai's set exactly (`Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write`, `WebFetch`, `WebSearch`, plus `Skill`, `LSP`, `TodoWrite`, and `mcp__<server>__<tool>`). It is comma-joined into the string spelling the vendor documents rather than a YAML list.
  - `model` is the opposite case: Trae accepts "Only built-in models provided by TraeCode", a table of its own IDs (`gpt-5.4`, `minimax-m3`, ...) with no overlap with a cross-target `model:` value, so a bare generic model drops with a coverage note. Name one for Trae with `model: {trae: <id>}` or `x-trae.model`. `disallowedTools` and `mcpServers` reach the file through `x-trae` the same way. `x-trae.disallowedTools` is a denylist that wins over `tools` and is comma-joined like it.
  - Subagents sit behind a switch: "Go to Settings > Beta > Subagents, ensure that the Enable Subagents Directory switch is toggled on." The vendor never states its default, so a project may need that switch flipped before the emitted files load.
- **Skills**: one folder per skill under `.trae/skills/<name>/SKILL.md`, [Trae's native skills layout](https://docs.trae.ai/ide/skills). The SKILL.md frontmatter carries `name` + `description`. Sibling assets (`examples/`, `templates/`, `resources/`) copy byte-for-byte alongside it. A flat file directly under `.trae/rules/` never loads as a skill, so this is a folder, not a rule-form file.
- **Commands**: one `.md` per command under `.trae/commands/<name>.md`, `name` + `description` frontmatter and the body as the prompt.
  - This shape is vendor-confirmed rather than reverse-engineered: [Trae's slash-commands doc](https://docs.trae.ai/ide/slash-commands) tables the same two frontmatter fields, `Name` and `Description`, matching what this adapter already emitted from real `.trae/commands/*.md` files. Only `name` and `description` are documented, so nothing else emits.
  - Nesting under `.trae/commands/` is bounded at 3 levels, not merely organizational: the vendor's own example tree marks a file 3 levels deep as the deepest readable one and a file 4 levels deep as exceeding the limit, so this adapter still writes every command flat rather than risk crossing it.
- **MCP servers**: merge into `.trae/mcp.json` under a root `mcpServers` map, [confirmed by a direct fetch of Trae's own MCP doc](https://docs.trae.ai/ide/add-mcp-servers).
  - Stdio entries carry `command` (required) plus optional `args` / `env`. HTTP entries carry `url` (required) plus optional `headers`. Neither carries a `type` field: Trae tells the two apart by which of `command` or `url` is present, so this adapter never writes one.
  - `disabled` has no documented per-server key either (only a project-level MCP toggle under Settings > MCP), so a spec's `disabled: true` is stripped with a coverage note instead of written dead.
  - The vendor doc cautions that a stdio `command` must not contain spaces, or parsing fails.
- **Hooks**: written to `.trae/hooks.json`, the project tier of [Trae's hook configuration reference](https://docs.trae.ai/ide/hook-configuration-reference): "Project Hook | `$PROJECT_FOLDER/.trae/hooks.json` | Applies only to the current project or workspace" (target-audit 2026-09-11, #729).
  - The file is an integer `version` envelope ("The default value is 1, and currently only 1 is supported") around a `hooks` map keyed by event, each event holding the same `{matcher, hooks: [{type, command, timeout}]}` groups Claude Code, Codex, OpenHands, and Qoder already get, so one hook spec feeds all five.
  - Trae documents [six events](https://docs.trae.ai/ide/automate-actions-with-hooks) (`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`) and three fields per entry: `type` ("currently only `command` is supported"), `command`, and `timeout` (seconds, default 30).
  - A spec's `loop_limit` also emits, Trae's own group-level field capping how often a `Stop` hook may block the agent from stopping. Leave it unset for the vendor default of 5.
  - **The matcher vocabulary is the trap here.** Trae's hook `tool_name` table is not its subagent `tools` table: it reads `Read`, `Write`, `Edit`, `Glob`, `Grep`, `LS`, `RunCommand`, `WebSearch`, `WebFetch`, `AskUserQuestion`, `Skill`, and `mcp__<serverName>__<toolName>`. The terminal tool is `RunCommand`, not `Bash`, and there is no `TodoWrite`, so a Claude-style `matcher: Bash` parses as a valid regex and then matches nothing. That case emits verbatim with a coverage note rather than a guessed rename, the same line OpenHands and Crush hold.
  - `matcher` is read on `PreToolUse`, `PostToolUse`, and `Notification` only. On `Notification` it selects a notification type (`idle_prompt`, `permission_prompt`, ...) rather than a tool.
  - Project hooks are enabled through Settings > Hooks behind a security-consent panel ("read the warning message. After confirming there is no risk, click the Enable button"), the same shape as the project-level MCP toggle this adapter already emits past, so the file is inert until a user flips it.

  Trae also reads Claude Code's hook file: "Additionally, TraeCode supports reading hook configurations from Claude Code", with `$PROJECT_FOLDER/.claude/settings.json` and `.claude/settings.local.json` in its own Project Hook table, and "If both Claude Code hooks and TraeCode hooks are enabled at the same time, TraeCode will read all enabled hook configurations and execute them in combination."

  A repo syncing `claude` and `trae` together therefore runs every hook twice, but only after you turn it on: [automate-actions-with-hooks](https://docs.trae.ai/ide/automate-actions-with-hooks) says "Toggle the **Import Hook configuration in CLAUDE** switch on", and it is off by default, behind the same security-warning panel (#756).
- **Ignore**: ignore specs emit as `.trae/.ignore`, the path Trae's own Settings > Indexing & Docs flow creates: "TraeCode automatically creates the `.ignore` file in the `.trae/` folder and opens this file in the editor" ([docs.trae.ai/ide/ignore-files](https://docs.trae.ai/ide/ignore-files), target-audit 2026-09-11, #728).
  - The filename is a bare `.ignore`, scoped by the directory rather than a tool prefix. It supplements `.gitignore`, which Trae already honors by default, and governs codebase indexing plus `#Workspace` / `#Folder` context ("any ignored files or folders will not be included as context").
  - Unlike every other ignore target, it does not apply on save: "The `.ignore` file will take effect after re-indexing is complete", so a freshly synced pattern needs a Build under Settings > Indexing & Docs before it holds.
  - Multiple specs concatenate. Override via `outputs.trae.ignore-file`.

| Key | Default |
| --- | --- |
| `outputs.trae.rules-dir` | `.trae/rules` |
| `outputs.trae.agents-dir` | `.trae/agents` |
| `outputs.trae.skills-dir` | `.trae/skills` |
| `outputs.trae.commands-dir` | `.trae/commands` |
| `outputs.trae.hooks-file` | `.trae/hooks.json` |
| `outputs.trae.ignore-file` | `.trae/.ignore` |
| `outputs.trae.mcp-file` | `.trae/mcp.json` |

Verify with the real IDE:

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
