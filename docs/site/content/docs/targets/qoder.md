+++
title = "Qoder"
description = "How agnostic-ai emits Qoder configuration: native paths, capability limits, and output options."
weight = 190

[extra]
group = "Reference"
target_id = "qoder"
+++

# Qoder (`qoder`)

## Output

```
AGENTS.md                        # entry-point pointer body (shared path)
.qoder/rules/<name>.md           # one per rule
.qoder/agents/<name>.md          # one per agent
.qoder/skills/<name>/SKILL.md    # one folder per skill, plus bundled assets
.qoder/commands/<name>.md        # one per command
.qoder/settings.json             # MCP, hooks, and settings (merged; unrelated keys preserved)
```

Alibaba [Qoder](https://docs.qoder.com/user-guide/rules) reads one Markdown file per rule from `.qoder/rules/` and also reads the root `AGENTS.md`. Per-rule files take precedence, so rules emit there instead of inlining into the pointer.

Skills use Qoder's own tree at `.qoder/skills/<name>/SKILL.md` ([Qoder Skills](https://docs.qoder.com/extensions/skills)). Qoder does not list `.agents/skills/` as a compatible path, so this tree does not dedupe with the shared one. The user-level `~/.qoder/skills/` tier is out of reach.

`import qoder` reads rules, agents, skill folders, commands, and portable settings fields. Rule activation conditions survive import and sync. It does not read hooks or MCP servers back out of `.qoder/settings.json`.

- **Agents**: [Qoder Subagents](https://docs.qoder.com/extensions/subagent) reads `.qoder/agents/<name>.md`. `name` and `description` are required; `model`, `tools`, `color`, `skills`, `mcpServers`, `effort`, `permissionMode`, `memory`, and `hooks` are optional. `effort` takes a scalar or a per-target map, and an integer budget passes through as a number. See [per-target `model` and `effort`](@/docs/spec-format.md#per-target-model-and-effort) and [agent policy support by target](@/docs/spec-format.md#agent-policy-support-by-target).
  - `color` takes one of eight names: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan` ([CLI field reference](https://docs.qoder.com/cli/subagent)).
  - `tools` renders as a comma-separated string (`tools: Read, Grep, Bash`). `import qoder` splits it back into a list. Qoder's tool names are Claude-style (`Bash`, `Edit`, `Write`, `Glob`, `Grep`, `Read`, `WebFetch`, `WebSearch`), so the generic `tools` list passes straight through.
  - `skills` and `mcpServers` pass through from `x-qoder`.
  - `memory` passes through unchanged. See [Subagent memory](#subagent-memory).
  - The adapter manages `name`, `description`, `model`, `tools`, `skills`, and `mcpServers`. Any other `x-qoder` key passes through into the frontmatter.
- **Skills**: frontmatter is plain `name` and `description`. Bundled files (scripts, references, templates) copy byte-for-byte next to `SKILL.md`.
- **Commands**: one file per command at `.qoder/commands/<name>.md`, the project path for both the [CLI](https://docs.qoder.com/cli/commands) and the [IDE](https://docs.qoder.com/user-guide/commands).
  - The adapter always writes `description`, falling back to the command's name. It never writes `name`, since the file path sets the invocation name.
  - On a name clash, the Qoder CLI lets the user-level command (`~/.qoder/commands/`) override the project one. The IDE lists both with a scope indicator. The adapter cannot see the user tier to warn about clashes.
- **MCP**: merges into `.qoder/settings.json` under `mcpServers` (stdio: `command`/`args`/`env`/`cwd`, no `type`; remote: `type` plus `url`/`headers`).
  - Every entry also accepts the [common optional fields](https://docs.qoder.com/cli/mcp-reference): `timeout` (milliseconds), `description`, `trust` (skip tool-call confirmation), `includeTools`, `excludeTools`, `alwaysAllow`, `disabled` (keep the config but turn the server off), and an `oauth` object passed through as declared.
  - A `type: ws` spec emits no server and raises a coverage note. Qoder's ws transport takes a TCP host and port, which the spec cannot express.
  - Unrelated keys (`mcp.enableAllProjectMcpServers`, permissions, custom models) survive every sync; only `mcpServers` is overwritten.
  - The file may be JSONC ([settings](https://docs.qoder.com/cli/settings)). `sync` keeps the keys but drops comments and trailing commas, and prints a warning when it does.
- **Settings**: the shared default model maps to `model.name`; `permissions.allow`, `permissions.deny`, and `permissions.ask` map directly. The write merges with hooks, MCP servers, and unrelated keys. An `x-qoder` block on a settings spec merges into the same write. Import restores the portable fields to `settings/qoder.yaml`.

  Older agnostic-ai versions wrote MCP servers to the project-root `.mcp.json`, shared with Claude Code. The two tools' per-server fields diverged, so Qoder moved to `.qoder/settings.json`, which Claude Code never reads. A leftover `.mcp.json` still loads and wins over `.qoder/settings.json` for a same-named server. Sync cannot remove it (it has no provenance header and may belong to Claude Code), so delete it by hand in a Qoder-only project. See [`disabled` support by target](@/docs/spec-format.md#disabled-support-by-target).

- **Hooks**: merge into the same `.qoder/settings.json` under `hooks` ([Qoder hooks](https://docs.qoder.com/cli/hooks)), using the nested shape Claude Code, Codex, and OpenHands share. Hooks and MCP servers merge in one write.
  - 27 PascalCase events ([hooks reference](https://docs.qoder.com/cli/hooks-reference)): `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `PreCompact`, `PostCompact`, `Notification`, `InstructionsLoaded`, `ConfigChange`, `CwdChanged`, `FileChanged`, `WorktreeCreate`, `WorktreeRemove`, `Elicitation`, `ElicitationResult`, `TaskCreated`, `TaskCompleted`, `TeammateIdle`, `Setup`.
  - `event:` passes through verbatim. Listed events sort in the vendor's order, ahead of unlisted ones.
  - `PreToolUse`/`PostToolUse` matchers use Claude Code's tool names (`Bash`, `Write`, `Edit`, `Read`, `Glob`, `Grep`, `mcp__server__tool`), so Claude-authored matchers work unchanged.
  - Command entries carry `command`, `type: command`, optional `args`, `timeout` (seconds, default 600), `statusMessage`, `async`, `asyncRewake`, `shell` (`bash` or `powershell`), `if` (a permission-rule filter, e.g. `Bash(git *)`), and `once`.
  - `args` switches to exec form: `command` is one executable and each `args` element is one literal argument, run with no shell. Qoder ignores `shell` once `args` is set; a spec with both still writes both, and `sync` prints a note.
  - `once` is written but has no effect. Qoder honors it only for session-scoped hooks ([Hooks](https://docs.qoder.com/cli/hooks), [Subagent](https://docs.qoder.com/cli/subagent)), and portable hooks land in settings, so the hook runs every time. `sync` prints a note.
  - Qoder's `env`, `rewakeMessage`, `rewakeSummary`, and agent handlers are not emitted.

HTTP and prompt hooks emit from portable hook specs. HTTP handlers carry `url`, optional `headers`, and `allowedEnvVars`; prompt handlers carry `prompt` and optional `model`. Both keep filters, timeouts in seconds, and matcher groups. Other handler types produce a coverage note. Hook import is unsupported.

## Rule activation

Qoder rules keep `description`, `alwaysApply`, `trigger`, `glob`, and `paths` frontmatter. Native `trigger`, `glob`, and `paths` import under `x-qoder`; `description` and `alwaysApply` use the existing rule metadata. Scalar and list selectors keep their shape. Native fields authored under `x-qoder` override top-level metadata.

[Qoder's rule activation modes](https://docs.qoder.com/cli/memory#rule-activation-methods):

| Mode | Frontmatter |
| --- | --- |
| Always active | No activation fields, `trigger: always_on`, or `alwaysApply: true` |
| Manual | `trigger: manual` or `alwaysApply: false` |
| Model-selected | `trigger: model_decision` plus a non-empty `description` |
| File matching | `trigger: glob` plus `glob`, or `paths` |

`trigger` wins over `alwaysApply`. Portable `scope` stays authoritative: scoped rules emit normalized `paths` with no competing activation fields. Scope validation still rejects selectors it cannot safely intersect.

For a manual release rule:

```markdown
---
name: release
x-qoder:
  trigger: manual
---

Run release steps only on request.
```

## Config keys

| Key | Default |
| --- | --- |
| `outputs.qoder.rules-dir` | `.qoder/rules` |
| `outputs.qoder.agents-dir` | `.qoder/agents` |
| `outputs.qoder.skills-dir` | `.qoder/skills` |
| `outputs.qoder.commands-dir` | `.qoder/commands` |
| `outputs.qoder.mcp-file` | `.qoder/settings.json` (also the hooks path) |

## Subagent memory

The portable `memory` field reaches `.qoder/agents/<name>.md` unchanged. Qoder's [subagent reference](https://docs.qoder.com/cli/subagent) accepts the same three scopes: `user`, `project`, `local`. Markdown is the only way in; the `--agents` JSON schema does not carry `memory`.

It only works when top-level `autoMemoryEnabled` is on in [settings](https://docs.qoder.com/cli/settings-reference), which defaults to `false`.

This is documented, not runtime-verified. Claude Code is the one target confirmed to act on `memory`.

## Auto memory

This is Qoder's own store, separate from the field above ([Qoder memory](https://docs.qoder.com/cli/memory)). The project store lives at `~/.qoder/projects/<project>/memory/` and the user store at `~/.qoder/memory/`. Each is a `MEMORY.md` index plus one file per topic. Run `/memory` for the overview and `/memory manage` to view, edit, or delete a topic. A session loads the first 200 lines or about 25KB of each active `MEMORY.md`.

Auto memory is off by default, and agnostic-ai never reads or writes the store. See [Memory and local state](@/docs/target-behavior.md#memory-and-local-state).

## Verify

1. Install Qoder from [qoder.com](https://qoder.com).
2. Check the tree:
   - `ls AGENTS.md .qoder/rules/ .qoder/agents/ .qoder/skills/ .qoder/commands/ .qoder/settings.json`
   - `grep "Generated by agnostic-ai" .qoder/rules/*.md .qoder/agents/*.md .qoder/skills/*/SKILL.md .qoder/commands/*.md` for the provenance header
   - `python -m json.tool .qoder/settings.json > /dev/null`
3. Open the project:
   - The rules panel lists every `.qoder/rules/*.md` with no parse warnings.
   - The agent picker lists every `.qoder/agents/*.md`.
   - Each `.qoder/skills/<name>/` folder loads as a skill.
   - Each `.qoder/commands/<name>.md` runs from `/`.
   - The MCP picker shows each `mcpServers.<name>` connected (project servers need approval on first use).
   - A configured hook fires on its event (e.g. a `PreToolUse` hook prints or blocks before the matching tool runs).
