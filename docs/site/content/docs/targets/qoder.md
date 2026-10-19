+++
title = "Qoder"
description = "What agnostic-ai writes for Qoder: file paths, what Qoder supports, and config options."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"

[extra.moved]
set-up-this-tool = "@/docs/targets/qoder.md#first-rule"
+++

# Qoder (`qoder`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

agnostic-ai writes Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) rules, agents, skills, and commands under `.qoder/`, and merges MCP, hooks, and settings into `.qoder/settings.json`.

## First rule

Install the client using the [official setup guide](https://qoder.com), then open your project in it.

Select `qoder` in `agnostic-ai.yaml`, then follow [First rule](@/docs/getting-started.md#first-rule) to add the shared commit-prefix rule and sync it. With the default paths, it reaches `.qoder/rules/conventional-commits.md`, or shared `AGENTS.md` when another selected target already puts the rule there.

When another target includes the rule in `AGENTS.md`, Qoder uses that copy. Keep `AGENTS.md` in the CLI context file list.

For existing files, check [what `import qoder` reads](#import) before syncing.

With `handoff` or `handoff-hook`, linked worktrees get shared handoff write access through `permissions.additionalDirectories` in the ignored `.qoder/settings.json`. See [shared handoff access](@/docs/handoff.md#write-from-a-linked-worktree) for preserved configuration and fallback behavior.

## Verify

Run `agnostic-ai sync --check` and find the commit-prefix rule in `.qoder/rules/conventional-commits.md` or the shared `AGENTS.md`. Open the project in Qoder. Confirm the rule is active, then ask for a commit subject for a small fix; it should use `fix:`.

Next, [add a skill](@/docs/spec-format/skills.md).

## Output

```
AGENTS.md                        # entry-point pointer body, plus the rules block when another tool adds it
.qoder/rules/<name>.md           # one per rule that AGENTS.md does not carry, or that is not always-on
.qoder/agents/<name>.md          # one per agent
.qoder/skills/<name>/SKILL.md    # one folder per skill, plus bundled assets
.qoder/commands/<name>.md        # one per command
.qoder/settings.json             # MCP, hooks, and settings (merged; other keys are kept)
```

## Native mappings

<details>
<summary>Field mappings, discovery, and compatibility</summary>

### Rules

Qoder reads `.qoder/rules/` and the root `AGENTS.md`. Per-rule files win a conflict, so rules go there instead of including them in that file. When Codex or another tool that copies rules into `AGENTS.md` adds the `## Rules` block, an always-on rule gets no file and loads once. A rule whose text differs for Qoder keeps its file. The Qoder CLI reads `AGENTS.md` unless `context.fileName` names other files. Keep it listed there, or the CLI misses the rules that have no file. See [Rule activation](#rule-activation) and [target behavior](@/docs/target-behavior.md#entry-point-files).

### Agents

[Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`. `name` and `description` are required. `model`, `tools`, `color`, `skills`, `mcpServers`, `effort`, `permissionMode`, `memory`, `hooks`, `maxTurns`, and `background` are optional. `effort` takes a single value or a per-tool map, and an integer budget stays a number. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort) and [agent policy support by target](@/docs/spec-format/agents.md#agent-policy-support-by-target).

- `color` is one of `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([CLI field reference](https://docs.qoder.com/cli/subagent)).
- `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`), which `import qoder` splits back. Qoder uses Claude-style names (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), so the portable list is copied unchanged.
- `skills` and `mcpServers` come from `x-qoder`. `memory` is copied unchanged. See [Subagent memory](#subagent-memory).
- Sync sets `name`, `description`, `model`, `tools`, `skills`, and `mcpServers`. Any other `x-qoder` key is copied as written.

### Skills

Qoder's own `.qoder/skills/<name>/SKILL.md` ([Qoder Skills](https://docs.qoder.com/extensions/skills)), with plain `name` and `description` frontmatter. Bundled files (scripts, references, templates) are copied unchanged next to `SKILL.md`. Qoder does not list `.agents/skills/` as compatible, so skills are not shared with that folder. Sync does not write your personal `~/.qoder/skills/`.

### Commands

`.qoder/commands/<name>.md`, the project path for the [CLI](https://docs.qoder.com/cli/commands) and the [IDE](https://docs.qoder.com/user-guide/commands). Sync always writes `description` (default: the command's name) and never `name`, because the file path sets it. On a name clash, the CLI prefers your personal command (`~/.qoder/commands/`), and the IDE lists both with a scope label. Sync cannot see your personal commands, so it cannot warn.

### MCP

`mcpServers` is merged into `.qoder/settings.json` (stdio: `command`/`args`/`env`/`cwd`, no `type`; remote: `type` plus `url`/`headers`). Other keys such as `mcp.enableAllProjectMcpServers`, permissions, and custom models are kept.

- Entries accept these [optional fields](https://docs.qoder.com/cli/mcp-reference): `timeout` (milliseconds), `description`, `trust` (skip tool-call confirmation), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` (keep the config, turn the server off), and an `oauth` object, written as you declare it. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
- A `type: ws` spec gets only a sync note. Qoder's ws transport takes a TCP host and port that a spec cannot express.
- The file may be JSONC ([settings](https://docs.qoder.com/cli/settings)). `sync` keeps the keys but drops comments and trailing commas, and warns.

### Settings

The shared default model becomes `model.name`. `permissions.allow`, `permissions.deny`, and `permissions.ask` are copied directly. An `x-qoder` settings block is merged in too.

- With `memory.personal: repo`, sync adds the personal store to `permissions.additionalDirectories`, so Qoder approves saves there. Your own entries stay. See [repo mode](@/docs/memory.md#one-store-per-repository).

### Hooks

Hooks are merged under `hooks` in the same file ([Qoder hooks](https://docs.qoder.com/cli/hooks)), in the nested form that Claude Code, Codex, and OpenHands share. With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session. Its reply is `{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": ...}}`, the form Qoder documents.

- 27 PascalCase events ([hooks reference](https://docs.qoder.com/cli/hooks-reference)): `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`. `event:` is copied as written. Listed events are sorted in Qoder's order, ahead of unlisted ones.
- `PreToolUse`/`PostToolUse` matchers use Claude Code's tool names (`Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`, `mcp__server__tool`), so matchers written for Claude Code work unchanged.
- Command entries have `command`, `type: command`, optional `args`, `timeout` (seconds, default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), `if` (a permission rule filter, for example `Bash(git *)`), and `once`.
- `args` switches to the form without a shell: `command` is one program and each `args` element is one literal argument. Qoder then ignores `shell`. Sync still writes both and adds a note.
- `once` has no effect, and `sync` says so. Qoder honors it only for session-scoped hooks ([Hooks](https://docs.qoder.com/cli/hooks), [Subagent](https://docs.qoder.com/cli/subagent)), and portable hooks go in settings.
- HTTP handlers have `url`, optional `headers`, and `allowedEnvVars`. Prompt handlers have `prompt` and optional `model`. Both keep filters, timeouts in seconds, and matcher groups.
- Other handler types get a sync note. Sync writes only one `env` entry, `AGNOSTIC_AI_TARGET`, and no `rewakeMessage`, `rewakeSummary`, or agent handlers.

<details>
<summary>Unmanaged AGENTS.md</summary>

Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. Check [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).

</details>

<details>
<summary>Leftover root .mcp.json</summary>

Older versions wrote MCP servers to the root `.mcp.json` shared with Claude Code. The two tools' per-server fields differed, so Qoder moved to `.qoder/settings.json`, which Claude Code never reads. A leftover `.mcp.json` still loads and wins over `.qoder/settings.json` for a server with the same name. Sync cannot remove it, because it has no generated-file header and may belong to Claude Code. Delete it by hand in a Qoder-only project.

</details>

</details>

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

Auto memory is off by default; see [each tool's own memory](@/docs/memory.md#each-tools-own-memory). agnostic-ai never writes that store. To copy its facts into shared personal memory, run `agnostic-ai memory import --path ~/.qoder/projects/<project>/memory`.

## Import

`import qoder` reads rules, agents, skill folders, commands, MCP servers, and portable settings fields. It reads `mcpServers` from project `.qoder/settings.json`, including JSONC, into the configured MCP source directory. Environment and header credentials become environment references. A server with a literal OAuth credential is left out, and import names the field to fix. Portable settings go to `settings/qoder.yaml`. Rule activation is kept.

Hooks, personal settings, and the separate project `.mcp.json` are not imported.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a [protected-path coverage note](@/docs/spec-format/settings.md#protected-paths).
