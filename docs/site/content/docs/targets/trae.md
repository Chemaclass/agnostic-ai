+++
title = "Trae"
description = "What agnostic-ai writes for Trae: file paths, what Trae cannot do, and output options."
weight = 180

[extra]
group = "Reference"
target_id = "trae"

[extra.moved]
set-up-this-tool = "@/docs/targets/trae.md#first-rule"
+++

# Trae (`trae`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

ByteDance [Trae](https://docs.trae.ai/ide/rules) reads `.trae/rules/`, the root `AGENTS.md`, and project subagents in `.trae/agents/`.

## First rule

Install the client using the [official setup guide](https://www.trae.ai), then open your project in it.

Select `trae` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.trae/rules/conventional-commits.md`.

Project hooks and subagent directories require separate switches in Trae settings. Syncing their files does not enable those features.

For existing files, check [what `import trae` reads](#import) before syncing.

## Verify

Run `agnostic-ai sync --check` and inspect `.trae/rules/conventional-commits.md` for the commit-prefix rule. Open the project in Trae. Confirm the rule is active in the Rules panel, then ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                     # pointer body, plus the rules block when another tool that inlines rules shares it
.trae/rules/<name>.md         # one per rule
.trae/agents/<name>.md        # one per agent (project subagent)
.trae/skills/<name>/SKILL.md  # one folder per skill
.trae/commands/<name>.md      # one per command
.trae/hooks.json              # when hook entries exist
.trae/.ignore                 # when ignore entries exist
.trae/mcp.json                # MCP server registry
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

Trae reads `AGENTS.md` only when **Include AGENTS.md in the context** is on (Settings > Rules & Memories). So every rule keeps its `.trae/rules/` file. With that switch on and codex or another tool that includes rules in instruction files enabled, always-on rules load twice. See [target behavior](@/docs/target-behavior.md#entry-point-files).

### Rules

Sync always writes `description`, `globs`, and `alwaysApply` (Cursor's `.mdc` fields for when a rule applies), because Trae documents no default for a file without them.

- `alwaysApply` is `true`, or `false` when the spec sets `globs` other than a catch-all such as `**/*`. A `true` rule omits `globs`.
- `alwaysApply: false` with no `globs` uses the Claude `paths` list, joined with commas.
- `x-trae.scene: git_message` marks a rule for AI-written commit messages.

### Agents

([subagents docs](https://docs.trae.ai/ide/subagents)): frontmatter carries `name` and `description` (required), plus optional `model`, `tools`, `disallowedTools`, and `mcpServers`. The body is the system prompt.

- `tools` is written as is, as a string joined with commas, because Trae uses Claude-style names (`Bash`, `Edit`, `Glob`, `Grep`, `Read`, `Write`, `WebFetch`, `WebSearch`, plus `Skill`, `LSP`, `TodoWrite`, and `mcp__<server>__<tool>`).
- Omit `tools` to load every available tool. An explicitly empty tool list (`can: []`, `tools: []`, `tools: ""`, or `x-trae.tools`) writes `tools: ""`, which disables all tools. Import preserves this restriction through later syncs.
- Trae accepts only its built-in model IDs (`gpt-5.4`, `minimax-m3`, ...). A generic `model` drops with a sync note. Use `model: {trae: <id>}` or `x-trae.model`.
- `disallowedTools` and `mcpServers` come from `x-trae`. `x-trae.disallowedTools` is a comma-separated list of blocked tools, and it wins over `tools`.
- Names start with an ASCII letter, end with a letter or digit, use only letters, digits, or hyphens, and run at most 50 characters. Sync rejects any other name.
- Subagents need Settings > Beta > Subagents > Enable Subagents Directory. Trae does not say whether it is on by default.

### Skills

([skills docs](https://docs.trae.ai/ide/skills)): `.trae/skills/<name>/SKILL.md` with `name` and `description`. Files next to the skill (`examples/`, `templates/`, `resources/`) are copied as they are. A flat file under `.trae/rules/` never loads as a skill.

### Commands

([slash-commands docs](https://docs.trae.ai/ide/slash-commands)): only `name` and `description` frontmatter (the documented fields), with the body as the prompt. Trae reads commands up to 3 levels deep, and sync writes them flat.

### MCP servers

([MCP docs](https://docs.trae.ai/ide/add-mcp-servers)): a root `mcpServers` map. Stdio entries carry `command` (required), `args`, and `env`. HTTP entries carry `url` (required) and `headers`. Trae works out the `type` from `command` or `url`, so none is written.

- A stdio `command` with spaces fails to parse in Trae.
- Trae has no per-server `disabled` key, only a project toggle (Settings > MCP). `disabled: true` is dropped with a sync note.

### Hooks

`.trae/hooks.json`, the project tier of Trae's [hook configuration](https://docs.trae.ai/ide/hook-configuration-reference). [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests them before a session, using Trae's event data, shell, and timeout.

- A `version` field (always 1) wraps the `hooks` map. Each event holds `{matcher, hooks: [{type, command, timeout}]}` groups. Claude Code, Codex, OpenHands, and Qoder use the same shape, so one hook spec feeds all five.
- Six [events](https://docs.trae.ai/ide/automate-actions-with-hooks): `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `Notification`. Each entry takes `type` (only `command`), `command`, and `timeout` (seconds, default 30).
- `loop_limit` caps how often a `Stop` hook can block the agent (default 5).
- **Hook `tool_name` values differ from subagent `tools`**: `Read`, `Write`, `Edit`, `Glob`, `Grep`, `LS`, `RunCommand`, `WebSearch`, `WebFetch`, `AskUserQuestion`, `Skill`, and `mcp__<serverName>__<toolName>`. There is no `TodoWrite`. The terminal tool is `RunCommand`, so `matcher: Bash` is written as is, raises a sync note, and matches nothing.
- `matcher` applies to `PreToolUse`, `PostToolUse`, and `Notification` only. On `Notification` it selects a type (`idle_prompt`, `permission_prompt`, ...).
- Project hooks do nothing until you enable them under Settings > Hooks and accept the security warning.
- Trae also runs Claude Code's hooks from `.claude/settings.json` and `.claude/settings.local.json` once you turn on **Import Hook configuration in CLAUDE** (off by default, same warning). Syncing `claude` and `trae` then runs every hook twice.

### Ignore

([ignore docs](https://docs.trae.ai/ide/ignore-files)): the file Trae's Settings > Indexing & Docs creates. It adds to `.gitignore`, which Trae already honors, and keeps paths out of indexing and `@Folder` context. Specs are joined. Override with `outputs.trae.ignore-file`. Run a Build under Settings > Indexing & Docs after a sync, because the change applies only after re-indexing.

## Shared memory

With `builtins: [memory]`, sync adds the native `memory-trae-session-start` hook for `SessionStart` to `.trae/hooks.json`. Once you enable project hooks in Settings > Hooks, it runs `agnostic-ai project -- hook memory --target trae` before the first message and adds up to 6,000 bytes from the personal and project memory indexes. Trae's own Memories stay available.

Trae sets `TRAE_PROJECT_DIR` to the workspace used by the hook, so the command finds that project's indexes even when its shell starts elsewhere. The command uses the project's installed agnostic-ai package when one is declared; install or upgrade that package before starting Trae. If hooks are disabled, the shared memory rule still tells Trae to read the indexes.

<details>
<summary>Agents from older versions</summary>

For every current agent, sync removes a generated copy at the old `.trae/rules/agent-<name>.md` path.

</details>

</details>

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

`agnostic-ai import trae` reads these back:

| Source | Becomes |
|--------|---------|
| `.trae/rules/*.md` | rules, agents, and skills by [filename prefix](@/docs/cli-reference/start.md#filename-prefix-reclassification) |
| `.trae/agents/<name>.md` | `<agents>/<name>.md`, copied, minus the generated header |
| `.trae/skills/<name>/SKILL.md` and `.agents/skills/<name>/SKILL.md` (+ bundled files) | `<skills>/<name>/SKILL.md` (folder copied as is) |
| `.trae/commands/<name>.md` | `<commands>/<name>.md` |
| `.trae/hooks.json` | one hook spec per matcher group, carrying `loop_limit` |
| `.trae/mcp.json` (`mcpServers.<name>`) | `<mcps>/<name>.yaml`, a `url`-only entry becomes `type: http` |
| `.trae/.ignore` | an ignore spec |

Import reads `.trae/skills/` first, then `.agents/skills/`. When both hold a skill with the same folder name, Trae's own copy wins. Shared skills keep the same checks for existing specs and generated assets. Sync still writes `.trae/skills/` by default.

Hook commands that share an event and matcher become one spec with a `command:` list, as sync merges them into one group. Import skips `version` (always 1), a `loop_limit` of 0 (Trae treats it as unset), and the global `~/.trae/hooks.json`.

## Protected paths

Not enforced. Trae takes no settings specs, so sync reports a spec with a `protected` block as unsupported. See [protected-path alternatives](@/docs/spec-format/settings.md#protected-paths).
