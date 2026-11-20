+++
title = "Qoder native reference"
description = "Native field mappings, output keys, and import coverage for qoder."
weight = 191

[extra]
nav_title = "Qoder reference"
group = "Reference"
+++

# Qoder native reference

Start with the [Qoder setup](@/docs/targets/qoder.md).

## Native mappings

### Rules

- Qoder reads `.qoder/rules/` and root `AGENTS.md`. The IDE lets per-rule files win, so sync writes rules there. The CLI loads both, in no documented order.
- When another tool adds the `## Rules` block to `AGENTS.md`, an always-on rule gets no file and loads once. A rule whose text differs for Qoder keeps its file.
- The CLI reads `AGENTS.md` unless `context.fileName` names other files. Keep it listed, or the CLI misses rules that have no file.

See [Rule activation](#rule-activation) and [target behavior](@/docs/target-behavior.md#entry-point-files).

### Agents

[Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`.

- Required: `name`, `description`. Optional: `model`, `tools`, `color`, `skills`, `mcpServers`, `effort`, `permissionMode`, `memory`, `hooks`, `maxTurns`, `background`.
- An [agent-only MCP server](@/docs/spec-format/mcps.md#agent-only) goes in full under `mcpServers`, not into `.qoder/settings.json`.
- `effort` takes one value or a per-tool map; an integer budget stays a number. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort) and [agent policy support by target](@/docs/spec-format/agents-reference.md#agent-policy-support-by-target).
- `color` is one of `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([CLI field reference](https://docs.qoder.com/cli/subagent)).
- `tools` is a comma-separated string (`tools: Read, Grep, Bash`); `import qoder` splits it back. Names are Claude-style (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), so the portable list is copied unchanged.
- `skills` and `mcpServers` come from `x-qoder`. `memory` is copied unchanged ([Subagent memory](#subagent-memory)).
- Sync sets `name`, `description`, `model`, `tools`, `skills`, `mcpServers`. Other `x-qoder` keys are copied as written.

### Skills

- Path: `.qoder/skills/<name>/SKILL.md` ([Qoder Skills](https://docs.qoder.com/extensions/skills)), with plain `name` and `description` frontmatter.
- Bundled files (scripts, references, templates) are copied unchanged beside it.
- `sync --global` writes personal skills under `~/.qoder/skills/`.
- [Qoder CLI 1.1.11](https://docs.qoder.com/release-notes/qoder-cli#1111-2026-08-01) (August 1, 2026) turned on `.agents/skills/` by default. Sync keeps the native path the IDE guide documents and does not use the shared skills folder.

### Commands

- Path: `.qoder/commands/<name>.md`, for the [CLI](https://docs.qoder.com/cli/commands) and the [IDE](https://docs.qoder.com/user-guide/commands).
- Sync always writes `description` (default: the command's name) and never `name`, since the path sets it.
- On a name clash the CLI prefers your personal command (`~/.qoder/commands/`); the IDE lists both with a scope label. Sync cannot see personal commands, so it cannot warn.

### MCP

`mcpServers` is merged into `.qoder/settings.json`.

- stdio: `command`, `args`, `env`, `cwd`, no `type`. Remote: `type`, `url`, `headers`.
- Other keys (`mcp.enableAllProjectMcpServers`, permissions, custom models) are kept.
- Optional fields ([MCP reference](https://docs.qoder.com/cli/mcp-reference)), written as declared: `timeout` (ms), `description`, `trust` (skip confirmation), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` (keep config, server off), `oauth`. See [`disabled` support by target](@/docs/spec-format/mcps-reference.md#disabled-support-by-target).
- Set `x-qoder.protocolVersion` on an MCP spec to pin Qoder's protocol revision. Project and global imports store the revision there. Sync also reads the top-level field written by older imports.
- `type: ws` gets only a sync note: Qoder's ws transport takes a TCP host and port that a spec cannot express.
- The file may be JSONC ([settings](https://docs.qoder.com/cli/settings)). Sync keeps keys, drops comments and trailing commas, and warns.

### Settings

- The shared default model becomes `model.name`.
- `permissions.allow`, `deny`, and `ask` are copied directly. An `x-qoder` settings block is merged in. Voice has no native key.
- `approvals` set to `review` for Qoder makes `sync --global` write `general.defaultPermissionMode: "auto"` to `~/.qoder/settings.json`, unless a settings spec sets `x-qoder.general.defaultPermissionMode`. See [review through the tool's own reviewer](@/docs/global-configuration-reference.md#global-approvals-review).
- With `memory.personal: repo`, sync adds the personal store to `permissions.additionalDirectories`, so saves there are approved. Your entries stay. See [repo mode](@/docs/memory.md#one-store-per-repository).

### Hooks

Hooks merge under `hooks` in the same file ([Qoder hooks](https://docs.qoder.com/cli/hooks)), in the nested form Claude Code, Codex, and OpenHands share.

- With `builtins: [memory]`, a `SessionStart` hook adds the [shared memory](@/docs/memory.md) index to the session. Its reply is `{"hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": ...}}`, the form Qoder documents.
- 27 PascalCase events ([hooks reference](https://docs.qoder.com/cli/hooks-reference)): `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`.
- `event:` is copied as written. Listed events sort in Qoder's order, before unlisted ones.
- `PreToolUse` and `PostToolUse` matchers use Claude Code's tool names (`Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`, `mcp__server__tool`).
- Command entries: `command`, `type: command`, and optional `args`, `timeout` (seconds, default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), `if` (a permission rule filter such as `Bash(git *)`), `once`.
- `args` runs without a shell: `command` is one program, each element one literal argument. Qoder then ignores `shell`; sync still writes both and adds a note.
- `once` has no effect, and sync says so. Qoder honors it only for session-scoped hooks ([Hooks](https://docs.qoder.com/cli/hooks), [Subagent](https://docs.qoder.com/cli/subagent)); portable hooks go in settings.
- HTTP handlers: `url`, optional `headers`, `allowedEnvVars`. Prompt handlers: `prompt`, optional `model`. Both keep filters, timeouts in seconds, and matcher groups.
- Other handler types get a sync note.
- Sync writes one `env` entry, `AGNOSTIC_AI_TARGET`, and no `rewakeMessage`, `rewakeSummary`, or agent handlers.

<details>
<summary>Unmanaged AGENTS.md</summary>

Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file. See [the effect on other targets sharing that file](@/docs/target-behavior.md#inlined-rules).

</details>

<details>
<summary>Leftover root .mcp.json</summary>

- Older versions wrote MCP servers to the root `.mcp.json`, shared with Claude Code. The tools' per-server fields differed, so Qoder moved to `.qoder/settings.json`, which Claude Code never reads.
- A leftover `.mcp.json` still loads and wins for a server with the same name.
- Sync cannot remove it: it has no generated-file header and may belong to Claude Code. In a Qoder-only project, delete it by hand.

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

Rule frontmatter keeps `description`, `alwaysApply`, `trigger`, `glob`, and `paths`.

- Native `trigger`, `glob`, and `paths` import under `x-qoder`; `description` and `alwaysApply` use the rule's own fields.
- A single selector stays single, a list stays a list.
- `x-qoder` fields override top-level ones.

[Qoder's activation modes](https://docs.qoder.com/cli/memory#rule-activation-methods):

| Mode | Frontmatter |
| --- | --- |
| Always active | No activation fields, `trigger: always_on`, or `alwaysApply: true` |
| Manual | `trigger: manual` or `alwaysApply: false` |
| Model-selected | `trigger: model_decision` plus a non-empty `description` |
| File matching | `trigger: glob` plus `glob`, or `paths` |

- `trigger` wins over `alwaysApply`.
- A scoped rule gets `paths` covering the scope folder and file patterns, and no other activation fields.
- Validation rejects selectors sync cannot keep.

A manual release rule:

```markdown
---
name: release
x-qoder:
  trigger: manual
---

Run release steps only on request.
```

## Subagent memory

- The portable `memory` field is copied unchanged to `.qoder/agents/<name>.md`, with the scopes Qoder's [subagent reference](https://docs.qoder.com/cli/subagent) accepts: `user`, `project`, `local`. The `--agents` JSON format has no `memory`.
- It works only when top-level `autoMemoryEnabled` is on in [settings](https://docs.qoder.com/cli/settings-reference) (default `false`).
- Qoder documents this; it is not tested at runtime. Only Claude Code is confirmed to act on `memory`.

## Auto memory

Qoder's own memory store is separate from the field above ([Qoder memory](https://docs.qoder.com/cli/memory)).

- Project store: `~/.qoder/projects/<project>/memory/`. User store: `~/.qoder/memory/`.
- Each is a `MEMORY.md` index plus one file per topic. `/memory` shows them; `/memory manage` views, edits, or deletes a topic.
- A session loads the first 200 lines or about 25KB of each active `MEMORY.md`.
- Off by default; see [each tool's own memory](@/docs/memory.md#each-tools-own-memory).
- agnostic-ai never writes that store. To copy its facts into shared personal memory, run `agnostic-ai memory import --path ~/.qoder/projects/<project>/memory`.

## Import

`import qoder` reads rules, agents, skill folders, commands, MCP servers, and portable settings fields.

- `mcpServers` comes from project `.qoder/settings.json` (JSONC allowed) into the MCP source directory. Env and header credentials become environment references.
- A server with a literal OAuth credential is left out, and import names the field to fix.
- Portable settings go to `settings/qoder.yaml`. Rule activation is kept.
- Not imported: hooks, personal settings, the separate project `.mcp.json`.
