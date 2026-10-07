+++
title = "Qoder"
description = "What agnostic-ai writes for Qoder: file paths, what Qoder supports, and config options."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"
+++

# Qoder (`qoder`)

agnostic-ai writes Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) rules, agents, skills, and commands under `.qoder/`, and merges MCP, hooks, and settings into `.qoder/settings.json`.

## Output

```
AGENTS.md                        # entry-point pointer body, plus the rules block when another tool adds it
.qoder/rules/<name>.md           # one per rule that AGENTS.md does not carry, or that is not always-on
.qoder/agents/<name>.md          # one per agent
.qoder/skills/<name>/SKILL.md    # one folder per skill, plus bundled assets
.qoder/commands/<name>.md        # one per command
.qoder/settings.json             # MCP, hooks, and settings (merged; other keys are kept)
```

- **Rules**: Qoder reads `.qoder/rules/` and the root `AGENTS.md`. Per-rule files win a conflict, so rules go there instead of inline. When Codex or another tool that copies rules into `AGENTS.md` adds the `## Rules` block, an always-on rule gets no file and loads once. A rule whose text differs for Qoder keeps its file. The Qoder CLI reads `AGENTS.md` unless `context.fileName` names other files. Keep it listed there, or the CLI misses the rules that have no file. See [Rule activation](#rule-activation) and [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Agents**: [Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`. `name` and `description` are required. `model`, `tools`, `color`, `skills`, `mcpServers`, `effort`, `permissionMode`, `memory`, and `hooks` are optional. `effort` takes a single value or a per-tool map, and an integer budget stays a number. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort) and [agent policy support by target](@/docs/spec-format/agents.md#agent-policy-support-by-target).
  - `color` is one of `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([CLI field reference](https://docs.qoder.com/cli/subagent)).
  - `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`), which `import qoder` splits back. Qoder uses Claude-style names (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), so the portable list is copied unchanged.
  - `skills` and `mcpServers` come from `x-qoder`. `memory` is copied unchanged. See [Subagent memory](#subagent-memory).
  - Sync sets `name`, `description`, `model`, `tools`, `skills`, and `mcpServers`. Any other `x-qoder` key is copied as written.
- **Skills**: Qoder's own `.qoder/skills/<name>/SKILL.md` ([Qoder Skills](https://docs.qoder.com/extensions/skills)), with plain `name` and `description` frontmatter. Bundled files (scripts, references, templates) are copied unchanged next to `SKILL.md`. Qoder does not list `.agents/skills/` as compatible, so skills are not shared with that folder. Sync does not write your personal `~/.qoder/skills/`.
- **Commands**: `.qoder/commands/<name>.md`, the project path for the [CLI](https://docs.qoder.com/cli/commands) and the [IDE](https://docs.qoder.com/user-guide/commands). Sync always writes `description` (default: the command's name) and never `name`, because the file path sets it. On a name clash, the CLI prefers your personal command (`~/.qoder/commands/`), and the IDE lists both with a scope label. Sync cannot see your personal commands, so it cannot warn.
- **MCP**: `mcpServers` is merged into `.qoder/settings.json` (stdio: `command`/`args`/`env`/`cwd`, no `type`; remote: `type` plus `url`/`headers`). Other keys such as `mcp.enableAllProjectMcpServers`, permissions, and custom models are kept.
  - Entries accept these [optional fields](https://docs.qoder.com/cli/mcp-reference): `timeout` (milliseconds), `description`, `trust` (skip tool-call confirmation), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` (keep the config, turn the server off), and an `oauth` object, written as you declare it. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
  - A `type: ws` spec gets only a coverage note. Qoder's ws transport takes a TCP host and port that a spec cannot express.
  - The file may be JSONC ([settings](https://docs.qoder.com/cli/settings)). `sync` keeps the keys but drops comments and trailing commas, and warns.
- **Settings**: the shared default model becomes `model.name`. `permissions.allow`, `permissions.deny`, and `permissions.ask` are copied directly. An `x-qoder` settings block is merged in too.
- **Hooks**: hooks are merged under `hooks` in the same file ([Qoder hooks](https://docs.qoder.com/cli/hooks)), in the nested form that Claude Code, Codex, and OpenHands share. With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session.
  - 27 PascalCase events ([hooks reference](https://docs.qoder.com/cli/hooks-reference)): `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`. `event:` is copied as written. Listed events are sorted in Qoder's order, ahead of unlisted ones.
  - `PreToolUse`/`PostToolUse` matchers use Claude Code's tool names (`Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`, `mcp__server__tool`), so matchers written for Claude Code work unchanged.
  - Command entries have `command`, `type: command`, optional `args`, `timeout` (seconds, default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), `if` (a permission rule filter, for example `Bash(git *)`), and `once`.
  - `args` switches to the form without a shell: `command` is one program and each `args` element is one literal argument. Qoder then ignores `shell`. Sync still writes both and adds a note.
  - `once` has no effect, and `sync` says so. Qoder honors it only for session-scoped hooks ([Hooks](https://docs.qoder.com/cli/hooks), [Subagent](https://docs.qoder.com/cli/subagent)), and portable hooks go in settings.
  - HTTP handlers have `url`, optional `headers`, and `allowedEnvVars`. Prompt handlers have `prompt` and optional `model`. Both keep filters, timeouts in seconds, and matcher groups.
  - Other handler types get a coverage note. Sync does not write Qoder's `env`, `rewakeMessage`, `rewakeSummary`, or agent handlers.

{% <details summary="Unmanaged AGENTS.md"> %}
Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. But sync then stops writing `AGENTS.md`, so other tools that read it, such as Codex, miss rule changes.
{% </details> %}

{% <details summary="Leftover root .mcp.json"> %}
Older versions wrote MCP servers to the root `.mcp.json` shared with Claude Code. The two tools' per-server fields differed, so Qoder moved to `.qoder/settings.json`, which Claude Code never reads. A leftover `.mcp.json` still loads and wins over `.qoder/settings.json` for a server with the same name. Sync cannot remove it, because it has no generated-file header and may belong to Claude Code. Delete it by hand in a Qoder-only project.
{% </details> %}

## Config keys

| Key | Default |
| --- | --- |
| `outputs.qoder.rules-dir` | `.qoder/rules` |
| `outputs.qoder.agents-dir` | `.qoder/agents` |
| `outputs.qoder.skills-dir` | `.qoder/skills` |
| `outputs.qoder.commands-dir` | `.qoder/commands` |
| `outputs.qoder.mcp-file` | `.qoder/settings.json` (also the hooks path) |

## Rule activation

Qoder rules keep `description`, `alwaysApply`, `trigger`, `glob`, and `paths` frontmatter. Native `trigger`, `glob`, and `paths` import under `x-qoder`. `description` and `alwaysApply` use the rule's own fields. A single selector stays single and a list stays a list. Native fields under `x-qoder` override the top-level fields.

[Qoder's rule activation modes](https://docs.qoder.com/cli/memory#rule-activation-methods):

| Mode | Frontmatter |
| --- | --- |
| Always active | No activation fields, `trigger: always_on`, or `alwaysApply: true` |
| Manual | `trigger: manual` or `alwaysApply: false` |
| Model-selected | `trigger: model_decision` plus a non-empty `description` |
| File matching | `trigger: glob` plus `glob`, or `paths` |

`trigger` wins over `alwaysApply`. A scoped rule gets `paths` covering the scope folder and the file patterns together, with no other activation fields. Validation rejects selectors that sync cannot keep.

For a manual release rule:

```markdown
---
name: release
x-qoder:
  trigger: manual
---

Run release steps only on request.
```

## Subagent memory

The portable `memory` field is copied unchanged to `.qoder/agents/<name>.md`. Qoder's [subagent reference](https://docs.qoder.com/cli/subagent) accepts the same scopes: `user`, `project`, `local`. Only the Markdown file has it. The `--agents` JSON format has no `memory`.

It works only when top-level `autoMemoryEnabled` is on in [settings](https://docs.qoder.com/cli/settings-reference) (default `false`). Qoder documents this, but it is not tested at runtime. Only Claude Code is confirmed to act on `memory`.

## Auto memory

Qoder's own memory store is separate from the field above ([Qoder memory](https://docs.qoder.com/cli/memory)). It lives at `~/.qoder/projects/<project>/memory/` (project) and `~/.qoder/memory/` (user). Each is a `MEMORY.md` index plus one file per topic. `/memory` shows them. `/memory manage` views, edits, or deletes a topic. A session loads the first 200 lines or about 25KB of each active `MEMORY.md`.

Auto memory is off by default. agnostic-ai never reads or writes that store. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

## Import

`import qoder` reads rules, agents, skill folders, commands, and portable settings fields, which go to `settings/qoder.yaml`. Rule activation is kept. Hooks and MCP servers in `.qoder/settings.json` are not read.

## Protected paths

Advisory. Sync has no native edit guard to write for this tool, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Qoder from [qoder.com](https://qoder.com).
2. Check the tree:
   - `ls AGENTS.md .qoder/rules/ .qoder/agents/ .qoder/skills/ .qoder/commands/ .qoder/settings.json`
   - `grep "Generated by agnostic-ai" .qoder/rules/*.md .qoder/agents/*.md .qoder/skills/*/SKILL.md .qoder/commands/*.md` for the generated-file header
   - `python -m json.tool .qoder/settings.json > /dev/null`
3. Open the project:
   - The rules panel lists every `.qoder/rules/*.md` with no parse warnings, and the agent picker every `.qoder/agents/*.md`.
   - Each `.qoder/skills/<name>/` loads as a skill, and each `.qoder/commands/<name>.md` runs from `/`.
   - The MCP picker shows each `mcpServers.<name>` connected (project servers need approval on first use).
   - A hook fires on its event (for example a `PreToolUse` hook prints or blocks first).
