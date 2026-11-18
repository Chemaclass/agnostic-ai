+++
title = "Augment native reference"
description = "Native field mappings, output keys, and import coverage for augment."
weight = 251

[extra]
nav_title = "Augment reference"
group = "Reference"
+++

# Augment native reference

Start with the [Augment setup](@/docs/targets/augment.md).

## Native mappings

### Rules

Rule bodies go into the shared `## Rules` block of `AGENTS.md`. Augment also loads `.augment/rules/`.

- An always-applied rule already in the block gets no `.augment/rules/` file. See [target behavior](@/docs/target-behavior.md#entry-point-files).
- A spec with `alwaysApply: false` gets `type: agent_requested` and a `description` (falls back to the rule name). Rules have no `name` key.
- `outputs.augment.rules-file: .augment-guidelines` also writes all rules as one legacy document. It is opt-in, because Augment cuts it first when context runs short.

<details>
<summary>Entry point off AGENTS.md</summary>

Moving the entry point off root `AGENTS.md` with a `file` override, or listing it in `sync.unmanaged`, makes every rule keep a `.augment/rules/` file. The default `always_apply` is left out. Before making the shared file unmanaged, check [its effect on other targets](@/docs/target-behavior.md#inlined-rules).

</details>

### Agents

[Subagents docs](https://docs.augmentcode.com/cli/subagents). Files go to `.augment/agents/<name>.md`.

- Frontmatter has `name` (required) and `description` (falls back to the spec name).
- `color` and `model` are written when set. `color` is written as is. Augment expects an ANSI color name.
- `effort` becomes `thinking` when it is `off`, `minimal`, `low`, `medium`, `high`, or `xhigh`. Any other value raises a sync note.
- `x-augment: {thinking: ...}` wins over `effort`.
- Auggie 1.x lists `thinking` in its [agent fields](https://docs.augmentcode.com/cli/subagents.md). The 0.x agent docs do not, so 0.x may ignore it.
- Augment's `tools` and `disabled_tools` take its own tool names, not Claude-style ones. The generic `tools` field uses neither, so it raises a sync note. Set `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}`.

| Field | Accepts |
|---|---|
| `tools` allowlist, Auggie 0.x | `view`, `codebase-retrieval`, `str-replace-editor`, ... |
| `tools` allowlist, Auggie 1.x | Only `read`, `bash`, `edit`, `write`, `grep`, `find`, `ls`, `web-fetch` |
| `disabled_tools` | Any tool name |

### Skills

Augment also scans `.claude/skills/` and `.augment/skills/`.

### Commands

`.augment/commands/<scope>/<name>.md`. A directory scope from the source folder becomes a nested folder name. `description`, `argument-hint`, and `model` stay in frontmatter.

### Ignore

Ignore specs write `.augmentignore`. `import augment` restores a hand-written one, keeping pattern order and negation.

### MCP

Servers merge into `.augment/settings.json` under `mcpServers`. This is the Auggie CLI project file Augment recommends for team-shared servers ([config docs](https://docs.augmentcode.com/cli/config)), not the IDE Settings Panel.

- Stdio uses `command`, `args`, and `env`, with no `type`.
- Remote uses `type: http` or `type: sse`, plus `url` and `headers`.
- Sync changes only its own keys. `shell`, `startupScript`, `theme`, plugin keys, and tool permissions stay.
- If the file is JSONC, sync keeps its keys but drops comments and trailing commas, with a one-line warning.
- Augment documents no per-server `disabled` key. `disabled: true` is dropped with a sync note. Use `auggie mcp remove`.
- Transports are `stdio`, `sse`, and `http` only ([integrations docs](https://docs.augmentcode.com/cli/integrations)). A `type: ws` spec writes no server and raises a sync note.

### Hooks

Hooks merge in under `hooks`, in the same write as `mcpServers`. Events ([hooks docs](https://docs.augmentcode.com/cli/hooks)): `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd`, `PromptSubmit`, `Notification`.

- `PromptSubmit` and `Notification` come from the Auggie 1.x reference. The 0.x reference documents neither as registrable ([0.x docs](https://docs.augmentcode.com/cli/v0/hooks.md)).
- [`agnostic-ai hook run`](@/docs/cli-reference/hook-diagnostics.md#hook-run) tests hooks before a session, using Auggie's event data, shell, and timeout.
- A hook on another event, such as Claude Code's `UserPromptSubmit`, is skipped with a sync note, and `validate` rejects it.
- Auggie's `PromptSubmit` passes the prompt as `user_prompt` and cannot block, so a shared prompt hook is not renamed. If the hook fits, set `x-augment: {event: PromptSubmit}`.
- `timeout` is in **milliseconds**: the spec's seconds times 1000 (vendor default 60000).
- `args` is its own array. Auggie runs the command with them and no shell. Auggie 1.x first expands `${VAR}` and `${VAR:-default}` in the command and each argument, so `sync` writes them as you wrote them.
- A command without `args` runs as written: 1.x hands it to a shell, 0.x splits it into words or uses `bash -c` for shell syntax.
- `matcher` is optional on `PreToolUse` and `PostToolUse` (default `.*`). The other five events take none, so sync leaves it out with a note.

Matchers use Augment's tool names:

- Auggie 1.x names its tools `bash`, `read`, `edit`, `write`, `grep`, `find`, `web-fetch`, ... and matches case-insensitively.
- It still matches the 0.x names `launch-process`, `view`, `str-replace-editor`, and `save-file` for now. Its `/hooks` command flags them ([hooks docs](https://docs.augmentcode.com/cli/hooks.md)).
- Auggie 0.x knows only the 0.x names and matches case-sensitively.
- A Claude-style matcher (`Bash`, `Write`, ...) raises a sync note. In 1.x only `Bash`, `Read`, `Edit`, `Write`, and `Grep` reach a tool. In 0.x none does.

Blocking:

- Only `PreToolUse` stops a tool: exit 2 or `permissionDecision: "deny"`.
- A `PostToolUse` `decision: "block"` sends its reason to the agent. The tool has already run.
- `Stop` blocks in Auggie 0.x only.
- Auggie 1.x runs `Stop`, `SessionStart`, `SessionEnd`, `PromptSubmit`, and `Notification` hooks in the background, where a block reply does nothing.
- `hook run` reports block on `PostToolUse` and `Stop`, with a note on `Stop`. On the other four it reads a block reply as allow, with a note.

### Shared memory

With `builtins: [memory]`, sync adds an executable `.augment/hooks/memory-augment-session-start.sh` and a `SessionStart` entry for Auggie CLI on its [supported platforms](https://docs.augmentcode.com/cli/setup-auggie/install-auggie-cli#system-requirements): macOS, Linux, and Windows WSL.

- The script finds the project from its working directory and returns both indexes in `hookSpecificOutput.additionalContext`. `sync --global` installs it too.
- Auggie 0.x injects that output.
- Auggie 1.x (stable from 1.2.0) runs `SessionStart` hooks in the background and drops their output ([1.x hooks docs](https://docs.augmentcode.com/cli/hooks.md)), so the memory rule tells the agent to read the indexes.
- The IDE keeps the memory rule.
- See [shared memory](@/docs/memory.md#load-at-session-start) for the context limit and the tool's own memory.

### Permissions

Settings spec permissions merge into the same file under `toolPermissions`, which Auggie CLI and Cosmos cloud agents follow. Augment recommends committing `.augment/settings.json` so the policy applies to every cloud agent in the repo ([permissions docs](https://docs.augmentcode.com/cli/permissions)).

- Rules are an ordered array and the first match wins. Deny rules come first, then ask, then allow.
- `ask` becomes `{ "type": "ask-user" }`. Auggie asks before the call, and denies when there is no interactive UI (headless). Auggie 0.x reads the same type.
- Each `permission` is an **object** such as `{ "type": "deny" }`. Augment ignores a bare-string permission.
- Bare tool names map onto Augment's six tools (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, ...).
- `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`. An exact `Bash(command)` is anchored at both ends.
- `mcp__<server>__<tool>` becomes `{tool-name}_{server-name}`.
- Path or URL rules such as `Read(src/**)` are not written and raise a sync note. Only `terminal` takes a matcher, and a bare `read` would cover every file.
- `x-augment.toolPermissions` writes Augment's rule objects as is, ahead of the translated ones.
- Other `x-augment` keys, such as `shell` or `startupScript`, merge in as written.

Model and effort:

- `model` becomes `defaultModel`. `effort` becomes `defaultThinkingLevel` (`off`, `minimal`, `low`, `medium`, `high`, or `xhigh`). Another value raises a sync note.
- Auggie 1.x reads both keys ([settings docs](https://docs.augmentcode.com/cli/settings.md)). It also reads `model` when `defaultModel` is absent.
- Auggie 0.x ignores them and reads only `model`. Set it with `x-augment: {model: ...}`.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.augment.rules-dir` | `.augment/rules` | |
| `outputs.augment.agents-dir` | `.augment/agents` | |
| `outputs.augment.skills-dir` | `.agents/skills` | |
| `outputs.augment.commands-dir` | `.augment/commands` | |
| `outputs.augment.ignore-file` | `.augmentignore` | |
| `outputs.augment.rules-file` | unset | opt-in, writes the legacy `.augment-guidelines` document |
| `outputs.augment.mcp-file` | `.augment/settings.json` | also the hooks and permissions file, because all three merge into one document |

## Import

`import augment` reads `.augmentignore`, `.augment/agents/*.md`, and the `toolPermissions` policy in `.augment/settings.json` for Augment's six tools. Nothing else is imported.

- An `ask-user` rule goes to the `ask` list.
- `shellInputRegex` entries are skipped (no portable form).
- Bare-string `permission` entries are skipped (invalid in Augment).
- `defaultModel` becomes `model.augment`, or `x-augment.defaultModel` when another settings spec sets a different Augment model.
- `defaultThinkingLevel` becomes `effort`, following the [effort import rules](@/docs/spec-format/settings-reference.md#effort-by-target).
- In agents, `thinking` becomes `effort` when it is one of the six levels.
- Agent keys with no portable field (`tools`, `max_turns`, and similar) go under `x-augment`.
