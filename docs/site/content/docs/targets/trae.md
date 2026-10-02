+++
title = "Trae"
description = "How agnostic-ai emits Trae configuration: native paths, capability limits, and output options."
weight = 180

[extra]
group = "Reference"
target_id = "trae"
+++

# Trae (`trae`)

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads `.trae/rules/`, the root `AGENTS.md`, and project subagents in `.trae/agents/`.

## Output

```
AGENTS.md                     # pointer body, plus the rules block when an inlining target shares it
.trae/rules/<name>.md         # one per rule
.trae/agents/<name>.md        # one per agent (project subagent)
.trae/skills/<name>/SKILL.md  # one folder per skill
.trae/commands/<name>.md      # one per command
.trae/hooks.json              # when hook entries exist
.trae/.ignore                 # when ignore entries exist
.trae/mcp.json                # MCP server registry
```

Trae reads `AGENTS.md` only with **Include AGENTS.md in the context** on (Settings > Rules), so every rule keeps its `.trae/rules/` file. With that switch on and codex or another inlining target enabled, always-on rules load twice. See [target behavior](@/docs/target-behavior.md#entry-point-files).

- **Rules**: sync always writes `description`, `globs`, and `alwaysApply` (Cursor's `.mdc` activation fields), since Trae documents no default for a file without them.
  - `alwaysApply` is `true`, or `false` when the spec sets `globs` other than a catch-all such as `**/*`. A `true` rule omits `globs`.
  - `alwaysApply: false` with no `globs` falls back to the Claude `paths` list, comma-joined.
  - `x-trae.scene: git_message` marks a rule for AI-generated commit messages, alongside the other activation fields.
- **Agents** ([subagents docs](https://docs.trae.ai/ide/subagents)): frontmatter carries `name` and `description` (required), plus optional `model`, `tools`, `disallowedTools`, and `mcpServers`. The body is the system prompt.
  - `tools` passes through untranslated as a comma-joined string, since Trae uses Claude-style names (`Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write`, `WebFetch`, `WebSearch`, plus `Skill`, `LSP`, `TodoWrite`, and `mcp__<server>__<tool>`).
  - Trae accepts only its built-in model IDs (`gpt-5.4`, `minimax-m3`, ...), so a generic `model` drops with a coverage note. Use `model: {trae: <id>}` or `x-trae.model`.
  - `disallowedTools` and `mcpServers` come from `x-trae`. `x-trae.disallowedTools` is a comma-joined denylist that wins over `tools`.
  - Names start with an ASCII letter, end with a letter or digit, use only letters, digits, or hyphens, and run at most 50 characters. Sync rejects others.

  - Subagents need Settings > Beta > Subagents > Enable Subagents Directory. Trae does not state the default.
- **Skills** ([skills docs](https://docs.trae.ai/ide/skills)): `.trae/skills/<name>/SKILL.md` with `name` and `description`. Sibling assets (`examples/`, `templates/`, `resources/`) copy byte-for-byte. A flat file under `.trae/rules/` never loads as a skill.
- **Commands** ([slash-commands docs](https://docs.trae.ai/ide/slash-commands)): only `name` and `description` frontmatter, the documented fields, with the body as the prompt. Trae reads commands up to 3 levels deep; sync writes them flat.
- **MCP servers** ([MCP docs](https://docs.trae.ai/ide/add-mcp-servers)): a root `mcpServers` map. Stdio entries carry `command` (required), `args`, and `env`; HTTP entries carry `url` (required) and `headers`. Trae infers the `type` from `command` or `url`, so none is written.
  - A stdio `command` with spaces fails to parse in Trae.
  - Trae has no per-server `disabled` key, only a project toggle (Settings > MCP), so `disabled: true` is stripped with a coverage note.
- **Hooks**: `.trae/hooks.json`, the project tier of Trae's [hook configuration](https://docs.trae.ai/ide/hook-configuration-reference). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with Trae's payload, shell, and timeout before a session does.
  - A `version` field (always 1) wraps the `hooks` map. Each event holds `{matcher, hooks: [{type, command, timeout}]}` groups, the shape Claude Code, Codex, OpenHands, and Qoder share, so one hook spec feeds all five.
  - Six [events](https://docs.trae.ai/ide/automate-actions-with-hooks): `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`. Each entry takes `type` (only `command`), `command`, and `timeout` (seconds, default 30).
  - `loop_limit` caps how often a `Stop` hook may block the agent (default 5).
  - **Hook `tool_name` values differ from subagent `tools`**: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `LS`, `RunCommand`, `WebSearch`, `WebFetch`, `AskUserQuestion`, `Skill`, and `mcp__<serverName>__<toolName>`. There is no `TodoWrite`, and the terminal is `RunCommand`, so `matcher: Bash` emits verbatim with a coverage note and matches nothing.
  - `matcher` applies to `PreToolUse`, `PostToolUse`, and `Notification` only. On `Notification` it selects a type (`idle_prompt`, `permission_prompt`, ...).
  - Project hooks stay inert until you enable them under Settings > Hooks and accept the security warning.
  - Trae also runs Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json` once you turn on **Import Hook configuration in CLAUDE** (off by default, same warning). Syncing `claude` and `trae` then runs every hook twice.
- **Ignore** ([ignore docs](https://docs.trae.ai/ide/ignore-files)): the file Trae's Settings > Indexing & Docs creates. It supplements `.gitignore`, which Trae already honors, and excludes paths from indexing and `#Workspace` / `#Folder` context. Specs concatenate. Override via `outputs.trae.ignore-file`. Run a Build under Settings > Indexing & Docs after a sync, since it applies only after re-indexing.

{% <details summary="Agents from older versions"> %}
Sync sweeps a managed copy at the old `.trae/rules/agent-<name>.md` path for every current agent.
{% </details> %}

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

## Import

`agnostic-ai import trae` reverses the Trae layout:

| Source | Becomes |
|--------|---------|
| `.trae/rules/*.md` | rules, agents, and skills by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification) |
| `.trae/agents/<name>.md` | `<agents>/<name>.md`, byte-for-byte minus the provenance header |
| `.trae/skills/<name>/SKILL.md` (+ bundled assets) | `<skills>/<name>/SKILL.md` (folder copied byte-for-byte) |
| `.trae/commands/<name>.md` | `<commands>/<name>.md` |
| `.trae/hooks.json` | one hook spec per matcher group, carrying `loop_limit` |
| `.trae/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml`, a `url`-only entry inferring `type: http` |
| `.trae/.ignore` | an ignore spec |

Hook commands sharing an event and matcher collapse into one spec with a `command:` list, as sync merges them into one group. Import skips `version` (always 1), a `loop_limit` of 0 (Trae treats it as unset), and the global `~/.trae/hooks.json`.

## Protected paths

Advisory. This target takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [Protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

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
4. Turn on Settings > Beta > Subagents > Enable Subagents Directory and ask the Agent to delegate by name. Each `.trae/agents/<name>.md` is routable, with no BOM or delimiter error.
5. Enable the project hook under Settings > Hooks (accept the security panel). Configured Hooks lists the file, and triggering the matched tool runs the command.
6. Edit `.trae/.ignore`, then Build under Settings > Indexing & Docs. The ignored paths drop out of `#Workspace` context.
