+++
title = "Augment"
description = "What agnostic-ai writes for Augment: file paths, what Augment cannot do, and output options."
weight = 250

[extra]
group = "Reference"
target_id = "augment"
+++

# Augment (`augment`)

[Augment Code](https://docs.augmentcode.com/setup-augment/guidelines) reads the root `AGENTS.md`, `.augment/`, and the shared `.agents/skills/` tree.

## Set up this tool

Add `augment` to the [`targets` list](@/docs/configuration.md#targets) in `agnostic-ai.yaml`. Run these commands from the project root:

```bash
agnostic-ai sync --dry-run
agnostic-ai sync
agnostic-ai sync --check
```

Follow [Verify](#verify) for the outputs you enable. To keep existing tool configuration, start with [`agnostic-ai import augment`](@/docs/cli-reference/start.md#import).

## Output

```
AGENTS.md                     # entry-point pointer body + rules (written by sync, shared path)
.augment/
├── rules/<name>.md           # one per rule the root AGENTS.md does not carry, and per agent_requested rule
├── agents/<name>.md          # one per agent
├── commands/<name>.md        # one per command; nested source scope becomes a namespace
└── settings.json             # mcpServers + hooks + toolPermissions, merged; only with MCP, hook, or settings specs
.agents/skills/<name>/SKILL.md  # one folder per skill (shared cross-tool tree)
.augmentignore                # workspace indexing exclusions
.augment-guidelines           # opt-in legacy joined rules, only when rules-file is set
```

- **Rules**: rule bodies go into the shared `## Rules` block of `AGENTS.md`. Augment also loads `.augment/rules/`.
  - An always-applied rule that is already in the block gets no `.augment/rules/` file. See [target behavior](@/docs/target-behavior.md#entry-point-files).
  - A spec with `alwaysApply: false` gets `type: agent_requested` and a `description` (falling back to the rule name). Rules have no `name` key.
- **Legacy guidelines**: `outputs.augment.rules-file: .augment-guidelines` also writes all rules as one document. It is opt-in, because Augment cuts it first when context runs short.
- **Agents** ([subagents docs](https://docs.augmentcode.com/cli/subagents)): `.augment/agents/<name>.md` with `name` (required), `description` (falls back to the spec name), and, when set, `color` (written as is; Augment expects an ANSI color name) and `model`.
  - Augment's `tools` and `disabled_tools` take its own tool names (`view`, `codebase-retrieval`, `str-replace-editor`, ...), so the generic `tools` field raises a sync note. Set `x-augment: {tools: [...]}` or `x-augment: {disabled_tools: [...]}` instead.
- **Skills**: Augment also scans `.claude/skills/` and `.augment/skills/`.
- **Commands**: `.augment/commands/<scope>/<name>.md`, with a directory scope from the source folder as a nested folder name. `description`, `argument-hint`, and `model` stay in frontmatter.
- **Ignore**: ignore specs write `.augmentignore`. `import augment` restores a hand-written one, keeping pattern order and negation.

{% <details summary="Entry point off AGENTS.md"> %}
A `file` output override can move the entry point off the root `AGENTS.md`. Or `AGENTS.md` can be in `sync.unmanaged`, which stops sync writing it, so codex and every other tool that reads it miss rule changes. Either way every rule gets a `.augment/rules/` file, and the default `always_apply` is left out.
{% </details> %}

### MCP

Servers merge into `.augment/settings.json` under `mcpServers`. Stdio uses `command`, `args`, and `env` with no `type`. Remote uses `type: http` or `type: sse` plus `url` and `headers`. This is the Auggie CLI project file Augment recommends for team-shared servers ([config docs](https://docs.augmentcode.com/cli/config)), not the IDE Settings Panel.

- Sync changes only its own keys. `shell`, `startupScript`, `theme`, plugin keys, and tool permissions stay.
- If the file is JSONC, sync drops comments and trailing commas and prints a one-line warning. Keys stay, comments do not.
- Augment documents no per-server `disabled` key, so `disabled: true` is dropped with a sync note. Use `auggie mcp remove`.
- Transports are `stdio`, `sse`, and `http` only ([integrations docs](https://docs.augmentcode.com/cli/integrations)). A `type: ws` spec writes no server and raises a sync note.

### Hooks

Hooks merge in under `hooks`, in the same write as `mcpServers`. Supported events: `PreToolUse`, `PostToolUse`, `Stop`, `SessionStart`, `SessionEnd` ([hooks docs](https://docs.augmentcode.com/cli/hooks)). [`agnostic-ai hook run`](@/docs/spec-format/hooks.md#hook-run) runs them with Auggie's event data, shell, and timeout, so you can test them before a session.

- `timeout` is in **milliseconds**: the spec's seconds times 1000 (vendor default 60000).
- `command` must be a script ending in `.sh`, `.ps1`, `.cmd`, or `.bat`, because Augment never runs an inline shell string. Other commands are written as is with a sync note. A hook with `args` is skipped with a note, because Augment has no shell to fold them into.
- `matcher` is optional on `PreToolUse` and `PostToolUse` (vendor default `.*`) and left out on the session events.
- Matchers use Augment's tool names (`launch-process`, `str-replace-editor`, `save-file`, ...). A Claude-style matcher (`Bash`, `Write`, ...) matches nothing and raises a sync note.

### Shared memory

With `builtins: [memory]`, sync adds an executable `.augment/hooks/memory-augment-session-start.sh` and a `SessionStart` entry for Auggie CLI on Unix. The script finds the project from its working directory and returns both memory indexes through `hookSpecificOutput.additionalContext`. The same script is installed for `sync --global`.

Windows and the IDE keep the memory rule as their fallback. See [shared memory](@/docs/memory.md#load-at-session-start) for the context limit and how to keep the tool's own memory.

### Permissions

Settings specs merge into the same file under `toolPermissions`, which Auggie CLI and Cosmos cloud agents follow. Augment recommends committing `.augment/settings.json` so the policy applies to every cloud agent in the repo ([permissions docs](https://docs.augmentcode.com/cli/permissions)).

- Rules are an ordered array and the first match wins, so deny rules come first.
- Each `permission` is an **object** such as `{ "type": "deny" }`. Augment ignores a bare-string permission.
- Bare tool names map onto Augment's six tools (`Read` to `read`, `Bash` to `terminal`, `WebFetch` to `web-fetch`, ...).
- `Bash(prefix:*)` becomes a `terminal` rule with `shellInputRegex: ^prefix`. An exact `Bash(command)` is anchored at both ends. `mcp__<server>__<tool>` becomes `{tool-name}_{server-name}`.

These are not written and raise a sync note:

- `model`: the file has no documented key.
- The `ask` list: none of Augment's types (`allow`, `deny`, `webhook-policy`, `script-policy`) prompts.
- Path- or URL-scoped rules such as `Read(src/**)`: only `terminal` takes a matcher, and a bare `read` would cover every file.

`x-augment.toolPermissions` writes Augment's rule objects as is, ahead of the translated ones. Other `x-augment` keys, such as `shell` or `startupScript`, merge in as written.

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

`import augment` reads `.augmentignore` and the `toolPermissions` policy in `.augment/settings.json` for Augment's six tools. It skips `shellInputRegex` entries (no portable form) and bare-string `permission` entries (invalid in Augment). Nothing else is imported.

## Protected paths

Not enforced. Sync cannot write an edit-blocking rule or hook for this tool, so it prints a sync note for [protected paths](@/docs/spec-format/settings.md#protected-paths). State the paths in a rule if the agent should know about them.

## Verify

1. Install the Augment Code extension ([guidelines docs](https://docs.augmentcode.com/setup-augment/guidelines)).
2. Check the files: `ls AGENTS.md .augment/agents/ .agents/skills/`, plus `.augment/rules/` (a rule with `alwaysApply: false`), `.augment-guidelines` (`outputs.augment.rules-file` set), and `.augment/settings.json` (MCP, hook, or settings specs).
3. Open the project. Augment reads `AGENTS.md`, `.augment/rules/`, `.augment/agents/`, `.agents/skills/`, `.augment-guidelines` when present, and (through Auggie CLI) `.augment/settings.json`.
4. With hook specs, `auggie` starts with no "invalid hook" warning, and a `PreToolUse` hook on a `.sh`/`.ps1`/`.cmd`/`.bat` script runs on the matching tool call.
5. With settings specs, `auggie` starts with no dropped-rule warning. A `deny` tool is blocked, and an `allow` tool runs without approval.
