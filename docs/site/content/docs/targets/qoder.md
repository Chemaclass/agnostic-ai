+++
title = "Qoder"
description = "How agnostic-ai emits Qoder configuration: native paths, capability limits, and output options."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"
+++

# Qoder (`qoder`)

agnostic-ai writes Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) rules, agents, skills, and commands under `.qoder/`, and merges MCP, hooks, and settings into `.qoder/settings.json`.

## Output

```
AGENTS.md                        # entry-point pointer body, plus the rules block when an inlining target shares it
.qoder/rules/<name>.md           # one per rule AGENTS.md does not carry, or that is not always-on
.qoder/agents/<name>.md          # one per agent
.qoder/skills/<name>/SKILL.md    # one folder per skill, plus bundled assets
.qoder/commands/<name>.md        # one per command
.qoder/settings.json             # MCP, hooks, and settings (merged; unrelated keys preserved)
```

- **Rules**: Qoder reads `.qoder/rules/` and the root `AGENTS.md`, and per-rule files win a conflict, so rules emit there instead of inlining. When codex or another inlining target adds the `## Rules` block to `AGENTS.md`, an always-on rule gets no file and loads once. A rule whose text differs for Qoder keeps its file. The Qoder CLI reads `AGENTS.md` unless `context.fileName` names other files; keep it listed there, or the CLI misses rules without a file. See [Rule activation](#rule-activation) and [target behavior](@/docs/target-behavior.md#entry-point-files).
- **Agents**: [Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`. `name` and `description` are required; `model`, `tools`, `color`, `skills`, `mcpServers`, `effort`, `permissionMode`, `memory`, and `hooks` are optional. `effort` takes a scalar or per-target map, and an integer budget stays a number. See [per-target `model` and `effort`](@/docs/spec-format/agents.md#per-target-model-and-effort) and [agent policy support by target](@/docs/spec-format/agents.md#agent-policy-support-by-target).
  - `color` is one of `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([CLI field reference](https://docs.qoder.com/cli/subagent)).
  - `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`), which `import qoder` splits back. Qoder uses Claude-style names (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), so the generic list passes straight through.
  - `skills` and `mcpServers` come from `x-qoder`. `memory` passes through unchanged; see [Subagent memory](#subagent-memory).
  - The adapter manages `name`, `description`, `model`, `tools`, `skills`, and `mcpServers`; any other `x-qoder` key passes through.
- **Skills**: Qoder's own `.qoder/skills/<name>/SKILL.md` ([Qoder Skills](https://docs.qoder.com/extensions/skills)), with plain `name` and `description` frontmatter and bundled files (scripts, references, templates) copied byte-for-byte next to `SKILL.md`. Qoder does not list `.agents/skills/` as compatible, so no dedupe with the shared tree. The user-level `~/.qoder/skills/` tier is out of reach.
- **Commands**: `.qoder/commands/<name>.md`, the project path for the [CLI](https://docs.qoder.com/cli/commands) and the [IDE](https://docs.qoder.com/user-guide/commands). The adapter always writes `description` (default: the command's name) and never `name`, since the file path sets it. On a clash, the CLI prefers the user-level command (`~/.qoder/commands/`) and the IDE lists both with a scope indicator; the adapter cannot see that tier to warn.
- **MCP**: merges `mcpServers` into `.qoder/settings.json` (stdio: `command`/`args`/`env`/`cwd`, no `type`; remote: `type` plus `url`/`headers`); other keys such as `mcp.enableAllProjectMcpServers`, permissions, and custom models survive.
  - Entries accept the [common optional fields](https://docs.qoder.com/cli/mcp-reference): `timeout` (milliseconds), `description`, `trust` (skip tool-call confirmation), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` (keep the config, turn the server off), and an `oauth` object as declared. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).
  - A `type: ws` spec emits only a coverage note: Qoder's ws transport takes a TCP host and port the spec cannot express.
  - The file may be JSONC ([settings](https://docs.qoder.com/cli/settings)). `sync` keeps the keys but drops comments and trailing commas, with a warning.
- **Settings**: the shared default model maps to `model.name`; `permissions.allow`, `permissions.deny`, and `permissions.ask` map directly. An `x-qoder` settings block merges in too.
- **Hooks**: merge under `hooks` in the same write ([Qoder hooks](https://docs.qoder.com/cli/hooks)), in the nested shape Claude Code, Codex, and OpenHands share.
  - 27 PascalCase events ([hooks reference](https://docs.qoder.com/cli/hooks-reference)): `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`. `event:` passes through verbatim; listed events sort in vendor order, ahead of unlisted ones.
  - `PreToolUse`/`PostToolUse` matchers use Claude Code's tool names (`Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`, `mcp__server__tool`), so Claude-authored matchers work unchanged.
  - Command entries carry `command`, `type: command`, optional `args`, `timeout` (seconds, default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), `if` (a permission-rule filter, e.g. `Bash(git *)`), and `once`.
  - `args` switches to exec form: `command` is one executable, each `args` element one literal argument, with no shell. Qoder then ignores `shell`; sync still writes both, with a note.
  - `once` has no effect, since Qoder honors it only for session-scoped hooks ([Hooks](https://docs.qoder.com/cli/hooks), [Subagent](https://docs.qoder.com/cli/subagent)) and portable hooks land in settings; `sync` notes it.
  - HTTP handlers carry `url`, optional `headers`, and `allowedEnvVars`; prompt handlers carry `prompt` and optional `model`. Both keep filters, timeouts in seconds, and matcher groups.
  - Other handler types get a coverage note. Qoder's `env`, `rewakeMessage`, `rewakeSummary`, and agent handlers are not emitted.

{% <details summary="Unmanaged AGENTS.md"> %}
Listing `AGENTS.md` under `sync.unmanaged` makes every rule keep its file, but sync then stops writing `AGENTS.md`, so other readers such as codex lose rule changes.
{% </details> %}

{% <details summary="Leftover root .mcp.json"> %}
Older versions wrote MCP servers to the root `.mcp.json` shared with Claude Code; the tools' per-server fields diverged, so Qoder moved to `.qoder/settings.json`, which Claude Code never reads. A leftover `.mcp.json` still loads and wins over `.qoder/settings.json` for a same-named server. Sync cannot remove it (no provenance header, and it may belong to Claude Code); delete it by hand in a Qoder-only project.
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

Qoder rules keep `description`, `alwaysApply`, `trigger`, `glob`, and `paths` frontmatter. Native `trigger`, `glob`, and `paths` import under `x-qoder`; `description` and `alwaysApply` use the existing rule metadata. Scalar and list selectors keep their shape. Native fields under `x-qoder` override top-level metadata.

[Qoder's rule activation modes](https://docs.qoder.com/cli/memory#rule-activation-methods):

| Mode | Frontmatter |
| --- | --- |
| Always active | No activation fields, `trigger: always_on`, or `alwaysApply: true` |
| Manual | `trigger: manual` or `alwaysApply: false` |
| Model-selected | `trigger: model_decision` plus a non-empty `description` |
| File matching | `trigger: glob` plus `glob`, or `paths` |

`trigger` wins over `alwaysApply`. Scoped rules emit normalized `paths` for the union of the scope directory and file patterns, with no competing activation fields. Scope validation rejects selectors it cannot preserve.

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

The portable `memory` field reaches `.qoder/agents/<name>.md` unchanged. Qoder's [subagent reference](https://docs.qoder.com/cli/subagent) accepts the same scopes: `user`, `project`, `local`. Only Markdown carries it; the `--agents` JSON schema has no `memory`.

It works only with top-level `autoMemoryEnabled` on in [settings](https://docs.qoder.com/cli/settings-reference) (default `false`). Documented, not runtime-verified: only Claude Code is confirmed to act on `memory`.

## Auto memory

Qoder's own store, separate from the field above ([Qoder memory](https://docs.qoder.com/cli/memory)), lives at `~/.qoder/projects/<project>/memory/` (project) and `~/.qoder/memory/` (user). Each is a `MEMORY.md` index plus one file per topic. `/memory` shows them; `/memory manage` views, edits, or deletes a topic. A session loads the first 200 lines or about 25KB of each active `MEMORY.md`.

Off by default; agnostic-ai never reads or writes it. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

## Import

`import qoder` reads rules, agents, skill folders, commands, and portable settings fields, which land in `settings/qoder.yaml`. Rule activation survives the round trip. Hooks and MCP servers in `.qoder/settings.json` stay unread.

## Protected paths

Advisory. This target has no native edit guard that sync writes, so sync prints a coverage note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install Qoder from [qoder.com](https://qoder.com).
2. Check the tree:
   - `ls AGENTS.md .qoder/rules/ .qoder/agents/ .qoder/skills/ .qoder/commands/ .qoder/settings.json`
   - `grep "Generated by agnostic-ai" .qoder/rules/*.md .qoder/agents/*.md .qoder/skills/*/SKILL.md .qoder/commands/*.md` for the provenance header
   - `python -m json.tool .qoder/settings.json > /dev/null`
3. Open the project:
   - The rules panel lists every `.qoder/rules/*.md` with no parse warnings, and the agent picker every `.qoder/agents/*.md`.
   - Each `.qoder/skills/<name>/` loads as a skill, and each `.qoder/commands/<name>.md` runs from `/`.
   - The MCP picker shows each `mcpServers.<name>` connected (project servers need approval on first use).
   - A hook fires on its event (e.g. a `PreToolUse` hook prints or blocks first).
